package castor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

// GetCookie returns the named cookie's value, or "" when it does not exist.
func GetCookie(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// SetCookie writes a root-path, HTTP-only cookie.
func SetCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, newCookie(name, value))
}

// RemoveCookie expires the named cookie.
func RemoveCookie(w http.ResponseWriter, name string) {
	cookie := newCookie(name, "")
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0)
	http.SetCookie(w, cookie)
}

// SetSignedCookie writes an HMAC-SHA256 signed cookie.
func SetSignedCookie(w http.ResponseWriter, name, value, secret string) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(value))
	SetCookie(w, name, payload+"."+sign(payload, secret))
}

// GetSignedCookie verifies and returns a signed cookie, or "" when invalid.
func GetSignedCookie(r *http.Request, name, secret string) string {
	payload, signature, ok := strings.Cut(GetCookie(r, name), ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(sign(payload, secret))) {
		return ""
	}
	value, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	return string(value)
}

func newCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   2 * 60 * 60, // 2 hours
		HttpOnly: true,
	}
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
