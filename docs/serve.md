# kctx serve

```bash
kctx serve
kctx serve --listen :9090
kctx serve --verbose
```

`kctx serve` exposes `kctx` as a lightweight read-only HTTP server.

It answers:

> Can the same Kubernetes context engine be queried over HTTP?

The server mirrors the CLI command surface. It is not a Kubernetes REST proxy.

---

# What It Exposes

The HTTP server exposes explicit operational routes:

* `GET /healthz`
* `GET /version`
* `GET /context/pod/{namespace}/{name}`
* `GET /graph/pod/{namespace}/{name}`
* `GET /trace/service/{namespace}/{name}`
* `GET /health/namespace/{namespace}`
* `GET /dump/namespace/{namespace}`

Routes that support multiple output formats accept the same `format` values used by the CLI output modes.

Examples:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/context/pod/payments/api-xyz
curl "http://localhost:8080/graph/pod/payments/api-xyz?format=mermaid"
curl http://localhost:8080/trace/service/payments/payments-api
curl http://localhost:8080/health/namespace/payments
curl http://localhost:8080/dump/namespace/payments
```

---

# Why It Exists

The CLI is useful for humans at a terminal.

The HTTP server is useful when `kctx` needs to be called by:

* scripts
* local tools
* dashboards
* AI agents
* MCP servers
* integration tests

It keeps the same philosophy as the CLI:

* read-only
* explicit routes
* structured outputs
* no arbitrary Kubernetes REST passthrough
* no mutation or remediation actions

---

# Configuration

The server listens on `:8080` by default.

Use a flag:

```bash
kctx serve --listen :9090
```

Or an environment variable:

```bash
LISTEN_ADDR=:9090 kctx serve
```

Enable debug logging with:

```bash
kctx serve --verbose
```

Or:

```bash
VERBOSE=true kctx serve
```

Logs are written as structured JSON to stderr.

---

# Local Usage

Run the server against your current Kubernetes context:

```bash
kctx serve
```

Then query it:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/version
curl http://localhost:8080/health/namespace/default
```

---

# Testing With Kind

The repository includes scripts for a local kind lab.

Create or reuse the kind cluster:

```bash
scripts/kind-up.sh
```

This creates a cluster named `kctx-lab` by default and maps:

```text
localhost:8888 -> NodePort 30088
```

Deploy `kctx serve` into that cluster:

```bash
scripts/deploy-kctx-serve.sh
```

The deploy script:

* builds the Docker image
* loads it into the kind cluster
* creates a service account
* grants read-only Kubernetes permissions
* installs a Deployment
* exposes it through a NodePort Service

Try:

```bash
curl http://localhost:8888/healthz
curl http://localhost:8888/version
curl http://localhost:8888/health/namespace/default
```

---

# Testing With Online Boutique

To install a sample workload:

```bash
scripts/install-online-boutique.sh
```

Then query the server:

```bash
curl http://localhost:8888/health/namespace/boutique
curl http://localhost:8888/trace/service/boutique/frontend
```

Find a Pod name:

```bash
kubectl get pods --namespace boutique
```

Then query Pod context and graph:

```bash
curl http://localhost:8888/context/pod/boutique/<pod-name>
curl "http://localhost:8888/graph/pod/boutique/<pod-name>?format=mermaid"
```

---

# Script Defaults

The lab scripts can be customized with environment variables:

```bash
CLUSTER_NAME=kctx-lab
HOST_PORT=8888
NODE_PORT=30088
NAMESPACE=kctx-system
IMAGE=kctx:kind
VERBOSE=false
```

Example:

```bash
VERBOSE=true HOST_PORT=9090 NODE_PORT=30090 scripts/kind-up.sh
VERBOSE=true HOST_PORT=9090 NODE_PORT=30090 scripts/deploy-kctx-serve.sh
curl http://localhost:9090/healthz
```

Use the same `HOST_PORT` and `NODE_PORT` values for both `kind-up.sh` and `deploy-kctx-serve.sh`.

---

# Deploying With Helm

The repository also includes a Helm chart in `chart/`.

Install with the default `ClusterIP` Service:

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

For the kind lab, build and load the local image first:

```bash
docker build --tag kctx:kind .
kind load docker-image kctx:kind --name kctx-lab
```

Then expose it through the NodePort mapped by `scripts/kind-up.sh`:

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

The same install can be done without Helm release state:

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

Try:

```bash
curl http://localhost:8888/healthz
```

The chart exposes the `kctx serve` environment variables under:

```yaml
env:
  listenAddr: ":8080"
  verbose: false
```

---

# Cleanup

Delete the kind cluster:

```bash
scripts/kind-down.sh
```
