package castor

import (
	"encoding/json/v2"
	"net/http"
	"net/url"
	"time"
)

const (
	flashCookieName = "WG_FLASH"
	flashLifetime   = 2 * time.Second
)

// FlashMessage is a single flash key/value pair.
type FlashMessage struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Message creates a flash message.
func Message(key, value string) FlashMessage {
	return FlashMessage{Key: key, Value: value}
}

// FlashWriter writes flash messages to a cookie.
type FlashWriter struct {
	w http.ResponseWriter
}

// Flash starts the flash-message DSL.
func Flash(w http.ResponseWriter) FlashWriter {
	return FlashWriter{w: w}
}

// Messages stores messages in a URL-escaped JSON cookie that expires after two seconds.
func (f FlashWriter) Messages(messages ...FlashMessage) {
	data, err := json.Marshal(messages)
	if err != nil {
		return
	}

	http.SetCookie(f.w, &http.Cookie{
		Name:     flashCookieName,
		Value:    url.QueryEscape(string(data)),
		Path:     "/",
		MaxAge:   int(flashLifetime / time.Second),
		Expires:  time.Now().Add(flashLifetime),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
