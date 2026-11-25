package middleware

import (
	"context"
	"net/http"
)

// WithValue stores key/value in the request context.
func WithValue(key, value any) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), key, value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
