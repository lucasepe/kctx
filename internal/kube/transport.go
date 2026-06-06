package kube

import (
	"net/http"

	"github.com/lucasepe/kctx/internal/observability"
)

func requestIDTransport(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return requestIDRoundTripper{next: rt}
}

type requestIDRoundTripper struct {
	next http.RoundTripper
}

func (rt requestIDRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if requestID := observability.RequestIDFromContext(req.Context()); requestID != "" {
		req = req.Clone(req.Context())
		req.Header.Set(observability.RequestIDHeader, requestID)
	}
	return rt.next.RoundTrip(req)
}
