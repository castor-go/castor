package middleware

import (
	"io"
	"net/http"
	"strings"
)

// Heartbeat answers GET and HEAD requests to endpoint without invoking the
// rest of the chain. It must run globally, before the mux.
func Heartbeat(endpoint string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
				strings.EqualFold(r.URL.Path, endpoint) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, ".")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
