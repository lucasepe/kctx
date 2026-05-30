# kctx

> A Kubernetes context engine for humans and AI agents.

`kctx` is a small, composable tool that builds structured operational context around Kubernetes Pods and the resources directly connected to them.

It is designed to answer questions like:

* What is this Pod connected to?
* Which workload owns these Pods?
* Which Services actually route traffic to healthy backends?
* Why does this namespace look unhealthy?
* What are the important operational signals in this part of the cluster?

Without requiring:

* dashboards
* AI models
* cloud platforms
* observability stacks
* graph databases
* vendor-specific tooling

---

# Philosophy

`kctx` follows a simple idea:

> Kubernetes troubleshooting is mostly a context problem.

Most tools expose:

* raw YAML
* logs
* metrics
* events
* manifests

But operators, SREs, platform engineers, and AI agents still need to manually reconstruct:

* ownership
* dependencies
* service relationships
* workload hierarchy
* endpoint health
* scheduling relationships
* operational signals

`kctx` focuses on building that missing context layer.

It does not try to:

* replace observability platforms
* become an AI copilot
* automate remediation
* manage clusters
* predict failures

Instead, it produces:

* normalized entities
* relationships
* operational signals
* machine-readable context
* deterministic outputs

---

# What `kctx` Is

`kctx` is:

* a Kubernetes context resolver
* a cluster relationship explorer
* a graph-aware troubleshooting primitive
* an operational context exporter
* an AI-friendly infrastructure tool
* a reusable backend for agents and automation

It is intentionally:

* small
* composable
* read-only
* deterministic
* boring
* scriptable

---

# What `kctx` Is NOT

`kctx` is NOT:

* a monitoring system
* an observability platform
* a dashboard
* a service mesh
* a log aggregation system
* a metrics pipeline
* an AI assistant
* a remediation engine
* a cluster manager
* a GitOps platform
* a graph database

It does not:

* mutate resources
* restart workloads
* apply manifests
* execute remediation
* make AI decisions
* infer speculative root causes

---

# Why This Exists

Kubernetes resources are deeply interconnected.

Even simple operational questions often require reconstructing relationships manually:

```text
Service
  → EndpointSlice
    → Pod
      → ReplicaSet
        → Deployment
```

Or:

```text
Pod
  → Secret
  → ConfigMap
  → PVC
  → Node
  → Events
```

Most troubleshooting workflows still involve:

```bash
kubectl get ...
kubectl describe ...
kubectl logs ...
grep ...
```

followed by manual correlation.

`kctx` exists to make that correlation explicit and reusable.

---

# Design Goals

## 1. Context First

The primary goal is to resolve operational context around concrete runtime entities, starting with Pods.

Not metrics.
Not logs.
Not dashboards.

Context.

---

## 2. Human and Machine Readable

Every output should work for:

* humans
* shell scripts
* CI systems
* AI agents
* MCP tools
* automation frameworks

Outputs are designed to be:

* stable
* structured
* deterministic
* parseable

---

## 3. Read-Only

`kctx` never mutates the cluster.

No:

* apply
* patch
* delete
* rollout restart
* remediation actions

This keeps the tool safe and composable.

---

## 4. Pod-First

The initial versions are Pod-first.

That is deliberate. Most Kubernetes incidents eventually touch Pods:

* readiness
* scheduling
* image pulls
* restart loops
* owner chains
* Services and EndpointSlices
* configuration and storage dependencies
* Nodes and Events

`kctx` may resolve Kubernetes resource names through discovery, including aliases such as `po` and `pods`, but deep operational context is intentionally implemented where relationships are knowable.

Generic resource resolution is a convenience layer.

Operational context remains explicit.

## 5. Kubernetes-Native

The first stable context model focuses on native Kubernetes resources connected to Pods and namespaces.

This includes:

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
* Nodes
* Events

The goal is to establish a stable core model before introducing ecosystem-specific adapters.

For CRDs and ecosystem resources, `kctx` does not infer controller-specific semantics automatically. A custom resource may be reconciled by a controller Pod, may create secondary resources, or may only influence behavior indirectly. Kubernetes does not expose those relationships in a generic, reliable way.

Future adapters can add that knowledge explicitly.

---

## 6. AI-Friendly Without Being AI-Dependent

`kctx` is intentionally designed to work well with AI systems.

However:

* the core does not require LLMs
* the core does not embed AI logic
* the core does not depend on any AI provider

AI systems should consume `kctx`.

`kctx` itself should remain infrastructure.

---

# Example Use Cases

## Operational Troubleshooting

Understand:

* why a Pod is unhealthy
* which workload owns a failing Pod
* which Services target broken backends
* which workloads are degraded in a namespace

---

## AI Agent Backends

Provide structured Kubernetes context to:

* AI assistants
* MCP servers
* autonomous agents
* incident analysis systems

Without forcing agents to parse raw Kubernetes YAML.

---

## Incident Snapshots

Export normalized namespace context for:

* debugging
* incident reports
* offline analysis
* support cases

---

## Platform Engineering Tooling

Use `kctx` as a backend primitive for:

* internal tooling
* operational portals
* automation systems
* troubleshooting APIs

---

## CI/CD Diagnostics

Run structured cluster checks inside:

* pipelines
* GitOps systems
* validation jobs
* deployment verification flows

---

# Who This Is For

`kctx` may be useful for:

* platform engineers
* SREs
* DevOps teams
* Kubernetes operators
* AI infrastructure engineers
* incident response teams
* internal platform tooling teams
* MCP/agent developers

Especially teams that:

* automate Kubernetes operations
* build AI infrastructure tooling
* need deterministic cluster context
* want composable operational primitives

---

# Who This Is Probably NOT For

`kctx` is probably not useful if you are looking for:

* a full observability platform
* a Kubernetes dashboard
* a metrics system
* centralized logging
* cluster management
* auto-remediation
* GUI-heavy tooling
* a complete AI copilot
* Kubernetes automation frameworks

There are already excellent tools for those problems.

---

# Architecture Direction

The project starts as:

* a CLI tool
* a reusable Go engine
* a structured context builder

Over time it may evolve into:

* an HTTP service
* an MCP tool
* a kubectl plugin
* an agent backend
* a reusable library

But the core philosophy remains the same:

> Build reliable Kubernetes operational context.

---

# Project Status

Early development.

The initial focus is:

* core entity modeling
* relationship resolution
* operational signals
* deterministic outputs
* reusable engine architecture

Before:

* CRD adapters
* AI integrations
* advanced graphing
* ecosystem-specific semantics

The current direction is intentionally conservative:

* Pod context is the primary primitive.
* Service tracing is supported because Services select Pods.
* Namespace health and dumps summarize known Kubernetes operational context.
* Resource discovery exists to normalize user input and support future adapters.
* Generic CRD reasoning is not a goal without explicit adapter knowledge.

---

# Long-Term Vision

The long-term goal is not to build another Kubernetes platform.

The goal is to provide a small, reliable foundation layer for:

* troubleshooting
* automation
* AI tooling
* operational reasoning

A Unix-style primitive for Kubernetes context resolution.
