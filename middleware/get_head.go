package middleware

import "net/http"

// GetHead is a no-op for ServeMux-based servers, which already route HEAD
// requests to handlers registered for GET. It is kept for migration parity.
func GetHead(next http.Handler) http.Handler {
	return next
}
