package auth

import (
	"net/http"
	"time"
)

// The __Host- prefix makes browsers refuse the cookie unless it is Secure, has Path=/ and no
// Domain, so no other site under synqloud.com can set or read it.
const (
	cookieName       = "__Host-synqsong"
	deviceCookieName = "__Host-synqsong-device"
)

// SessionCookie's MaxAge is fixed when sent, so the web layer re-sends it on an authenticated response at most once a day to keep pace with the sliding idle timeout.
func SessionCookie(t Token) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    string(t),
		Path:     "/",
		MaxAge:   int(sessionIdle / time.Second),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func ClearedCookie() *http.Cookie {
	return &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func TokenFrom(r *http.Request) (Token, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return "", false
	}

	return Token(c.Value), true
}

// DeviceCookie keeps the device token as long as the token is valid. It is re-sent with every
// new session, since each sign-in issues a fresh token.
func DeviceCookie(t DeviceToken) *http.Cookie {
	return &http.Cookie{
		Name:     deviceCookieName,
		Value:    string(t),
		Path:     "/",
		MaxAge:   int(deviceLife / time.Second),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// DeviceTokenFrom returns the browser's device token, or an empty one when it has none.
func DeviceTokenFrom(r *http.Request) DeviceToken {
	c, err := r.Cookie(deviceCookieName)
	if err != nil {
		return ""
	}

	return DeviceToken(c.Value)
}
