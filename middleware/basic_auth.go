package middleware

import (
	"crypto/subtle"
	"net/http"
	"strconv"
)

// BasicAuth protects a route with HTTP basic authentication.
func BasicAuth(realm string, creds map[string]string) func(next http.Handler) http.Handler {
	challenge := "Basic realm=" + strconv.Quote(realm)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok {
				basicAuthFailed(w, challenge)
				return
			}

			expected, known := creds[user]
			valid := subtle.ConstantTimeCompare([]byte(pass), []byte(expected)) == 1
			if !known || !valid {
				basicAuthFailed(w, challenge)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func basicAuthFailed(w http.ResponseWriter, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	w.WriteHeader(http.StatusUnauthorized)
}
