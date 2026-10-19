// Package auth signs people in with a username and PIN, and keeps sessions.
package auth

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
)

const secretMinLen = 32

// Keys come from one secret kept outside the database and its backups. Without them a
// stolen database cannot be used to test usernames or PINs, even though PINs are short.
type Keys struct {
	username []byte
	pin      []byte
}

var errShortSecret = errors.New("auth secret: use at least 32 random bytes")

func DeriveKeys(secret []byte) (Keys, error) {
	if len(secret) < secretMinLen {
		return Keys{}, errShortSecret
	}

	username, err := hkdf.Key(sha256.New, secret, nil, "synqsong username v1", 32)
	if err != nil {
		return Keys{}, fmt.Errorf("derive username key: %w", err)
	}

	pin, err := hkdf.Key(sha256.New, secret, nil, "synqsong pin v1", 32)
	if err != nil {
		return Keys{}, fmt.Errorf("derive pin key: %w", err)
	}

	return Keys{username: username, pin: pin}, nil
}

func mac(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)

	return m.Sum(nil)
}
