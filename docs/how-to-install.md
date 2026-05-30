# How To Install kctx

This guide covers two installation modes:

* installing the `kctx` CLI on your machine
* deploying `kctx serve` inside a Kubernetes cluster

The CLI is useful when you want to run commands from your terminal.

The in-cluster deployment is useful when you want a read-only HTTP endpoint that can be queried with `curl`, scripts, dashboards, agents, or other tools.

---

# Install The CLI

The repository includes an install script:

```bash
./install.sh
```

Without cloning the repository:

```bash
curl -fsSL https://raw.githubusercontent.com/lucasepe/kctx/main/install.sh | bash
```

By default, the script downloads the latest GitHub release for your operating system and architecture.

It installs the `kctx` binary into:

```text
/usr/local/bin
```

If that directory is not writable, it falls back to:

```text
$HOME/.local/bin
```

Make sure the install directory is in your `PATH`.

Verify the install:

```bash
kctx --help
```

---

# Install A Specific Version

Pass the version without the leading `v`:

```bash
./install.sh 1.2.3
```

Without cloning the repository:

```bash
curl -fsSL https://raw.githubusercontent.com/lucasepe/kctx/main/install.sh | bash -s -- 1.2.3
```

This downloads release tag:

```text
v1.2.3
```

---

# Run Locally

After installing the CLI, use your current Kubernetes context:

```bash
kctx health namespace default
```

Or start the HTTP server locally:

```bash
kctx serve
```

Then try:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/version
curl http://localhost:8080/health/namespace/default
```

---

# Deploy In A Cluster

`kctx serve` can run inside Kubernetes using the Helm chart in `chart/`.

The chart deploys:

* ServiceAccount
* read-only RBAC
* Deployment
* Service
* health probes

The chart exposes the environment variables supported by `kctx serve`:

```yaml
env:
  listenAddr: ":8080" # LISTEN_ADDR
  verbose: false      # VERBOSE
```

---

# Deploy Without Cloning The Repo

Release builds can include the packaged Helm chart as:

```text
kctx-<version>.tgz
```

You can use that chart archive directly from the GitHub Release.

Set the version:

```bash
VERSION=1.2.3
CHART_URL="https://github.com/lucasepe/kctx/releases/download/v${VERSION}/kctx-${VERSION}.tgz"
```

Render and apply:

```bash
helm template kctx "${CHART_URL}" \
  --namespace kctx-system \
  --set namespace.create=true \
  | kubectl apply -f -
```

Or install with Helm release state:

```bash
helm upgrade --install kctx "${CHART_URL}" \
  --namespace kctx-system \
  --create-namespace
```

The packaged chart points to the matching container image tag without the leading `v`.

For example, release `v1.2.3` uses:

```text
ghcr.io/lucasepe/kctx:1.2.3
```

---

# Option 1: Helm Template And Kubectl Apply

This is the lightest install path.

It renders Kubernetes manifests with Helm and applies them with `kubectl`, without storing Helm release state in the cluster.

```bash
helm template kctx ./chart \
  --namespace kctx-system \
  --set namespace.create=true \
  | kubectl apply -f -
```

Check the rollout:

```bash
kubectl rollout status deployment/kctx --namespace kctx-system
```

Port-forward locally:

```bash
kubectl port-forward --namespace kctx-system svc/kctx 8080:8080
```

Then try:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/version
curl http://localhost:8080/health/namespace/default
```

To remove it:

```bash
helm template kctx ./chart \
  --namespace kctx-system \
  --set namespace.create=true \
  | kubectl delete -f -
```

---

# Option 2: Helm Install

Use this when you want Helm to manage release state.

```bash
helm upgrade --install kctx ./chart \
  --namespace kctx-system \
  --create-namespace
```

Check the rollout:

```bash
kubectl rollout status deployment/kctx --namespace kctx-system
```

Port-forward locally:

```bash
kubectl port-forward --namespace kctx-system svc/kctx 8080:8080
```

Then try:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/version
curl http://localhost:8080/health/namespace/default
```

To remove it:

```bash
helm uninstall kctx --namespace kctx-system
```

---

# Kind Lab

For local testing with kind, create the cluster:

```bash
scripts/kind-up.sh
```

Build and load the local image:

```bash
docker build --tag kctx:kind .
kind load docker-image kctx:kind --name kctx-lab
```

Install with Helm template and `kubectl apply`:

```bash
helm template kctx ./chart \
  --namespace kctx-system \
  --set namespace.create=true \
  --set image.repository=kctx \
  --set image.tag=kind \
  --set image.pullPolicy=Never \
  --set service.type=NodePort \
  --set service.nodePort=30088 \
  | kubectl apply -f -
```

Or install with Helm release state:

```bash
helm upgrade --install kctx ./chart \
  --namespace kctx-system \
  --create-namespace \
  --set image.repository=kctx \
  --set image.tag=kind \
  --set image.pullPolicy=Never \
  --set service.type=NodePort \
  --set service.nodePort=30088
```

The default kind lab maps:

```text
localhost:8888 -> NodePort 30088
```

Try:

```bash
curl http://localhost:8888/healthz
curl http://localhost:8888/version
curl http://localhost:8888/health/namespace/default
```

You can also use the convenience deploy script:

```bash
scripts/deploy-kctx-serve.sh
```

---

# Enable Debug Logging

For local CLI server mode:

```bash
VERBOSE=true kctx serve
```

For the Helm chart:

```bash
helm upgrade --install kctx ./chart \
  --namespace kctx-system \
  --create-namespace \
  --set env.verbose=true
```

With `helm template`:

```bash
helm template kctx ./chart \
  --namespace kctx-system \
  --set namespace.create=true \
  --set env.verbose=true \
  | kubectl apply -f -
```

---

# Cleanup

Delete the kind cluster:

```bash
scripts/kind-down.sh
```
