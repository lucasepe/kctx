# kctx Roadmap

This roadmap tracks the work needed to move `kctx` from a useful operational
prototype to a production-ready context engine that can be safely exposed to
humans, automation, and AI SRE agents.

## Production Readiness Checklist

- [x] Define and version the JSON output contract.
- [x] Add structured error responses and stable error codes.
- [ ] Harden `kctx serve` authentication and authorization.
- [x] Document and enforce a redaction/data-safety policy.
- [x] Add end-to-end tests with kind and golden outputs.
- [x] Add request timeouts, limits, and performance safeguards.
- [x] Add server observability: logs, request IDs, latency, health, metrics.
- [ ] Harden packaging, container image, Helm chart, and release flow.
- [x] Mature CRD adapter support beyond the first ArgoCD adapter.
- [ ] Add an agent-friendly API surface, ideally MCP.
- [ ] Publish production deployment guidance. (Partial: install, serve, ArgoCD, adapter, and roadmap docs exist; production hardening guidance is still incomplete.)

## Roadmap Status Conventions

Use checklist items for production-readiness completion, not for activity.

- `[x]` means the item is complete enough to rely on for production beta.
- `[ ]` means the item is still open.
- `Partial:` in the checklist means useful implementation work exists, but the
  item is not production-ready yet.
- Each numbered section should include a `Status:` line so the checklist can
  stay short while the details remain easy to review.

Avoid strikethrough for roadmap sections. Completed sections should stay
readable as historical design intent and acceptance criteria. When an item is
fully done, mark the checklist entry with `[x]` and change the section status to
`Complete`, optionally adding a short implementation note.

## Sustainability

Sponsorships and donations support ongoing maintenance work that is easy to
underestimate but important for a tool like `kctx`: release automation,
Kubernetes compatibility testing, adapter maintenance, documentation, issue
triage, and production-hardening work.

Funding does not imply a paid support contract, an SLA, or priority access to
private operational advice. Those can be introduced later if the project grows
enough to justify them.

## 1. Versioned JSON Contract

Status: **Complete.** `schemaVersion: "kctx.io/v1alpha1"` and top-level `kind`
are emitted by the main success and error responses. CLI and HTTP outputs are
JSON by default, signal severities are normalized to `info`, `warning`, and
`error`, and signal fields use `reason` consistently. The contract is documented
in `book/11-json-contract.md`, machine-readable schemas live under
`schemas/kctx.io/v1alpha1`, and golden outputs are checked by
`scripts/validate-json-contract.sh`.

`kctx` exposes a stable, documented JSON contract before other tools or agents
depend on it.

Schema metadata is present on top-level responses:

```json
{
  "schemaVersion": "kctx.io/v1alpha1",
  "kind": "ResourceContext"
}
```

Documented core model types:

- `Entity`
- `Relation`
- `Signal`
- `ResolveContextResponse`
- `ResolveResourceContextResponse`
- `NamespaceHealthResponse`
- `DumpNamespaceResponse`
- `ServiceTraceResponse`
- graph responses

The contract specifies:

- required fields
- optional fields
- enum-like values such as signal severities and relation types
- empty list behavior
- ID format
- timestamp format
- compatibility rules between schema versions

Current implementation notes:

- CLI commands emit JSON by default and no longer expose `--output`.
- HTTP routes emit JSON and no longer expose `format`.
- Success envelopes include `schemaVersion` and `kind`.
- Error envelopes include `schemaVersion`, `kind: "Error"`, and `error.code`.
- Empty slices are normalized to `[]` in the main response types.
- `critical` severity has been replaced with `error`.
- `DumpSignal.code` has been renamed to `DumpSignal.reason`.
- `book/11-json-contract.md` documents compatibility rules, empty-array behavior,
  timestamp handling, ID format, response kinds, and the error envelope.
- `schemas/kctx.io/v1alpha1/*.schema.json` provides machine-readable contract
  documents for the supported response families.
- `internal/contract/validate` validates envelope fields, response kinds,
  required arrays, error shape, and signal severity enums.
- `scripts/e2e-golden.sh` runs contract validation after golden comparison.

This is especially important for AI SRE agents. A stable contract lets agents
and MCP tools treat `kctx` output as structured evidence instead of best-effort
text.

