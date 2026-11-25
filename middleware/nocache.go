package middleware

import (
	"net/http"
	"time"
)

var noCacheHeaders = [...]struct {
	key, value string
}{
	{"Expires", time.Unix(0, 0).UTC().Format(http.TimeFormat)},
	{"Cache-Control", "no-cache, no-store, no-transform, must-revalidate, private, max-age=0"},
	{"Pragma", "no-cache"},
	{"X-Accel-Expires", "0"},
}

var etagRequestHeaders = [...]string{
	"ETag",
	"If-Modified-Since",
	"If-Match",
	"If-None-Match",
	"If-Range",
	"If-Unmodified-Since",
}

// NoCache sets response headers that prevent caching by clients and proxies.
func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range etagRequestHeaders {
			r.Header.Del(header)
		}
		for _, header := range noCacheHeaders {
			w.Header().Set(header.key, header.value)
		}
		next.ServeHTTP(w, r)
	})
}
