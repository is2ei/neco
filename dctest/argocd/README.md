Setup Argo CD
=============

This procedure is based on https://github.com/argoproj/argo-cd/blob/master/docs/getting_started.md

## 0. Setup dctest

```console
@host-vm
cd dctest/
make setup
make placemat
make test-light
# Wait for 30 mins...
```

## 1. Install Argo CD

You can see difference from original manifestby:

```console
diff -u <(curl -sL https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml) install.yaml
```

Deploy Argo CD using `install.yaml`.

```console
@host-vm
./dcscp dctest/argocd/install.yaml boot-0:

@boot-0 
kubectl create namespace argocd
kubectl apply -f install.yaml
```

## 2. Install `argocd` CLI

```console
@boot-0 
curl -LO https://github.com/argoproj/argo-cd/releases/download/v0.11.0/argocd-linux-amd64 && sudo mv argocd-linux-amd64 /usr/local/bin/argocd && sudo chmod +x /usr/local/bin/argocd
```

## 3. Access the Argo CD API server

Change type of argocd-server to `NodePort`.

```console
@boot-0 
kubectl patch svc argocd-server -n argocd -p '{"spec": {"type": "NodePort"}}'
```

## 4. Login using the CLI

```console
@boot-0
$ kubectl get pods -n argocd -l app=argocd-server -o name | cut -d'/' -f 2
argocd-server-76d996445f-8wgfc # This is default admin password

$ argocd login <Node IP>:<Node Port>
WARNING: server certificate had error: x509: cannot validate certificate for 10.69.0.5 because it doesn't contain any IP SANs. Proceed insecurely (y/n)? y
Username: admin
Password:
'admin' logged in successfully
Context '10.69.0.5:30086** updated

### Update password
$ argocd account update-password
** Enter current password:
*** Enter new password:
*** Confirm new password:
Password updated
Context '10.69.0.5:30086' updated
```

## 5. Setup example app to deployment on your CD

1. Fork https://github.com/argoproj/argocd-example-apps
2. Create application in Argo CD.

    ```console
    cybozu@boot-0:~$ argocd app create guestbook --repo https://github.com/mitsutaka/argocd-example-apps --path guestbook --dest-server https://kubernetes.default.svc --dest-namespace default
    ```

3. If you need to deploy manually, run:

    ```console
    cybozu@boot-0:~$ argocd app sync guestbook
    ```

## 6. Setup notification for Slack

**TBD**