## 2. Structured Error Model

Status: **Complete.** CLI and HTTP errors use a JSON envelope with
`schemaVersion`, `kind: "Error"`, stable `error.code`, required
`error.message`, and optional `error.details`. Error classification covers bad
requests, not found, forbidden, unsupported resources, method not allowed,
timeouts, request limits, canceled requests, and internal errors.

Errors should be machine-readable in both CLI and HTTP contexts.

Stable error codes:

- `bad_request`
- `not_found`
- `forbidden`
- `method_not_allowed`
- `unsupported_resource`
- `timeout`
- `limit_exceeded`
- `client_closed_request`
- `internal_error`

HTTP errors should use a predictable shape:

```json
{
  "schemaVersion": "kctx.io/v1alpha1",
  "kind": "Error",
  "error": {
    "code": "unsupported_resource",
    "message": "resource argoproj.io/v1alpha1/Application is known to Kubernetes, but kctx has no semantic adapter yet",
    "details": {
      "resource": "applications.argoproj.io",
      "apiVersion": "argoproj.io/v1alpha1",
      "kind": "Application"
    }
  }
}
```

CLI errors can remain human-readable, but should still be backed by typed errors
internally so callers and tests can reason about them.

Current implementation notes:

- CLI JSON errors are written to stdout.
- Diagnostic command logs are still written to stderr.
- HTTP errors use the same envelope shape.
- Unsupported CRDs include `resource`, `apiVersion`, and `kind` in
  `error.details`.
- Error details are represented as an object with string values so future
  classifiers can add fields such as `namespace` and `name` without changing the
  top-level shape.
- `schemas/kctx.io/v1alpha1/error.schema.json` defines the error envelope and
  stable code enum.
- `internal/contract/validate` rejects undocumented error codes in golden
  outputs.

## 3. Authentication And Authorization

Status: **Not started.**

If `kctx serve` is exposed inside a cluster, it must be treated as a read-only
Kubernetes intelligence endpoint.

Minimum requirements:

- dedicated ServiceAccount
- least-privilege RBAC
- read-only permissions only
- no access to Secret data
- configurable namespace allowlist
- safe defaults in the Helm chart
- clear documentation for in-cluster deployment

Possible authentication strategies:

- put `kctx` behind an ingress or API gateway with auth
- use mTLS at the mesh or gateway layer
- support a bearer token for direct internal use
- rely on cluster-local network boundaries only for development, not production

The service should not accidentally become a convenient cluster-wide discovery
proxy.

## 4. Redaction And Data Safety

Status: **Complete.** Supported CLI and HTTP response paths avoid raw Secret
data, ConfigMap data, raw manifests, raw environment variables, logs, and
metrics. Metadata maps and Kubernetes-derived messages pass through the
centralized `internal/redaction` package, and the policy is documented in
`book/12-data-safety.md`.

`kctx` is intended to provide operational context, not sensitive data.

The redaction policy should be explicit and enforced.

Rules:

- never include Secret `.data`
- never include Secret `.stringData`
- avoid exposing raw environment variables
- avoid exposing full Pod specs when not needed
- be careful with annotations and labels that may contain tokens, URLs, emails,
  or internal hostnames
- avoid logs unless a future feature has a clear redaction layer

Implementation:

- always-on redaction for known sensitive fields
- explicit sensitive-key matching for metadata maps
- conservative message redaction for common `key=value`, `key: value`, and
  `Bearer ...` patterns
- tests for Secret handling
- tests for ConfigMap data exclusion
- tests for label and message redaction
- tests for Pod status message redaction
- tests for Argo CD adapter message redaction
- documentation explaining what `kctx` does and does not collect

For AI-agent usage, this is a production blocker. Agent context should stay
useful without becoming a data exfiltration path. Future response fields and CRD
adapters must use the same redaction package before exposing metadata maps or
user-controlled status text.

## 5. End-To-End Tests And Golden Outputs

Status: **Complete.** Capture scripts still exist for exploratory labs, and a
deterministic kind golden suite now verifies normalized JSON outputs under
`testdata/e2e/golden`.

Unit tests are not enough for production readiness. `kctx` should have a
repeatable end-to-end test suite using kind.

Covered golden scenarios:

