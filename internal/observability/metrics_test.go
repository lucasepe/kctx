package observability

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	var counter Counter
	counter.Inc()
	counter.Add(2)

	if got := counter.Value(); got != 3 {
		t.Fatalf("counter value = %d, want 3", got)
	}
	if got := counter.String(); got != "3" {
		t.Fatalf("counter string = %q, want 3", got)
	}
}

func TestHistogramUsesCumulativeBuckets(t *testing.T) {
	histogram := NewHistogram([]float64{0.1, 0.5})
	histogram.Observe(0.05)
	histogram.Observe(0.2)
	histogram.Observe(2)

	var got struct {
		Buckets []Bucket `json:"buckets"`
		Count   uint64   `json:"count"`
		Sum     float64  `json:"sum"`
	}
	if err := json.Unmarshal([]byte(histogram.String()), &got); err != nil {
		t.Fatalf("decode histogram: %v", err)
	}
	if got.Count != 3 {
		t.Fatalf("count = %d, want 3", got.Count)
	}
	wantBuckets := []Bucket{{LE: "0.1", Count: 1}, {LE: "0.5", Count: 2}, {LE: "+Inf", Count: 3}}
	if len(got.Buckets) != len(wantBuckets) {
		t.Fatalf("buckets = %v, want %v", got.Buckets, wantBuckets)
	}
	for i := range wantBuckets {
		if got.Buckets[i] != wantBuckets[i] {
			t.Fatalf("bucket %d = %#v, want %#v", i, got.Buckets[i], wantBuckets[i])
		}
	}
}

func TestServerMetricsObserveHTTPRequest(t *testing.T) {
	metrics := NewServerMetrics()
	metrics.ObserveHTTPRequest(http.MethodGet, "/context/pod/payments/api-1", http.StatusOK, 120*time.Millisecond, 512)
	metrics.ObserveHTTPRequest(http.MethodGet, "/context/pod/payments/api-2", http.StatusNotFound, 10*time.Millisecond, 64)

	var got struct {
		Requests map[string]uint64 `json:"http_requests_total"`
	}
	if err := json.Unmarshal([]byte(metrics.String()), &got); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if got.Requests["GET /context/pod/{namespace}/{name} 200"] != 1 {
		t.Fatalf("200 request count = %d, want 1", got.Requests["GET /context/pod/{namespace}/{name} 200"])
	}
	if got.Requests["GET /context/pod/{namespace}/{name} 404"] != 1 {
		t.Fatalf("404 request count = %d, want 1", got.Requests["GET /context/pod/{namespace}/{name} 404"])
	}
}

func TestHTTPRoute(t *testing.T) {
	tests := map[string]string{
		"/livez":                      "/livez",
		"/readyz":                     "/readyz",
		"/metrics":                    "/metrics",
		"/version":                    "/version",
		"/context/pod/payments/api-1": "/context/pod/{namespace}/{name}",
		"/graph/pod/payments/api-1":   "/graph/pod/{namespace}/{name}",
		"/trace/service/payments/api": "/trace/service/{namespace}/{name}",
		"/health/namespace/payments":  "/health/namespace/{namespace}",
		"/dump/namespace/payments":    "/dump/namespace/{namespace}",
		"/not-found":                  "unknown",
	}

	for path, want := range tests {
		if got := HTTPRoute(path); got != want {
			t.Fatalf("HTTPRoute(%q) = %q, want %q", path, got, want)
		}
	}
}
