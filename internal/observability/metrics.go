package observability

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	defaultLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	defaultSizeBuckets    = []float64{128, 512, 1024, 4096, 16384, 65536, 262144, 1048576}
)

type Counter struct {
	value atomic.Uint64
}

func (c *Counter) Inc() {
	c.Add(1)
}

func (c *Counter) Add(delta uint64) {
	c.value.Add(delta)
}

func (c *Counter) Value() uint64 {
	return c.value.Load()
}

func (c *Counter) String() string {
	return strconv.FormatUint(c.Value(), 10)
}

func (c *Counter) MarshalJSON() ([]byte, error) {
	return []byte(c.String()), nil
}

type Histogram struct {
	bounds []float64
	counts []atomic.Uint64
	count  atomic.Uint64
	sum    atomicFloat64
}

func NewHistogram(bounds []float64) *Histogram {
	if len(bounds) == 0 {
		bounds = []float64{1}
	}
	sorted := append([]float64(nil), bounds...)
	sort.Float64s(sorted)
	return &Histogram{
		bounds: sorted,
		counts: make([]atomic.Uint64, len(sorted)+1),
	}
}

func (h *Histogram) Observe(value float64) {
	if value < 0 {
		value = 0
	}
	idx := sort.SearchFloat64s(h.bounds, value)
	h.counts[idx].Add(1)
	h.count.Add(1)
	h.sum.Add(value)
}

func (h *Histogram) Count() uint64 {
	return h.count.Load()
}

func (h *Histogram) Sum() float64 {
	return h.sum.Load()
}

func (h *Histogram) String() string {
	b, err := h.MarshalJSON()
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (h *Histogram) MarshalJSON() ([]byte, error) {
	return json.Marshal(histogramSnapshot{
		Buckets: h.buckets(),
		Count:   h.Count(),
		Sum:     h.Sum(),
	})
}

func (h *Histogram) buckets() []Bucket {
	buckets := make([]Bucket, 0, len(h.counts))
	var cumulative uint64
	for i := range h.counts {
		cumulative += h.counts[i].Load()
		le := "+Inf"
		if i < len(h.bounds) {
			le = strconv.FormatFloat(h.bounds[i], 'g', -1, 64)
		}
		buckets = append(buckets, Bucket{LE: le, Count: cumulative})
	}
	return buckets
}

type Bucket struct {
	LE    string `json:"le"`
	Count uint64 `json:"count"`
}

type histogramSnapshot struct {
	Buckets []Bucket `json:"buckets"`
	Count   uint64   `json:"count"`
	Sum     float64  `json:"sum"`
}

type ServerMetrics struct {
	mu           sync.Mutex
	requests     map[string]*Counter
	latencies    map[string]*Histogram
	responseSize map[string]*Histogram
}

func NewServerMetrics() *ServerMetrics {
	return &ServerMetrics{
		requests:     make(map[string]*Counter),
		latencies:    make(map[string]*Histogram),
		responseSize: make(map[string]*Histogram),
	}
}

func (m *ServerMetrics) ObserveHTTPRequest(method, path string, status int, latency time.Duration, bytes int64) {
	if m == nil {
		return
	}
	key := httpMetricKey(method, path, status)
	m.mu.Lock()
	requests := counterFor(m.requests, key)
	latencies := histogramFor(m.latencies, key, defaultLatencyBuckets)
	responseSize := histogramFor(m.responseSize, key, defaultSizeBuckets)
	m.mu.Unlock()

	requests.Inc()
	latencies.Observe(latency.Seconds())
	responseSize.Observe(float64(bytes))
}

func (m *ServerMetrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(m)
}

func (m *ServerMetrics) String() string {
	b, err := m.MarshalJSON()
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (m *ServerMetrics) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	return json.Marshal(struct {
		RequestsTotal          map[string]*Counter   `json:"http_requests_total"`
		RequestDurationSeconds map[string]*Histogram `json:"http_request_duration_seconds"`
		ResponseSizeBytes      map[string]*Histogram `json:"http_response_size_bytes"`
	}{
		RequestsTotal:          cloneCounters(m.requests),
		RequestDurationSeconds: cloneHistograms(m.latencies),
		ResponseSizeBytes:      cloneHistograms(m.responseSize),
	})
}

func counterFor(counters map[string]*Counter, key string) *Counter {
	counter := counters[key]
	if counter == nil {
		counter = &Counter{}
		counters[key] = counter
	}
	return counter
}

func histogramFor(histograms map[string]*Histogram, key string, buckets []float64) *Histogram {
	histogram := histograms[key]
	if histogram == nil {
		histogram = NewHistogram(buckets)
		histograms[key] = histogram
	}
	return histogram
}

func cloneCounters(in map[string]*Counter) map[string]*Counter {
	out := make(map[string]*Counter, len(in))
	for key, counter := range in {
		out[key] = counter
	}
	return out
}

func cloneHistograms(in map[string]*Histogram) map[string]*Histogram {
	out := make(map[string]*Histogram, len(in))
	for key, histogram := range in {
		out[key] = histogram
	}
	return out
}

func httpMetricKey(method, path string, status int) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	return fmt.Sprintf("%s %s %d", method, HTTPRoute(path), status)
}

func HTTPRoute(path string) string {
	switch {
	case path == "/livez":
		return "/livez"
	case path == "/readyz":
		return "/readyz"
	case path == "/metrics":
		return "/metrics"
	case path == "/version":
		return "/version"
	case strings.HasPrefix(path, "/context/pod/"):
		return "/context/pod/{namespace}/{name}"
	case strings.HasPrefix(path, "/graph/pod/"):
		return "/graph/pod/{namespace}/{name}"
	case strings.HasPrefix(path, "/trace/service/"):
		return "/trace/service/{namespace}/{name}"
	case strings.HasPrefix(path, "/health/namespace/"):
		return "/health/namespace/{namespace}"
	case strings.HasPrefix(path, "/dump/namespace/"):
		return "/dump/namespace/{namespace}"
	default:
		return "unknown"
	}
}

type atomicFloat64 struct {
	bits atomic.Uint64
}

func (f *atomicFloat64) Add(delta float64) {
	for {
		oldBits := f.bits.Load()
		old := math.Float64frombits(oldBits)
		newBits := math.Float64bits(old + delta)
		if f.bits.CompareAndSwap(oldBits, newBits) {
			return
		}
	}
}

func (f *atomicFloat64) Load() float64 {
	return math.Float64frombits(f.bits.Load())
}
