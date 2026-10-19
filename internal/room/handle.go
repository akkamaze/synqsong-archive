package room

import "strings"

const (
	handleMinLen = 3
	handleMaxLen = 32
)

// Handle is a room's public, unique name, as in /r/chill.
type Handle string

// ParseHandle accepts "  Chill-Room " as "chill-room".
func ParseHandle(raw string) (Handle, error) {
	s := strings.ToLower(strings.TrimSpace(raw))

	if len(s) < handleMinLen || len(s) > handleMaxLen {
		return "", invalid("room handle", "use 3 to 32 characters")
	}

	for _, c := range s {
		if !isHandleChar(c) {
			return "", invalid("room handle", "use only letters a-z, digits 0-9 and hyphens")
		}
	}

	if s[0] == '-' || s[len(s)-1] == '-' {
		return "", invalid("room handle", "start and end with a letter or digit")
	}

	return Handle(s), nil
}

func isHandleChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
}
