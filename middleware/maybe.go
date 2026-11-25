package middleware

import "net/http"

// Maybe applies mw only when condition returns true for the request.
func Maybe(mw func(http.Handler) http.Handler, condition func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if condition(r) {
				wrapped.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
