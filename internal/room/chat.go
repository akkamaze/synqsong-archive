package room

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The longest chat message, in characters.
const chatMaxRunes = 300

// ChatBody is one chat line, trimmed, non-empty and free of control characters.
type ChatBody string

func ParseChatBody(raw string) (ChatBody, error) {
	if !utf8.ValidString(raw) {
		return "", invalid("chat message", "send text only")
	}

	s := strings.TrimSpace(raw)

	if s == "" {
		return "", invalid("chat message", "type something before sending")
	}

	if utf8.RuneCountInString(s) > chatMaxRunes {
		return "", invalid("chat message", "keep it to 300 characters")
	}

	for _, c := range s {
		if unicode.IsControl(c) && c != '\n' {
			return "", invalid("chat message", "remove control characters")
		}
	}

	return ChatBody(s), nil
}
