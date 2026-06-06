package kube

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lucasepe/kctx/internal/observability"
)

func TestRequestIDRoundTripperPropagatesContextValue(t *testing.T) {
	next := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get(observability.RequestIDHeader); got != "req-123" {
			t.Fatalf("request ID header = %q, want req-123", got)
		}
		return okResponse(), nil
	})

	req := newTransportRequest(t)
	req = req.WithContext(observability.ContextWithRequestID(req.Context(), "req-123"))

	_, err := requestIDRoundTripper{next: next}.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
}

func TestRequestIDRoundTripperLeavesMissingContextValueUnset(t *testing.T) {
	next := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get(observability.RequestIDHeader); got != "" {
			t.Fatalf("request ID header = %q, want empty", got)
		}
		return okResponse(), nil
	})

	_, err := requestIDRoundTripper{next: next}.RoundTrip(newTransportRequest(t))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTransportRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/api", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func okResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}
}
