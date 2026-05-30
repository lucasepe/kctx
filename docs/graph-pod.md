# kctx graph pod

```bash
kctx graph pod <pod-name> --namespace <namespace>
kctx graph pod <pod-name> --namespace <namespace> --output json
kctx graph pod <pod-name> --namespace <namespace> --output mermaid
kctx graph pod <pod-name> --namespace <namespace> --output dot
```

`kctx graph pod` builds a structured dependency and ownership graph around one Pod.

It answers:

> How is this Pod connected to the cluster?

This is not a live visualization system. It is an in-memory graph built from Kubernetes object relationships.

---

# What It Inspects

For a Pod, the graph can include:

* Pod
* owner chain, such as ReplicaSet and Deployment
* StatefulSet, DaemonSet, Job, and CronJob owners where applicable
* Node where the Pod runs
* Services selecting the Pod
* ConfigMaps used by the Pod
* Secrets used by the Pod
* PVCs mounted by the Pod

Only directly related objects are included.

The command does not crawl the entire cluster.

---

# Why It Exists

Kubernetes relationships are often implicit:

```text
Deployment owns ReplicaSet
ReplicaSet owns Pod
Service selects Pod
Pod mounts PVC
Pod uses Secret
Pod runs on Node
```

`graph pod` makes those relationships explicit.

It provides a graph that can be read in a terminal, serialized as JSON, or rendered later with Mermaid or Graphviz.

---

# Useful For

This command is useful for:

* understanding ownership around a Pod
* seeing direct runtime dependencies
* exporting a small graph for documentation or incident review
* giving AI agents a topology-like view without raw YAML
* generating Mermaid or DOT diagrams
* building higher-level graph tooling later

---

# Not Useful For

This command is not meant for:

* full-cluster graph discovery
* live topology visualization
* force-directed layouts
* graph database storage
* network packet tracing
* service mesh tracing
* dependency inference beyond Kubernetes references

It shows Kubernetes object relationships that can be observed directly.

---

# Human Output

Human output is a compact relationship tree:

```text
Pod/payments/api-xyz
├── owned by ReplicaSet/payments/api-7d9f8
│   └── owned by Deployment/payments/api
├── scheduled on Node/worker-01
├── selected by Service/payments/payments-api
├── uses ConfigMap/payments/api-config
├── uses Secret/payments/db-credentials
└── mounts PersistentVolumeClaim/payments/api-data
```

---

# JSON Output

JSON output contains a generic graph model:

```json
{
  "nodes": [],
  "edges": []
}
```

Use:

```bash
kctx graph pod api-xyz --namespace payments --output json
```

This is useful for scripts, agents, MCP tools, and future APIs.

---

# Mermaid Output

Use:

```bash
kctx graph pod api-xyz --namespace payments --output mermaid
```

This emits a simple Mermaid graph:

```text
graph TD
  Deployment_payments_api[Deployment api]
  ReplicaSet_payments_api_7d9f8[ReplicaSet api-7d9f8]
  Pod_payments_api_xyz[Pod api-xyz]

  Deployment_payments_api -->|owns| ReplicaSet_payments_api_7d9f8
  ReplicaSet_payments_api_7d9f8 -->|owns| Pod_payments_api_xyz
```

---

# DOT Output

Use:

```bash
kctx graph pod api-xyz --namespace payments --output dot
```

This emits Graphviz DOT.

---

# Relationship Philosophy

Edges are factual and deterministic.

Examples:

* `owns`
* `selects`
* `scheduled_on`
* `uses_configmap`
* `uses_secret`
* `mounts_pvc`

The command does not infer hidden dependencies.

