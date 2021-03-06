package sabakan

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ignition "github.com/coreos/ignition/config/v2_3/types"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/neco"
	"github.com/cybozu-go/neco/storage"
	sabakan "github.com/cybozu-go/sabakan/client"
	"github.com/cybozu-go/well"
	"github.com/ghodss/yaml"
)

const (
	imageOS = "coreos"
)

const retryCount = 5

// UploadContents upload contents to sabakan
func UploadContents(ctx context.Context, sabakanHTTP *http.Client, proxyHTTP *http.Client, version string, auth *DockerAuth, st storage.Storage) error {
	client, err := sabakan.NewClient(neco.SabakanLocalEndpoint, sabakanHTTP)
	if err != nil {
		return err
	}

	env := well.NewEnvironment(ctx)
	env.Go(func(ctx context.Context) error {
		return uploadOSImages(ctx, client, proxyHTTP)
	})
	env.Go(func(ctx context.Context) error {
		return uploadAssets(ctx, client, auth)
	})
	env.Go(func(ctx context.Context) error {
		return uploadIgnitions(ctx, client, version, st)
	})
	env.Stop()
	return env.Wait()
}

// uploadOSImages uploads CoreOS images
func uploadOSImages(ctx context.Context, c *sabakan.Client, p *http.Client) error {
	index, err := c.ImagesIndex(ctx, imageOS)
	if err != nil {
		return err
	}

	version := neco.CurrentArtifacts.CoreOS.Version
	if len(index) != 0 && index[len(index)-1].ID == version {
		// already uploaded
		return nil
	}

	kernelURL, initrdURL := neco.CurrentArtifacts.CoreOS.URLs()

	kernelFile, err := ioutil.TempFile("", "kernel")
	if err != nil {
		return err
	}
	defer func() {
		kernelFile.Close()
		os.Remove(kernelFile.Name())
	}()
	initrdFile, err := ioutil.TempFile("", "initrd")
	if err != nil {
		return err
	}
	defer func() {
		initrdFile.Close()
		os.Remove(initrdFile.Name())
	}()

	env := well.NewEnvironment(ctx)

	var kernelSize int64
	env.Go(func(ctx context.Context) error {
		return neco.RetryWithSleep(ctx, retryCount, 10*time.Second,
			func(ctx context.Context) error {
				err := kernelFile.Truncate(0)
				if err != nil {
					return err
				}
				_, err = kernelFile.Seek(0, 0)
				if err != nil {
					return err
				}
				kernelSize, err = downloadFile(ctx, p, kernelURL, kernelFile)
				return err
			},
			func(err error) {
				log.Warn("sabakan: failed to fetch Container Linux kernel", map[string]interface{}{
					log.FnError: err,
					"url":       kernelURL,
				})
			},
		)
	})

	var initrdSize int64
	env.Go(func(ctx context.Context) error {
		return neco.RetryWithSleep(ctx, retryCount, 10*time.Second,
			func(ctx context.Context) error {
				err := initrdFile.Truncate(0)
				if err != nil {
					return err
				}
				_, err = initrdFile.Seek(0, 0)
				if err != nil {
					return err
				}
				initrdSize, err = downloadFile(ctx, p, initrdURL, initrdFile)
				return err
			},
			func(err error) {
				log.Warn("sabakan: failed to fetch Container Linux initrd", map[string]interface{}{
					log.FnError: err,
					"url":       initrdURL,
				})
			},
		)
	})
	env.Stop()
	err = env.Wait()
	if err != nil {
		return err
	}

	_, err = kernelFile.Seek(0, 0)
	if err != nil {
		return err
	}
	_, err = initrdFile.Seek(0, 0)
	if err != nil {
		return err
	}

	return c.ImagesUpload(ctx, imageOS, version, kernelFile, kernelSize, initrdFile, initrdSize)
}

