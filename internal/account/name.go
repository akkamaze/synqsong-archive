package account

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// The longest display name, counted in characters.
const displayNameMaxRunes = 40

// DisplayName is what others in a room see. It is shown to everyone, so it is not secret.
type DisplayName string

var (
	errNameEmpty     = errors.New("display name: type a name")
	errNameTooLong   = errors.New("display name: keep it to 40 characters")
	errNameHidden    = errors.New("display name: remove line breaks and invisible characters")
	errNameNoVisible = errors.New("display name: include at least one letter, digit, punctuation mark or symbol")
	errNameStacked   = errors.New("display name: use at most 4 accent marks on one letter")
)

func ParseDisplayName(raw string) (DisplayName, error) {
	if !utf8.ValidString(raw) {
		return "", errNameHidden
	}

	s := strings.TrimSpace(raw)

	if s == "" {
		return "", errNameEmpty
	}

	if utf8.RuneCountInString(s) > displayNameMaxRunes {
		return "", errNameTooLong
	}

	if err := checkCharacters([]rune(s)); err != nil {
		return "", err
	}

	return DisplayName(s), nil
}
