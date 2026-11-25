package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/castor-go/castor"
)

// URLFormatCtxKey is the context key that holds the URL format.
var URLFormatCtxKey = &castor.ContextKey{Name: "URLFormat"}

// URLFormat extracts a trailing URL extension, stores it in the context, and
// strips it from the request path before routing. It must run globally.
func URLFormat(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format := ""
		requestPath := r.URL.Path
		slash := strings.LastIndexByte(requestPath, '/')
		dot := strings.LastIndexByte(requestPath, '.')

		if dot > slash+1 && dot < len(requestPath)-1 {
			format = requestPath[dot+1:]
			r.URL.Path = requestPath[:dot]
			r.URL.RawPath = ""
		}

		ctx := context.WithValue(r.Context(), URLFormatCtxKey, format)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetURLFormat returns the format extracted by URLFormat.
func GetURLFormat(r *http.Request) string {
	return castor.GetValue[string](r.Context(), URLFormatCtxKey)
}
