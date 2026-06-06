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

func TestLoggerAddsLoggerToRequestContext(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := LoggerFrom(r.Context()); got != logger {
			t.Fatalf("logger from context = %p, want %p", got, logger)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Logger(logger)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestLoggerFromFallsBackToDefault(t *testing.T) {
	got := LoggerFrom(context.TODO())
	if got != slog.Default() {
		t.Fatalf("logger from nil context = %p, want default %p", got, slog.Default())
	}
}

func TestLoggerIncludesRequestID(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LoggerFrom(r.Context()).Info("hello")
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(observability.ContextWithRequestID(context.Background(), "req-123"))
	rec := httptest.NewRecorder()

	Logger(logger)(next).ServeHTTP(rec, req)

	var log map[string]any
	if err := json.Unmarshal(out.Bytes(), &log); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if got := log["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
}
