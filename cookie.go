package castor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

type cookie struct {
	name     string
	value    string
	path     string
	httpOnly bool
	secure   bool
	maxAge   int
	expires  int
	sameSite http.SameSite
}

func (c *cookie) IsHttpOnly(val bool) *cookie {
	c.httpOnly = val
	return c
}

func (c *cookie) IsSecure(val bool) *cookie {
	c.secure = val
	return c
}

func (c *cookie) SetPath(val string) *cookie {
	c.path = val
	return c
}

func (c *cookie) SetMaxAge(val int) *cookie {
	c.maxAge = val
	return c
}

func (c *cookie) SetSameSite(val http.SameSite) *cookie {
	c.sameSite = val
	return c
}

func (c *cookie) Response(res http.ResponseWriter) {
	http.SetCookie(res, &http.Cookie{
		Name:     c.name,
		Value:    c.value,
		Path:     c.path,
		MaxAge:   c.maxAge,
		HttpOnly: c.httpOnly,
		Secure:   c.secure,
		SameSite: c.sameSite,
	})
}

// GetCookie returns the named cookie's value, or "" when it does not exist.
func GetCookie(req *http.Request, name string) string {
	cookie, err := req.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// SetCookie writes a root-path, HTTP-only cookie.
func AddCookie(name, value string) *cookie {
	c := &cookie{name: name, value: value, path: "/", maxAge: 2 * 60 * 60, httpOnly: true, secure: false}
	return c
}

// RemoveCookie expires the named cookie.
func RemoveCookie(res http.ResponseWriter, name string) {
	cookie := &http.Cookie{
		Name:   name,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(1, 0)
	http.SetCookie(res, cookie)
}

// SetSignedCookie writes an HMAC-SHA256 signed cookie.
func AddSignedCookie(name, value, secret string) *cookie {
	payload := base64.RawURLEncoding.EncodeToString([]byte(value))
	c := &cookie{
		name:  name,
		value: payload + "." + sign(payload, secret),
		path:  "/", maxAge: 2 * 60 * 60,
		httpOnly: true,
		secure:   false,
	}
	return c
}

// GetSignedCookie verifies and returns a signed cookie, or "" when invalid.
func GetSignedCookie(req *http.Request, name, secret string) string {
	payload, signature, ok := strings.Cut(GetCookie(req, name), ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(sign(payload, secret))) {
		return ""
	}
	value, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	return string(value)
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
