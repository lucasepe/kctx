package use

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lucasepe/kctx/internal/observability"
)

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get(observability.RequestIDHeader))
			if requestID == "" {
				requestID = newRequestID()
			}

			w.Header().Set(observability.RequestIDHeader, requestID)
			ctx := observability.ContextWithRequestID(r.Context(), requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequestIDFrom(r *http.Request) string {
	if r == nil {
		return ""
	}
	return observability.RequestIDFromContext(r.Context())
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
