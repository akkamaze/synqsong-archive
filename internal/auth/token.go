package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

const tokenBytes = 32

// Token is the session secret in the browser's cookie. Only its hash is stored.
type Token string

func NewToken(random io.Reader) (Token, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", fmt.Errorf("make session token: %w", err)
	}

	return Token(base64.RawURLEncoding.EncodeToString(raw)), nil
}

func HashToken(t Token) []byte {
	sum := sha256.Sum256([]byte(t))

	return sum[:]
}
