package use

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/lucasepe/kctx/internal/observability"
)

func Access(l *slog.Logger, metrics ...*observability.ServerMetrics) func(http.Handler) http.Handler {
	if l == nil {
		l = slog.Default()
	}
	var serverMetrics *observability.ServerMetrics
	if len(metrics) > 0 {
		serverMetrics = metrics[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = forwarded
			}

			next.ServeHTTP(rec, r)

			latency := time.Since(start)
			if serverMetrics != nil {
				serverMetrics.ObserveHTTPRequest(r.Method, r.URL.Path, rec.status, latency, rec.bytes)
			}

			l.Info("http request",
				slog.String("ip", ip),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("url", r.URL.String()),
				slog.Int("status", rec.status),
				slog.String("userAgent", r.UserAgent()),
				slog.String("latency", latency.String()),
				slog.Int64("latency_ms", latency.Milliseconds()),
				slog.String("request_id", observability.RequestIDFromContext(r.Context())),
			)
		})
	}
}

// statusRecorder captures response details that http.ResponseWriter does not
// expose after a handler runs. Access logs and metrics use these fields for
// status codes and response-size observations without changing handler code.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	bytes       int64
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}
