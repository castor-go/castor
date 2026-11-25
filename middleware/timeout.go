package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/castor-go/castor"
)

// Timeout cancels the request context after timeout and returns 504 when the
// handler finishes without writing a response and the deadline expired.
func Timeout(timeout time.Duration) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			wrapped := castor.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r.WithContext(ctx))
			if ctx.Err() == context.DeadlineExceeded && wrapped.Status() == 0 {
				wrapped.WriteHeader(http.StatusGatewayTimeout)
			}
		})
	}
}
