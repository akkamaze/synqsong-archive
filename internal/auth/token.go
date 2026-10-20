package auth

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"

	"synqsong/internal/account"
)

const (
	tokenAccountBytes = 8
	tokenRandomBytes  = 32
)

// Token is the session secret in the browser's cookie: the account id and 32 random bytes.
// Only its keyed digest is stored, and the digest covers the id, so a session row moved to
// another account no longer matches the token that finds it.
type Token string

func NewToken(random io.Reader, owner account.ID) (Token, error) {
	raw := make([]byte, tokenAccountBytes+tokenRandomBytes)
	binary.BigEndian.PutUint64(raw, uint64(owner))

	if _, err := io.ReadFull(random, raw[tokenAccountBytes:]); err != nil {
		return "", fmt.Errorf("make session token: %w", err)
	}

	return Token(base64.RawURLEncoding.EncodeToString(raw)), nil
}

// owner is the account the token was made for, if it is well formed.
func (t Token) owner() (account.ID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(string(t))
	if err != nil || len(raw) != tokenAccountBytes+tokenRandomBytes {
		return 0, false
	}

	return account.ID(binary.BigEndian.Uint64(raw)), true
}

// TokenDigest is how a session is stored. It needs the key, so a session row cannot be
// forged by writing to the database.
func (k Keys) TokenDigest(t Token) []byte {
	return mac(k.token, []byte(t))
}
