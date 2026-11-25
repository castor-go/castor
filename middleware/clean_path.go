package middleware

import (
	"net/http"
	"net/url"
	"path"
)

// CleanPath removes duplicate slashes and dot segments from the request path.
// It must run globally because it mutate the request URL
func CleanPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.RawPath
		requestPath := r.URL.Path
		if raw != "" {
			requestPath = raw
		}

		cleaned := path.Clean("/" + requestPath)
		if raw != "" {
			if decoded, err := url.PathUnescape(cleaned); err == nil {
				r.URL.Path = decoded
				r.URL.RawPath = cleaned
			} else {
				r.URL.Path = cleaned
				r.URL.RawPath = ""
			}
		} else {
			r.URL.Path = cleaned
		}

		next.ServeHTTP(w, r)
	})
}
