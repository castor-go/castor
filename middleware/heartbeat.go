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
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			if (req.Method == http.MethodGet || req.Method == http.MethodHead) &&
				strings.EqualFold(req.URL.Path, endpoint) {
				res.Header().Set("Content-Type", "application/json")
				res.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(res, "{\"status\":\"ok\"}")
				return
			}
			next.ServeHTTP(res, req)
		})
	}
}
