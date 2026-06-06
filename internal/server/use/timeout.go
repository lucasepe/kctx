package use

import (
	"context"
	"net/http"
	"time"
)

func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if timeout <= 0 {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The server handlers pass this context down to the engine and
			// Kubernetes clients. When the deadline expires, in-flight API calls
			// are canceled and the normal error mapping turns the deadline into a
			// JSON 504 response.
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
