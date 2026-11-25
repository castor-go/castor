package middleware

import (
	"context"
	"crypto/rand"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/castor-go/castor"
)

var (
	requestIDPrefix string
	requestIDSeq    atomic.Uint64
)

func init() {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "localhost"
	}
	requestIDPrefix = hostname + "/" + rand.Text()[:10]
}

// RequestID adds a request ID to the request context and reuses an incoming
// value from RequestIDHeader when present.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(castor.RequestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx := context.WithValue(r.Context(), castor.RequestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetReqID returns the request ID stored in ctx.
func GetReqID(ctx context.Context) string {
	return castor.GetValue[string](ctx, castor.RequestIDKey)
}

// NextRequestID returns the next value in the request ID sequence.
func NextRequestID() uint64 {
	return requestIDSeq.Add(1)
}

func newRequestID() string {
	sequence := strconv.FormatUint(requestIDSeq.Add(1), 10)
	const padding = 6
	size := len(requestIDPrefix) + 1 + max(padding, len(sequence))
	buf := make([]byte, 0, size)
	buf = append(buf, requestIDPrefix...)
	buf = append(buf, '-')
	for i := len(sequence); i < padding; i++ {
		buf = append(buf, '0')
	}
	buf = append(buf, sequence...)
	return string(buf)
}
