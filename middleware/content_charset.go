package middleware

import (
	"mime"
	"net/http"
	"slices"
	"strings"
)

// ContentCharset rejects requests whose body charset is not in charsets.
// An empty charset also allows requests without a Content-Type header.
// Requests without a body are always allowed.
func ContentCharset(charsets ...string) func(next http.Handler) http.Handler {
	allowed := make([]string, len(charsets))
	for i, charset := range charsets {
		allowed[i] = strings.ToLower(strings.TrimSpace(charset))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if !allowedCharset(r.Header.Get("Content-Type"), allowed) {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func allowedCharset(contentType string, allowed []string) bool {
	_, params, err := mime.ParseMediaType(contentType)
	if err == nil {
		return slices.Contains(allowed, strings.ToLower(params["charset"]))
	}

	// Keep lenient handling for malformed media types.
	_, value := cutTrim(strings.ToLower(contentType), ";")
	_, value = cutTrim(value, "charset=")
	value, _ = cutTrim(value, ";")
	return slices.Contains(allowed, value)
}

func cutTrim(value, separator string) (string, string) {
	before, after, found := strings.Cut(value, separator)
	before = strings.TrimSpace(before)
	if found {
		after = strings.TrimSpace(after)
	}
	return before, after
}
