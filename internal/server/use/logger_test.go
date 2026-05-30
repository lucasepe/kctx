package use

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