- empty namespace
- unhealthy namespace health
- namespace dump with Secret and ConfigMap payloads present but excluded
- Pod explain for a fixed bad-image Pod
- Service without ready endpoints
- PVC pending
- recent Warning events
- RBAC forbidden
- resource not found

Exploratory capture scenarios:

- ArgoCD installed with no Applications
- ArgoCD Application healthy and synced
- ArgoCD Application out of sync
- unsupported CRD
- Online Boutique namespace health and dump

Golden workflow:

```bash
scripts/kind-up.sh
scripts/e2e-golden.sh
```

Regenerate golden files only when the JSON contract intentionally changes:

```bash
UPDATE_GOLDEN=1 scripts/e2e-golden.sh
```

Exploratory capture workflow:

```bash
scripts/kind-up.sh
scripts/install-online-boutique.sh
scripts/install-argocd.sh
scripts/create-argocd-application.sh
scripts/e2e-all.sh
```

Golden output tests should compare normalized JSON, while ignoring expected
volatile fields such as:

- timestamps
- UIDs
- generated EndpointSlice suffixes
- Kubernetes image-pull Event message variants
- list ordering differences after normalization

The goal is to prove that the contract stays stable while implementation details
evolve.

Current implementation notes:

- `scripts/e2e-golden.sh` creates deterministic kind fixtures, captures core
  outputs, normalizes volatile JSON fields, and diffs them against
  `testdata/e2e/golden`.
- `UPDATE_GOLDEN=1 scripts/e2e-golden.sh` refreshes golden data.
- `scripts/e2e-golden-cleanup.sh` removes the golden fixture namespaces and
  optional outputs.
- `internal/e2e/normalize` canonicalizes JSON for golden comparisons.
- `scripts/e2e-argocd.sh` captures healthy ArgoCD Application context,
  AppProject policy context, target namespace health, and target namespace dump.
- `scripts/e2e-unhealthy.sh` creates image pull, Service endpoint, and PVC
  pending scenarios.
- `scripts/e2e-errors.sh` captures not found, unsupported CRD, and forbidden
  RBAC behavior.
- `scripts/e2e-all.sh` runs all captures.
- `scripts/e2e-cleanup.sh` removes temporary namespaces and optional outputs.

## 6. Timeouts, Limits, And Performance Safeguards

Status: **Complete.** HTTP request timeout defaults, `kctx serve`
configuration, HTTP timeout error handling, server-side Event limits,
Kubernetes API call budgeting, socket-level HTTP timeouts, clear
`timeout`/`limit_exceeded` error behavior, Helm values, and performance guidance
are implemented.

`kctx` is currently appropriate for small and medium namespaces. Production use
needs safeguards for large namespaces and repeated calls.

Add:

- request timeout defaults (done for `kctx serve`)
- CLI timeout flag (done for `kctx serve`)
- HTTP timeout handling (done)
- event limits (done for HTTP `eventLimit`)
- maximum namespace size guidance (done in `book/16-performance-limits-metrics.md`)
- Kubernetes API call budgeting (done for `kctx serve`)
- socket-level HTTP server timeouts (done)
- Helm values and deployment script configuration (done)
- clear error behavior when limits are exceeded (done for timeouts, event limits, and API budget)

Potential future work:

- discovery cache or RESTMapper reuse
- optional response size limits
- cache discovery information
- parallelize independent list calls carefully
- expose partial results with warnings
- add namespace-level sampling or filters
- dedicated timeout and Kubernetes API error metrics

The tool should fail clearly rather than hang or overload the Kubernetes API.

## 7. Server Observability

Status: **Done.** Structured request logs, request IDs, latency fields,
`/livez`, `/readyz`, and lightweight JSON metrics are implemented. Request IDs
are emitted on responses, added to structured logs, and propagated to Kubernetes
client calls. Metric design and monitoring guidance are documented in
`METRICS.md`.

If `kctx serve` runs in a cluster, teams need to observe `kctx` itself.

Add:

- structured request logs (done)
- request ID (done)
- method/path/status/latency fields (done)
- Kubernetes error classification (future hardening)
- readiness endpoint (done)
- liveness endpoint (done)
- lightweight JSON metrics (done)
- metrics documentation (done)

Useful metrics:

