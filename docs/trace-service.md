# kctx trace service

```bash
kctx trace service <service-name> --namespace <namespace>
kctx trace service <service-name> --namespace <namespace> --output json
```

`kctx trace service` traces a Kubernetes Service to its actual backend objects.

It answers:

> Does this Service have usable backends, and which Pods do its endpoints target?

This is Kubernetes object correlation.

It is not packet tracing.

---

# What It Inspects

For a Service, the command collects:

* Service type
* selector
* ports
* selected Pods
* EndpointSlices
* legacy Endpoints fallback
* endpoint readiness
* endpoint target references
* backend Pod readiness
* backend Pod restart counts
* backend Pod owner chain
* Nodes where backend Pods run
* factual service health signals

EndpointSlices are preferred. Legacy Endpoints are used only as a fallback.

---

# Why It Exists

A Service can exist and still not route to usable backends.

Common situations include:

* selector matches no Pods
* selected Pods are not Ready
* EndpointSlices have no ready endpoints
* endpoints point at unexpected Pods
* manually managed Services have endpoints but no selector

`trace service` makes those conditions explicit.

---

# Useful For

This command is useful for:

* debugging “Service has no endpoints” problems
* validating rollout health from the Service point of view
* checking whether selected Pods are Ready
* finding endpoint-to-Pod correlation
* seeing which workload owns backend Pods
* feeding service backend context to automation or AI agents

---

# Not Useful For

This command is not meant for:

* packet tracing
* DNS resolution
* CNI-specific analysis
* service mesh tracing
* Ingress debugging
* NetworkPolicy analysis
* latency or metrics analysis
* application-level health checks

It only reports Kubernetes object state and relationships.

---

# Human Output

Human output focuses on backend usability:

```text
Service payments/payments-api

Type
  ClusterIP 10.96.42.17

Selector
  app = payments-api

Ports
  http TCP 80 -> 8080

Selected Pods
  Pod payments/api-abc12   Ready=true   Restarts=0   Node=worker-01
  Pod payments/api-def34   Ready=false  Restarts=7   Node=worker-02

Endpoints
  10.244.1.25:8080   Ready=true   Pod=api-abc12
  10.244.2.19:8080   Ready=false  Pod=api-def34

Signals
  warning endpoint_not_ready: endpoint 10.244.2.19:8080 is not Ready
```

---

# JSON Output

Use:

```bash
kctx trace service payments-api --namespace payments --output json
```

JSON output includes:

* Service summary
* selector
* ports
* endpoints
* backend Pods
* owners
* relations
* signals

It is stable and parseable for tools.

---

# Signal Philosophy

Signals are factual.

Examples:

* `service_has_no_selector`
* `selector_matches_no_pods`
* `no_endpoints_found`
* `selected_pod_not_ready`
* `selected_pod_missing_from_endpoints`
* `endpoint_not_ready`
* `endpoint_without_pod`
* `all_backends_unready`
* `service_has_no_usable_backends`

The command does not guess why a backend is unhealthy.

It reports what Kubernetes exposes.

