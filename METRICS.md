# kctx Server Metrics

`kctx serve` exposes lightweight process metrics at:

```bash
curl http://localhost:8080/metrics
```

The endpoint returns JSON and uses only the Go standard library. It is meant to answer operational questions about `kctx` itself, not to monitor Kubernetes workloads through `kctx`.

## Why JSON Metrics

The metrics implementation intentionally avoids OpenTelemetry and Prometheus client dependencies for now.

That choice keeps the server small and predictable:

- no additional dependency tree
- no background exporters or collectors
- no required scrape infrastructure
- output that can be inspected with `curl`, `jq`, scripts, or tests
- enough signal for operating the read-only HTTP API

The internal implementation lives in `internal/observability`. It provides a small counter, a fixed-bucket histogram, and a `ServerMetrics` registry used by the HTTP access middleware.

## What Is Collected

`/metrics` currently includes:

- `http_requests_total`
- `http_request_duration_seconds`
- `http_response_size_bytes`

Metrics are grouped by:

- HTTP method
- normalized route
- HTTP status

Routes are normalized to avoid high-cardinality metric names. For example:

```text
/context/pod/payments/api-1
/context/pod/default/web-7d8f4
```

are both recorded as:

```text
GET /context/pod/{namespace}/{name} 200
```

This keeps labels stable even when namespace, Pod, or Service names vary.

## Example Output

```json
{
  "http_requests_total": {
    "GET /livez 200": 4,
    "GET /health/namespace/{namespace} 200": 2
  },
  "http_request_duration_seconds": {
    "GET /livez 200": {
      "buckets": [
        { "le": "0.005", "count": 4 },
        { "le": "0.01", "count": 4 },
        { "le": "+Inf", "count": 4 }
      ],
      "count": 4,
      "sum": 0.0012
    }
  },
  "http_response_size_bytes": {
    "GET /livez 200": {
      "buckets": [
        { "le": "128", "count": 4 },
        { "le": "+Inf", "count": 4 }
      ],
      "count": 4,
      "sum": 96
    }
  }
}
```

This example is shortened for readability. Histograms use cumulative bucket counts. The final bucket is always `+Inf`.

## How To Monitor

For local checks:

```bash
curl -s http://localhost:8080/metrics | jq .
```

For Kubernetes, expose the same endpoint through the Service and scrape or poll it from your preferred lightweight collector. A simple polling script can watch for:

- rising non-2xx request counts
- increasing latency bucket counts above `1` or `2.5` seconds
- `504` request counts, which indicate request timeouts
- `429` request counts, which can indicate exhausted Kubernetes API budgets
- unexpectedly large response-size buckets
- request volume changes on `/readyz` and `/livez`

Readiness and liveness use dedicated endpoints:

```bash
curl http://localhost:8080/readyz
curl http://localhost:8080/livez
```

Kubernetes probes should use `/readyz` for readiness and `/livez` for liveness.

## Current Limits

The metrics are in-memory and reset when the process restarts. They are designed for simple runtime visibility, smoke checks, and lightweight scraping.

Not yet included:

- Kubernetes API error counts
- timeout counts
- unsupported resource counts
- process/runtime metrics
- Prometheus text exposition

Those can be added later without changing the current endpoint shape too much, or a Prometheus-compatible endpoint can be introduced separately if the project needs it.
