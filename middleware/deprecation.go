package middleware

import (
	"net/http"
	"strconv"
	"time"
)

// Deprecation marks a route as deprecated per RFC 9745.
func Deprecation(deprecatedAt time.Time, links ...string) func(http.Handler) http.Handler {
	if deprecatedAt.IsZero() {
		panic("middleware.Deprecation: deprecatedAt must not be zero")
	}

	value := "@" + strconv.FormatInt(deprecatedAt.Unix(), 10)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", value)
			addLinks(w, links)
			next.ServeHTTP(w, r)
		})
	}
}

func addLinks(w http.ResponseWriter, links []string) {
	for _, link := range links {
		w.Header().Add("Link", link)
	}
}
