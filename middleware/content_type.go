package middleware

import (
	"net/http"
	"strings"
)

// SetHeader sets a response header before calling the next handler.
func SetHeader(key, value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, value)
			next.ServeHTTP(w, r)
		})
	}
}

// AllowContentType rejects request bodies whose media type is not in the
// allowed list. Requests without a body are always allowed.
func AllowContentType(contentTypes ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(contentTypes))
	for _, contentType := range contentTypes {
		mediaType, _, _ := strings.Cut(contentType, ";")
		allowed[strings.ToLower(strings.TrimSpace(mediaType))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}

			mediaType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
			if _, ok := allowed[strings.ToLower(strings.TrimSpace(mediaType))]; !ok {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
