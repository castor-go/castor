package middleware

import (
	"net/http"
	"time"
)

// Sunset announces when a route will stop working per RFC 8594.
func Sunset(sunsetAt time.Time, links ...string) func(http.Handler) http.Handler {
	if sunsetAt.IsZero() {
		panic("middleware.Sunset: sunsetAt must not be zero")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Sunset", sunsetAt.UTC().Format(http.TimeFormat))
			addLinks(w, links)
			next.ServeHTTP(w, r)
		})
	}
}
