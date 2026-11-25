package middleware

import (
	"net/http"
	"strings"
)

// StripSlashes removes one trailing slash before routing the request.
// It must run globally because it mutate the request URL
func StripSlashes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 1 && strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

// RedirectSlashes redirects paths with a trailing slash to the equivalent
// It must run globally because it mutate the request URL
func RedirectSlashes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) <= 1 || !strings.HasSuffix(r.URL.Path, "/") {
			next.ServeHTTP(w, r)
			return
		}

		// Prevent "/\evil.com"-style protocol-relative redirects.
		cleaned := strings.ReplaceAll(r.URL.Path, `\`, `/`)
		target := "/" + strings.Trim(cleaned, "/")
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}
