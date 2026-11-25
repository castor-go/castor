package middleware

import "net/http"

// SupressNotFound serves the mux's 404 or 405 response before the remaining
// middleware chain when the request cannot match a route.
func SupressNotFound(mux *http.ServeMux) func(http.Handler) http.Handler {
	if mux == nil {
		panic("middleware.SupressNotFound: mux must not be nil")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler, pattern := mux.Handler(r)
			if pattern == "" {
				handler.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SuppressNotFound is the correctly spelled alias for SupressNotFound.
func SuppressNotFound(mux *http.ServeMux) func(http.Handler) http.Handler {
	return SupressNotFound(mux)
}
