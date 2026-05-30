# kctx health namespace

```bash
kctx health namespace <namespace>
kctx health namespace <namespace> --output json
```

`kctx health namespace` produces a factual health snapshot for one namespace.

It answers:

> What looks unhealthy in this namespace right now?

This is not monitoring. It is a read-only point-in-time summary.

---

# What It Inspects

The command collects:

* Pods
* Deployments
* ReplicaSets
* StatefulSets
* DaemonSets
* Jobs
* CronJobs
* Services
* EndpointSlices
* PVCs
* Warning Events

It summarizes readiness, availability, endpoint health, PVC state, and recent warnings.

---

# Why It Exists

Namespace-level troubleshooting often starts with broad questions:

* How many Pods are not Ready?
* Which workloads are unavailable?
* Which Services have no ready endpoints?
* Are PVCs stuck Pending?
* Are there recent Warning Events?

`health namespace` collects those signals into one compact report.

---

# Useful For

This command is useful for:

* quick namespace triage
* deployment validation
* CI/CD smoke checks
* incident snapshots
* detecting obvious unhealthy Kubernetes state
* feeding summarized namespace health into automation or agents

---

# Not Useful For

This command is not meant for:

* continuous monitoring
* historical analysis
* metrics-based alerting
* log analysis
* root cause inference
* remediation
* cross-namespace analysis

It is a snapshot, not an observability platform.

---

# Human Output

Human output is designed for fast scanning:

```text
Namespace payments

Summary
  Pods:       12 total, 10 ready, 2 not ready
  Workloads:  5 total, 4 healthy, 1 unhealthy
  Services:   3 total, 1 without ready endpoints
  PVCs:       4 total, 1 pending
  Events:     7 recent warnings

Top Signals
  critical pod_crashloop: Pod payments/api-abc12 is in CrashLoopBackOff
  warning service_without_ready_endpoints: Service payments-api has no ready endpoints
```

---

# JSON Output

Use:

```bash
kctx health namespace payments --output json
```

The JSON includes:

* summary counts
* workload health
* Pod health
* Service endpoint health
* PVC health
* recent Warning Events
* factual signals

---

# Signal Philosophy

Signals are factual namespace-level observations.

Examples:

* `pod_crashloop`
* `pod_image_pull_error`
* `pod_not_ready`
* `high_restart_count`
* `workload_replicas_unavailable`
* `service_without_ready_endpoints`
* `pvc_pending`
* `recent_warning_event`
* `namespace_empty`

The command does not infer root causes or suggest fixes.

