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
// stolen database cannot be used to test usernames or PINs, even though PINs are short,
// and rows written by someone with database access cannot sign anyone in.
type Keys struct {
	username []byte
	pin      []byte
	token    []byte
	device   []byte
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

	token, err := hkdf.Key(sha256.New, secret, nil, "synqsong session token v1", 32)
	if err != nil {
		return Keys{}, fmt.Errorf("derive session token key: %w", err)
	}

	device, err := hkdf.Key(sha256.New, secret, nil, "synqsong device token v1", 32)
	if err != nil {
		return Keys{}, fmt.Errorf("derive device token key: %w", err)
	}

	return Keys{username: username, pin: pin, token: token, device: device}, nil
}

func mac(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)

	return m.Sum(nil)
}
