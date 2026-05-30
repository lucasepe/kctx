# kctx dump namespace

```bash
kctx dump namespace <namespace> --output json
```

`kctx dump namespace` exports a normalized operational snapshot of a Kubernetes namespace.

It answers:

> What is the current structured operational context of this namespace?

This command is JSON-only in the first version.

---

# What It Exports

The dump includes:

* Namespace
* Pods
* Deployments
* ReplicaSets
* StatefulSets
* DaemonSets
* Jobs
* CronJobs
* Services
* EndpointSlices
* ConfigMap metadata
* Secret metadata
* PVCs
* Nodes referenced by Pods
* recent Warning Events
* normalized relations
* factual operational signals

The output is a compact normalized model, not raw Kubernetes YAML.

---

# Why It Exists

Operational tools and AI agents should not need to parse raw manifests just to understand a namespace.

`dump namespace` creates a stable, machine-readable snapshot that can be used for:

* offline analysis
* incident reports
* troubleshooting automation
* AI agent context
* MCP tools
* future HTTP APIs
* support bundles

It is designed to be readable by humans, but optimized for machines.

---

# Useful For

This command is useful for:

* capturing incident context
* exporting namespace state for later analysis
* giving agents a safe, normalized cluster snapshot
* building automation around Kubernetes relationships
* comparing operational context across runs
* creating compact support artifacts

---

# Not Useful For

This command is not meant for:

* Kubernetes backups
* restoring resources
* exporting raw manifests
* replacing `kubectl get all`
* collecting logs
* collecting metrics
* streaming updates
* full cluster inventory
* CRD coverage

It is an operational context dump, not a backup format.

---

# Output Shape

The JSON is intentionally flat:

```json
{
  "generatedAt": "2026-05-23T12:00:00Z",
  "namespace": "payments",
  "summary": {},
  "entities": [],
  "relations": [],
  "signals": [],
  "events": []
}
```

Entities represent normalized Kubernetes objects.

Relations connect entities by deterministic IDs.

Signals report factual operational conditions.

---

# Entities

Entities are compact and normalized:

```json
{
  "id": "Pod/payments/api-xyz",
  "kind": "Pod",
  "namespace": "payments",
  "name": "api-xyz",
  "status": "CrashLoopBackOff",
  "ready": false,
  "restartCount": 12
}
```

The dump does not include:

* raw manifests
* managedFields
* full status blobs
* ownerReferences as raw metadata
* noisy Kubernetes internals

---

# Relations

Relations are normalized as ID-to-ID edges.

Examples:

* `owns`
* `selects`
* `scheduled_on`
* `mounts_pvc`
* `uses_secret`
* `uses_configmap`
* `has_endpoint`
* `endpoint_targets`
* `runs_on`

Example:

```json
{
  "type": "selects",
  "source": "Service/payments/payments-api",
  "target": "Pod/payments/api-xyz"
}
```

---

# Signals

Signals use machine-friendly codes:

* `pod_crashloop`
* `pod_not_ready`
* `image_pull_error`
* `workload_unavailable`
* `service_without_ready_endpoints`
* `pvc_pending`
* `warning_event_present`

Signals are factual.

They do not guess root causes.

---

# Secret Safety

Secrets are included as metadata only.

The dump never includes:

* Secret data
* token values
* passwords
* certificates
* binary content
* Secret data keys

Only metadata such as name, labels, UID, and Secret type are included.

---

# ConfigMap Safety

ConfigMaps are metadata-only in the first version.

The dump does not include ConfigMap data.

---

# Typical Usage

```bash
kctx dump namespace payments --output json > payments-dump.json
```

Use the resulting JSON for offline inspection, automation, support, or AI-agent context.