- request count by endpoint and status (done)
- request latency histogram (done)
- Kubernetes API error count (future hardening)
- unsupported resource count (future hardening)
- response size (done)
- timeout count (future hardening)

This is not about monitoring workloads through `kctx`; it is about operating
`kctx` safely.

## 8. Packaging, Helm, And Releases

Status: **Partial.** A Dockerfile, Helm chart, and install/deploy scripts exist.
The release flow, chart hardening, versioning, RBAC defaults, and production
values still need work.

Production exposure requires repeatable packaging.

Needed:

- versioned container image
- multi-arch builds if needed
- release tags
- changelog
- version injection into the binary
- signed or checksummed artifacts
- Helm chart defaults reviewed for safety
- minimal RBAC templates
- namespace allowlist values
- resource requests and limits
- probes
- example values for local, internal, and restricted deployments

The Helm chart should make the safe deployment path the easy path.

## 9. CRD Adapter Maturity

Status: **Complete.** The adapter layer now covers more than the first ArgoCD
adapter and proves the pattern across GitOps policy and certificate management
CRDs.

The supported adapters are deliberately narrow and semantic:

- ArgoCD `Application`
- ArgoCD `AppProject`
- cert-manager `Certificate`

Future candidate adapters:

- Flux `Kustomization`
- Flux `HelmRelease`
- External Secrets `ExternalSecret`
- Gateway API `Gateway` and `HTTPRoute`
- Crossplane managed resources

For each adapter:

- use `unstructured.Unstructured`
- avoid third-party CRD Go dependencies
- expose compact `status`
- use `related` for semantic resources
- use `relations` for explainable links
- emit factual `signals`
- add tests for healthy and unhealthy states

Current implementation notes:

- ArgoCD `Application` context adapter is registered by default.
- ArgoCD `Application` graph adapter supports JSON graph, Mermaid render, and
  DOT render through `kctx graph`.
- ArgoCD `Application` now links to its `AppProject` through `spec.project`.
- ArgoCD `AppProject` exposes allowed source repositories, namespaces, and
  clusters as policy relations.
- cert-manager `Certificate` links to its target `Secret` and issuer, and emits
  readiness and expiration signals.
- Adapter implementations depend on `unstructured.Unstructured`, not third-party
  operator Go packages.
- `ApplicationSet` remains a future adapter candidate, not part of the v1.0.0
  readiness bar.

The goal is not broad CRD inventory. The goal is meaningful operational
interpretation.

## 10. Agent-Friendly API Surface

Status: **Partial.** The JSON-only CLI/API contract is now much more
agent-friendly, but MCP tools have not been implemented.

The HTTP API is useful, but AI agents benefit from a tool-oriented API.

Potential MCP tools:

- `get_namespace_health(namespace)`
- `dump_namespace(namespace)`
- `explain_resource(resource, namespace, name)`
- `trace_service(namespace, name)`
- `graph_resource(resource, namespace, name, render)`

Design goals:

- small input schemas
- stable output schemas
- no raw YAML by default
- deterministic responses
- explicit error codes
- no speculative root-cause inference

MCP support would make `kctx` easier to plug into AI SRE workflows without
custom wrappers.

## 11. Production Deployment Guidance

Status: **Partial.** Installation, serve, ArgoCD lab, adapter authoring, and
roadmap docs exist. Production security guidance, redaction guarantees, and
safe exposure patterns still need to be written.

Documentation should explain how to deploy and expose `kctx` safely.

Include:

- local CLI usage
- in-cluster server usage
- Helm installation
- RBAC model
- namespace scoping
- redaction guarantees
- known limitations
- example ingress/gateway setup
- example agent integration
- troubleshooting guide

The docs should clearly distinguish:

- development lab usage
- internal production usage
- unsupported public exposure

## Production Ready Beta Definition

`kctx` can be considered production-ready beta when the following are complete:

- schema versioning is documented and stable
- structured errors are documented and stable
- server RBAC is least-privilege by default
- redaction policy is documented and tested
- kind e2e tests cover healthy and unhealthy scenarios
- request timeouts and limits exist
- server observability exists
- container and Helm release flow is repeatable
- at least one CRD adapter is production-tested
- deployment docs explain safe exposure

At that point, `kctx` should be safe to expose as an internal read-only service
for platform teams and controlled AI SRE agents.
