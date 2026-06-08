# kctx

> A small read-only Kubernetes context engine for humans, scripts, and AI agents.

`kctx` turns Kubernetes API state into structured operational context.

Instead of asking every operator, script, dashboard, or agent to reconstruct relationships from raw YAML, `kctx` exposes a compact model of:

- **entities**: Pods, Services, workloads, Nodes, PVCs, ConfigMaps, Secrets, CRDs
- **relations**: ownership, selection, scheduling, service routing, dependencies
- **signals**: factual observations such as unhealthy Pods, missing endpoints,
  warning Events, failed readiness, or degraded workloads
- **graphs**: dependency and ownership views around supported resources
- **dumps**: deterministic namespace snapshots for machines and incident review

The tool is intentionally conservative: it reads cluster state, normalizes facts, and avoids speculative root-cause claims.

The motivation, philosophy, and design argument behind `kctx` are covered in longer form here:

- Leanpub book: [**Kubernetes Context Engineering**](https://leanpub.com/kubernetes-context-engineering)


## What It Is For

Use `kctx` when you want to answer questions like:

- What owns this Pod?
- Which Services route to these backends?
- Why does this namespace look unhealthy?
- What resources and signals should I attach to an incident?
- What compact Kubernetes context should an AI agent receive before reasoning?
- What does this supported CRD mean operationally right now?

`kctx` is useful for SREs, platform teams, Kubernetes operators, CI/CD
diagnostics, internal tooling, MCP tools, and AI SRE workflows.

## What It Is Not

`kctx` is not:

- a monitoring platform
- a logging or metrics system
- a dashboard suite
- a remediation engine
- a Kubernetes cluster manager
- a graph database
- an AI assistant
- a replacement for `kubectl`

It does not mutate resources, restart workloads, apply manifests, or guess root cause.

## Data Safety

`kctx` avoids raw manifests, Secret data, ConfigMap data, raw environment variables, logs, and workload metrics. Metadata and Kubernetes messages that are returned by supported outputs pass through a small redaction policy for common secret-bearing keys and text patterns.

## Quick Start

Install the CLI:

```bash
./install.sh
```

Or install from the published release script:

```bash
curl -fsSL https://raw.githubusercontent.com/lucasepe/kctx/main/install.sh | bash
```

Then run it against your current Kubernetes context:

```bash
kctx health namespace default
kctx explain pod <pod-name> --namespace default
kctx trace service <service-name> --namespace default
kctx graph pod <pod-name> --namespace default
kctx dump namespace default
```

For local development:

```bash
go run . health namespace default
```

### Install `kctx serve` With Helm

Install the in-cluster read-only HTTP server from a packaged release chart:

```bash
VERSION=0.2.0
helm upgrade --install kctx \
  "https://github.com/lucasepe/kctx/releases/download/v${VERSION}/kctx-${VERSION}.tgz" \
  --namespace kctx-system \
  --create-namespace
```

Then expose it locally:

```bash
kubectl -n kctx-system port-forward svc/kctx 8080:8080
curl http://localhost:8080/health/namespace/default
```

From a source checkout, install the local chart and choose the image tag to run:

```bash
helm upgrade --install kctx ./chart \
  --namespace kctx-system \
  --create-namespace \
  --set image.tag=dev
```

See [chart/README.md](chart/README.md) for chart values, local kind setup, and NodePort examples.

## Commands

`kctx explain`

Resolve structured context around one resource. Native Pod context is supported, and registered CRD adapters can provide ecosystem-specific context.

```bash
kctx explain pod api-xyz --namespace payments
kctx explain applications.argoproj.io guestbook --namespace argocd
```

`kctx graph`

Build a graph around a supported resource. JSON is the default output; Mermaid and DOT renderers are available for graph-oriented views.

```bash
kctx graph pod api-xyz --namespace payments
kctx graph pod api-xyz --namespace payments --render mermaid
kctx graph applications.argoproj.io guestbook --namespace argocd --render dot
```

`kctx trace service`

Trace a Service to EndpointSlices, endpoints, Pods, owners, Nodes, and factual service health signals.

```bash
kctx trace service payments-api --namespace payments
```

`kctx health namespace`

Produce a compact namespace health snapshot.

```bash
kctx health namespace payments
```

`kctx dump namespace`

Export a deterministic namespace context snapshot for automation, incident review, or AI-agent grounding.

```bash
kctx dump namespace payments > payments-dump.json
```

`kctx serve`

Expose the same context engine through a lightweight read-only HTTP API.

```bash
kctx serve
curl http://localhost:8080/health/namespace/default
```

## CRD Adapters

`kctx` can fetch arbitrary Kubernetes resources through discovery, but it does not pretend that every custom resource can be understood generically.

Ecosystem-specific semantics belong in explicit adapters. An adapter can turn a CRD into the same core contract used everywhere else: resource identity, compact status, related entities, relations, signals, and optionally graph nodes and edges.

The current adapter set covers Argo CD `Application`, Argo CD `AppProject`, and cert-manager `Certificate` resources.

## Documentation

The long-form documentation is organized as a [PDF eBook](docs/kctx.pdf).

## Support

If `kctx` saves you time when debugging Kubernetes workloads, consider supporting
its maintenance:

- GitHub Sponsors: <https://github.com/sponsors/lucasepe>
- PayPal: <https://paypal.me/lucasepe71>

Support helps fund release work, compatibility testing, documentation, and
ongoing Kubernetes, Argo CD, and cert-manager integration maintenance.

## Project Status

`kctx` is under active development. It is already useful as a read-only Kubernetes context tool, but production hardening is still in progress.

See [ROADMAP.md](ROADMAP.md) for current production-readiness work, planned features, and open design areas.
