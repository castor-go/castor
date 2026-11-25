// Package middleware provides composable middleware for net/http handlers.
package middleware

import (
	"net/http"
)

// New turns an http.Handler into middleware that always serves h.
func New(h http.Handler) func(next http.Handler) http.Handler {
	return func(http.Handler) http.Handler { return h }
}
