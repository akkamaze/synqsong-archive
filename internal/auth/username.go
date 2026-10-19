package auth

import (
	"errors"
	"strings"
)

const (
	usernameMinLen = 4
	usernameMaxLen = 20
)

// Username is what someone types to sign in. Case does not matter: Ann1 and ann1 are one account.
type Username string

var (
	errUsernameChars  = errors.New("username: use only letters, digits and underscore")
	errUsernameLength = errors.New("username: use 4 to 20 characters")
	errUsernameDigits = errors.New("username: include at least one letter or underscore")
)

// ParseUsername applies the username rules: a-z, 0-9 and _, 4 to 20 long, not all digits.
func ParseUsername(raw string) (Username, error) {
	if len(raw) < usernameMinLen || len(raw) > usernameMaxLen {
		return "", errUsernameLength
	}

	allDigits := true

	for _, c := range raw {
		if !isUsernameChar(c) {
			return "", errUsernameChars
		}

		if c < '0' || c > '9' {
			allDigits = false
		}
	}

	if allDigits {
		return "", errUsernameDigits
	}

	return Username(strings.ToLower(raw)), nil
}

func isUsernameChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// UsernameDigest is how a username is stored and looked up; the name itself is never saved.
func (k Keys) UsernameDigest(u Username) []byte {
	return mac(k.username, []byte(u))
}
