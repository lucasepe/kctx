# kctx Helm Chart

This chart deploys `kctx serve` as a read-only HTTP server inside Kubernetes.

## Install

```bash
helm upgrade --install kctx ./chart --namespace kctx-system --create-namespace
```

Or render and apply without Helm release state:

```bash
helm template kctx ./chart \
  --namespace kctx-system \
  --set namespace.create=true \
  | kubectl apply -f -
```

## Install From Release Archive

Release builds can publish the packaged chart as `kctx-<version>.tgz`.

```bash
VERSION=1.2.3
CHART_URL="https://github.com/lucasepe/kctx/releases/download/v${VERSION}/kctx-${VERSION}.tgz"
```

Install with Helm:

```bash
helm upgrade --install kctx "${CHART_URL}" --namespace kctx-system --create-namespace
```

Or render and apply:

```bash
helm template kctx "${CHART_URL}" \
  --namespace kctx-system \
  --set namespace.create=true \
  | kubectl apply -f -
```

The packaged chart points to the matching image tag without the leading `v`.

For example, release `v1.2.3` uses:

```text
ghcr.io/lucasepe/kctx:1.2.3
```

## Kind Lab

Build and load the image first:

```bash
docker build --tag kctx:kind .
kind load docker-image kctx:kind --name kctx-lab
```

Install with a NodePort that matches `scripts/kind-up.sh`:

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

Or render and apply:

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

Then try:

```bash
curl http://localhost:8888/healthz
curl http://localhost:8888/version
curl http://localhost:8888/health/namespace/default
```

## Values

The chart exposes the environment variables supported by `kctx serve`:

```yaml
env:
  listenAddr: ":8080" # LISTEN_ADDR
  verbose: false      # VERBOSE
```

Other common values:

```yaml
image:
  repository: ghcr.io/lucasepe/kctx
  tag: ""
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080
  nodePort: null

rbac:
  create: true
  clusterWide: true

namespace:
  create: false
```

`rbac.clusterWide=true` is the default because `kctx` may read cluster-scoped resources such as Nodes while resolving Pod context and graphs.
