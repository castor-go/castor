package middleware

import (
	"net/http"
	"strings"
)

// AllowContentEncoding rejects request bodies whose Content-Encoding is not
// in the allowed list. Requests without a body are always allowed.
func AllowContentEncoding(contentEncodings ...string) func(next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(contentEncodings))
	for _, encoding := range contentEncodings {
		allowed[strings.ToLower(strings.TrimSpace(encoding))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}

			for _, header := range r.Header.Values("Content-Encoding") {
				for encoding := range strings.SplitSeq(header, ",") {
					encoding = strings.TrimSpace(encoding)
					if encoding == "" {
						continue
					}
					if _, ok := allowed[strings.ToLower(encoding)]; !ok {
						w.WriteHeader(http.StatusUnsupportedMediaType)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
