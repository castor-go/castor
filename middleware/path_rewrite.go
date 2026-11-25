package middleware

import (
	"net/http"
	"strings"
)

// PathRewrite replaces the first occurrence of old in the request path.
// It must run globally because it mutate the request URL
func PathRewrite(old, new string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rewritten := strings.Replace(r.URL.Path, old, new, 1)
			if rewritten != r.URL.Path {
				r.URL.Path = rewritten
				r.URL.RawPath = ""
			}
			next.ServeHTTP(w, r)
		})
	}
}