// uploadAssets uploads assets
func uploadAssets(ctx context.Context, c *sabakan.Client, auth *DockerAuth) error {
	// Upload bird and chrony with ubuntu-debug
	for _, img := range neco.SystemContainers {
		err := uploadSystemImageAssets(ctx, img, c)
		if err != nil {
			return err
		}
	}

	// Upload other images
	var fetches []neco.ContainerImage
	images := neco.SabakanPublicImages
	if auth != nil {
		images = append(images, neco.SabakanPrivateImages...)
	}
	for _, name := range images {
		img, err := neco.CurrentArtifacts.FindContainerImage(name)
		if err != nil {
			return err
		}
		fetches = append(fetches, img)
	}

	env := well.NewEnvironment(ctx)
	for _, img := range fetches {
		img := img
		env.Go(func(ctx context.Context) error {
			return UploadImageAssets(ctx, img, c, auth)
		})
	}
	env.Stop()
	err := env.Wait()
	if err != nil {
		return err
	}

	// Upload sabakan-cryptsetup with version name
	img, err := neco.CurrentArtifacts.FindContainerImage("sabakan")
	if err != nil {
		return err
	}
	name := neco.CryptsetupAssetName(img)
	need, err := needAssetUpload(ctx, name, c)
	if err != nil {
		return err
	}
	if !need {
		return nil
	}

	_, err = c.AssetsUpload(ctx, name, neco.SabakanCryptsetupPath, nil)
	return err
}

func uploadSystemImageAssets(ctx context.Context, img neco.ContainerImage, c *sabakan.Client) error {
	name := neco.ACIAssetName(img)
	need, err := needAssetUpload(ctx, name, c)
	if err != nil {
		return err
	}
	if !need {
		return nil
	}

	_, err = c.AssetsUpload(ctx, name, neco.SystemImagePath(img), nil)
	return err
}

// UploadImageAssets upload docker container image as sabakan assets.
func UploadImageAssets(ctx context.Context, img neco.ContainerImage, c *sabakan.Client, auth *DockerAuth) error {
	name := neco.ImageAssetName(img)
	need, err := needAssetUpload(ctx, name, c)
	if err != nil {
		return err
	}
	if !need {
		return nil
	}

	d, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(d)

	archive := filepath.Join(d, name)
	err = fetchDockerImageAsArchive(ctx, img, archive, auth)
	if err != nil {
		return err
	}

	_, err = c.AssetsUpload(ctx, name, archive, nil)
	return err
}

func needAssetUpload(ctx context.Context, name string, c *sabakan.Client) (bool, error) {
	_, err := c.AssetsInfo(ctx, name)
	if err == nil {
		return false, nil
	}
	if sabakan.IsNotFound(err) {
		return true, nil
	}
	return false, err
}

// UploadIgnitions updates ignitions from local file
func UploadIgnitions(ctx context.Context, c *http.Client, id string, st storage.Storage) error {
	client, err := sabakan.NewClient(neco.SabakanLocalEndpoint, c)
	if err != nil {
		return err
	}

	return uploadIgnitions(ctx, client, id, st)
}

