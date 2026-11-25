package middleware

import (
	"net/http"
	"strings"
)

// PageRoute serves handler for an exact, case-insensitive GET request path.
// It must run globally, before the mux.
func PageRoute(path string, handler http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.EqualFold(r.URL.Path, path) {
				handler.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
