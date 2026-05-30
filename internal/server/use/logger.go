package use

import (
	"context"
	"log/slog"
	"net/http"
)

func Logger(l *slog.Logger) func(http.Handler) http.Handler {
	if l == nil {
		l = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), loggerContextKey{}, l)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func LoggerFrom(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if l, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

type loggerContextKey struct{}
