package use

import (
	"net/http"

	"github.com/lucasepe/kctx/internal/limits"
)

func KubeAPIBudget(limit int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limit <= 0 {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The budget is request-scoped and consumed by the kube client wrapper.
			// It protects kctx and the API server from one HTTP request expanding
			// into an unbounded number of Kubernetes calls.
			ctx := limits.ContextWithBudget(r.Context(), limits.NewBudget(limit))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
