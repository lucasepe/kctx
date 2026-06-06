package use

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucasepe/kctx/internal/observability"
)

func TestRequestIDUsesIncomingHeader(t *testing.T) {
	const want = "req-123"

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFrom(r); got != want {
			t.Fatalf("request ID from context = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(observability.RequestIDHeader, want)
	rec := httptest.NewRecorder()

	RequestID()(next).ServeHTTP(rec, req)

	if got := rec.Header().Get(observability.RequestIDHeader); got != want {
		t.Fatalf("response request ID = %q, want %q", got, want)
	}
}

func TestRequestIDGeneratesMissingHeader(t *testing.T) {
	var requestID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID = RequestIDFrom(r)
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	RequestID()(next).ServeHTTP(rec, req)

	if requestID == "" {
		t.Fatal("request ID from context is empty")
	}
	if got := rec.Header().Get(observability.RequestIDHeader); got != requestID {
		t.Fatalf("response request ID = %q, want %q", got, requestID)
	}
}
