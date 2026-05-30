# kctx explain pod

```bash
kctx explain pod <pod-name> --namespace <namespace>
kctx explain pod <pod-name> --namespace <namespace> --output json
```

`kctx explain pod` resolves structured operational context around one Pod.

It answers:

> What is this Pod, what is connected to it, and what factual signals are visible?

This is the most direct troubleshooting command in `kctx`.

---

# What It Inspects

For a Pod, the command collects:

* Pod identity and labels
* Pod phase and readiness
* container states and restart counts
* waiting and terminated reasons
* owner chain, such as ReplicaSet and Deployment
* scheduled Node
* mounted ConfigMaps
* mounted Secrets
* mounted PVCs
* Services selecting the Pod
* recent Events involving the Pod
* factual operational signals

The output is normalized instead of raw Kubernetes YAML.

---

# Why It Exists

Pod troubleshooting usually starts with:

```bash
kubectl describe pod ...
kubectl get rs ...
kubectl get deploy ...
kubectl get svc ...
kubectl get events ...
```

Then the operator manually reconstructs relationships.

`explain pod` makes that reconstruction explicit.

It turns a Pod into a compact operational context object that can be read by humans or consumed by tools.

---

# Useful For

This command is useful for:

* quickly understanding why a Pod looks unhealthy
* finding the workload that owns a Pod
* seeing which Services select the Pod
* checking which ConfigMaps, Secrets, and PVCs the Pod depends on
* identifying CrashLoopBackOff and image pull problems
* feeding structured Pod context to scripts, AI agents, or MCP tools

---

# Not Useful For

This command is not meant for:

* reading Pod logs
* watching live Pod state
* debugging application-level behavior
* inspecting full raw manifests
* restarting or remediating workloads
* replacing `kubectl describe`

It gives context. It does not perform diagnosis beyond factual signals.

---

# Human Output

Human output is designed for quick terminal inspection:

```text
Pod payments/api-xyz

Status
  Phase: Running
  Ready: false
  Restarts: 12

Owners
  ReplicaSet payments/api-7d9f8
  Deployment payments/api

Selected by Services
  Service payments-api

Signals
  critical CrashLoopBackOff: container api is restarting repeatedly
```

---

# JSON Output

JSON output is intended for:

* shell scripts
* CI checks
* incident tooling
* AI agents
* MCP servers
* future APIs

Use:

```bash
kctx explain pod api-xyz --namespace payments --output json
```

The JSON includes structured entities, relations, events, and signals.

It does not include ANSI colors or prose-only fields.

---

# Signal Philosophy

Signals are factual.

Examples:

* `CrashLoopBackOff`
* `ImagePullBackOff`
* `ErrImagePull`
* `HighRestartCount`
* `NotReady`
* `FailedScheduling`

The command does not guess root causes.

It will say:

```text
container api is restarting repeatedly
```

It will not say:

```text
the database is down
```

---

# Typical Usage

```bash
kctx explain pod api-xyz --namespace payments
```

For automation:

```bash
kctx explain pod api-xyz --namespace payments --output json
```
