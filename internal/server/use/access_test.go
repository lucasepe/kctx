package use

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucasepe/kctx/internal/observability"
)

func TestAccessLogsRequest(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	req = req.WithContext(observability.ContextWithRequestID(context.Background(), "req-123"))
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("User-Agent", "kctx-test")
	req.Header.Set("X-Forwarded-For", "203.0.113.7")

	rec := httptest.NewRecorder()
	Access(logger)(next).ServeHTTP(rec, req)

	if !called {
		t.Fatal("next handler was not called")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}

	var log map[string]any
	if err := json.Unmarshal(out.Bytes(), &log); err != nil {
		t.Fatalf("decode log: %v", err)
	}

	assertLogField(t, log, "msg", "http request")
	assertLogField(t, log, "ip", "203.0.113.7")
	assertLogField(t, log, "method", http.MethodGet)
	assertLogField(t, log, "path", "/livez")
	assertLogField(t, log, "url", "/livez")
	assertLogField(t, log, "userAgent", "kctx-test")
	assertLogField(t, log, "request_id", "req-123")

	if status, ok := log["status"].(float64); !ok || status != http.StatusAccepted {
		t.Fatalf("status = %v, want %d", log["status"], http.StatusAccepted)
	}

	if latency, ok := log["latency"].(string); !ok || latency == "" {
		t.Fatalf("latency = %v, want non-empty string", log["latency"])
	}
	if _, ok := log["latency_ms"].(float64); !ok {
		t.Fatalf("latency_ms = %v, want number", log["latency_ms"])
	}
}

func assertLogField(t *testing.T, log map[string]any, key, want string) {
	t.Helper()

	if got, ok := log[key].(string); !ok || got != want {
		t.Fatalf("%s = %v, want %q", key, log[key], want)
	}
}
