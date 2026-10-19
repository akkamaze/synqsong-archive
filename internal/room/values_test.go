package room

import (
	"errors"
	"strings"
	"testing"
)

func TestParseHandle(t *testing.T) {
	accepted := map[string]Handle{
		"chill":                 "chill",
		"  Chill-Room ":         "chill-room",
		"abc":                   "abc",
		"r2d2":                  "r2d2",
		strings.Repeat("a", 32): Handle(strings.Repeat("a", 32)),
	}

	for raw, want := range accepted {
		got, err := ParseHandle(raw)
		if err != nil || got != want {
			t.Errorf("ParseHandle(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	rejected := []string{"", "ab", strings.Repeat("a", 33), "-chill", "chill-", "chill room", "ห้องชิล", "chill_room", "chill.room"}

	for _, raw := range rejected {
		if got, err := ParseHandle(raw); err == nil {
			t.Errorf("ParseHandle(%q) = %q, want an error", raw, got)
		}
	}
}

func TestParseVideoID(t *testing.T) {
	for _, raw := range []string{"dQw4w9WgXcQ", "a-b_c-d_e-f", "00000000000"} {
		if got, err := ParseVideoID(raw); err != nil || string(got) != raw {
			t.Errorf("ParseVideoID(%q) = %q, %v; want it unchanged", raw, got, err)
		}
	}

	rejected := []string{"", "dQw4w9WgXc", "dQw4w9WgXcQQ", "dQw4w9WgXc!", "https://youtu.be/dQw4w9WgXcQ", " dQw4w9WgXcQ"}

	for _, raw := range rejected {
		if got, err := ParseVideoID(raw); err == nil {
			t.Errorf("ParseVideoID(%q) = %q, want an error", raw, got)
		}
	}
}

func TestParseChatBody(t *testing.T) {
	accepted := map[string]ChatBody{
		"  hello  ": "hello",
		"เพลงนี้เพราะมาก 🎶": "เพลงนี้เพราะมาก 🎶",
		"line one\nline two":     "line one\nline two",
		strings.Repeat("ก", 300): ChatBody(strings.Repeat("ก", 300)),
	}

	for raw, want := range accepted {
		got, err := ParseChatBody(raw)
		if err != nil || got != want {
			t.Errorf("ParseChatBody(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	rejected := []string{"", "   ", "\n\t", strings.Repeat("ก", 301), "bell\a", "null\x00byte", "bad \xff utf8"}

	for _, raw := range rejected {
		if got, err := ParseChatBody(raw); err == nil {
			t.Errorf("ParseChatBody(%q) = %q, want an error", raw, got)
		}
	}
}

func TestInvalidErrorNeverEchoesInput(t *testing.T) {
	secret := "my-private-note-" + strings.Repeat("x", 600)

	_, err := ParseChatBody(secret)

	var invalidErr *InvalidError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("ParseChatBody error = %v, want *InvalidError", err)
	}

	if strings.Contains(err.Error(), "my-private-note") {
		t.Fatalf("error %q repeats the user's input", err)
	}
}
