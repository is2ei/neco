// Code generated by generate-artifacts. DO NOT EDIT.
// +build !release,!new

package neco

var CurrentArtifacts = ArtifactSet{
	Images: []ContainerImage{
		{Name: "cke", Repository: "quay.io/cybozu/cke", Tag: "0.11-1"},
		{Name: "etcd", Repository: "quay.io/cybozu/etcd", Tag: "3.3.9-2"},
		{Name: "omsa", Repository: "quay.io/cybozu/omsa", Tag: "18.08.00-3"},
		{Name: "sabakan", Repository: "quay.io/cybozu/sabakan", Tag: "0.27-1"},
		{Name: "serf", Repository: "quay.io/cybozu/serf", Tag: "0.8.1-3"},
		{Name: "vault", Repository: "quay.io/cybozu/vault", Tag: "0.11.0-1"},
		{Name: "hyperkube", Repository: "quay.io/cybozu/hyperkube", Tag: "1.11.2-5"},
	},
	Debs: []DebianPackage{
		{Name: "etcdpasswd", Owner: "cybozu-go", Repository: "etcdpasswd", Release: "v0.5"},
	},
	CoreOS: CoreOSImage{Channel: "stable", Version: "1855.5.0"},
}