func uploadIgnitions(ctx context.Context, c *sabakan.Client, id string, st storage.Storage) error {
	roles, err := getInstalledRoles()
	if err != nil {
		return err
	}

	copyRoot, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer func() {
		os.RemoveAll(copyRoot)
	}()

	err = exec.Command("cp", "-a", neco.IgnitionDirectory, copyRoot).Run()
	if err != nil {
		return err
	}

	ckeImagePath := filepath.Join(copyRoot, "ignitions", "common", "systemd", "cke-images.service")
	data, err := ioutil.ReadFile(ckeImagePath)
	if err != nil {
		return err
	}

	ckeData, err := getCKEData()
	if err != nil {
		return err
	}
	replacer := strings.NewReplacer(
		"%%ETCD_FILE%%", ckeData["ETCD_FILE"],
		"%%ETCD_NAME%%", ckeData["ETCD_NAME"],
		"%%TOOLS_FILE%%", ckeData["CKE-TOOLS_FILE"],
		"%%TOOLS_NAME%%", ckeData["CKE-TOOLS_NAME"],
		"%%HYPERKUBE_FILE%%", ckeData["HYPERKUBE_FILE"],
		"%%HYPERKUBE_NAME%%", ckeData["HYPERKUBE_NAME"],
		"%%PAUSE_FILE%%", ckeData["PAUSE_FILE"],
		"%%PAUSE_NAME%%", ckeData["PAUSE_NAME"],
		"%%COREDNS_FILE%%", ckeData["COREDNS_FILE"],
		"%%COREDNS_NAME%%", ckeData["COREDNS_NAME"],
		"%%UNBOUND_FILE%%", ckeData["UNBOUND_FILE"],
		"%%UNBOUND_NAME%%", ckeData["UNBOUND_NAME"])
	err = ioutil.WriteFile(ckeImagePath, []byte(replacer.Replace(string(data))), 0644)
	if err != nil {
		return err
	}

	pubkey, err := st.GetSSHPubkey(ctx)
	switch err {
	case storage.ErrNotFound:
	case nil:
		passwdPath := filepath.Join(copyRoot, "ignitions", "common", "passwd.yml")
		data, err := ioutil.ReadFile(passwdPath)
		if err != nil {
			return err
		}
		passwd := new(ignition.Passwd)
		err = yaml.Unmarshal(data, passwd)
		if err != nil {
			return err
		}

		key := ignition.SSHAuthorizedKey(pubkey)
		for i := range passwd.Users {
			passwd.Users[i].SSHAuthorizedKeys = append(passwd.Users[i].SSHAuthorizedKeys, key)
		}

		data, err = yaml.Marshal(passwd)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(passwdPath, data, 0644)
		if err != nil {
			return err
		}
	default:
		return err
	}

	for _, role := range roles {
		path := filepath.Join(copyRoot, "ignitions", "roles", role, "site.yml")

		newer := new(bytes.Buffer)
		err := sabakan.AssembleIgnitionTemplate(path, newer)
		if err != nil {
			return err
		}

		need, err := needIgnitionUpdate(ctx, c, role, id, newer.String())
		if err != nil {
			return err
		}
		if !need {
			continue
		}
		err = c.IgnitionsSet(ctx, role, id, newer, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func needIgnitionUpdate(ctx context.Context, c *sabakan.Client, role, id string, newer string) (bool, error) {
	index, err := c.IgnitionsGet(ctx, role)
	if err != nil {
		if sabakan.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	latest := index[len(index)-1].ID
	if latest == id {
		return false, nil
	}

	current := new(bytes.Buffer)
	err = c.IgnitionsCat(ctx, role, latest, current)
	if err != nil {
		return false, err
	}
	return current.String() != newer, nil
}

func getInstalledRoles() ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(neco.IgnitionDirectory, "roles", "*", "site.yml"))
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		paths[i] = filepath.Base(filepath.Dir(path))
	}
	return paths, nil
}

func downloadFile(ctx context.Context, p *http.Client, url string, w io.Writer) (int64, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req = req.WithContext(ctx)
	resp, err := p.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.ContentLength <= 0 {
		return 0, errors.New("unknown content-length")
	}
	return io.Copy(w, resp.Body)
}

func getCKEData() (map[string]string, error) {
	output, err := exec.Command(neco.CKECLIBin, "images").Output()
	if err != nil {
		return nil, err
	}

	data := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(output))
	for sc.Scan() {
		img, err := neco.ParseContainerImageName(sc.Text())
		if err != nil {
			return nil, err
		}
		nameUpper := strings.ToUpper(img.Name)
		data[fmt.Sprintf("%s_FILE", nameUpper)] = neco.ImageAssetName(img)
		data[fmt.Sprintf("%s_NAME", nameUpper)] = img.FullName()
	}
	err = sc.Err()
	if err != nil {
		return nil, err
	}

	return data, nil
}
