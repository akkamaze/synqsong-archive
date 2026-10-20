package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTheDeviceCookieIsAHostCookieKeptForAYear(t *testing.T) {
	c := DeviceCookie("token")

	if !strings.HasPrefix(c.Name, "__Host-") || c.Path != "/" || c.Domain != "" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("device cookie = %+v, want __Host-, Path=/, no Domain, Secure, HttpOnly, SameSite=Lax", c)
	}

	if c.Name == cookieName {
		t.Fatal("the device cookie shares the session cookie's name")
	}

	if got := time.Duration(c.MaxAge) * time.Second; got < 360*24*time.Hour || got > 366*24*time.Hour {
		t.Fatalf("device cookie MaxAge = %v, want about a year", got)
	}

	r := httptest.NewRequest(http.MethodPost, "/sign-in", nil)
	r.AddCookie(c)

	if got := DeviceTokenFrom(r); got != "token" {
		t.Fatalf("device token from the request = %q, want %q", got, "token")
	}

	if got := DeviceTokenFrom(httptest.NewRequest(http.MethodPost, "/sign-in", nil)); got != "" {
		t.Fatalf("device token without the cookie = %q, want none", got)
	}
}
