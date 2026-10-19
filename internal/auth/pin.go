package auth

import (
	"crypto/hmac"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

// Node's scrypt defaults with a 64-byte key, so legacy PINs still verify.
const (
	scryptN      = 16384
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 64
	saltBytes    = 16
)

const pinLen = 4

// PIN is the 4 digits typed at sign-in. Longer PINs are refused on purpose:
// 6 digits is what banks and government apps use, and people reuse those.
type PIN string

var errPINFormat = errors.New("PIN: use exactly 4 digits")

func ParsePIN(raw string) (PIN, error) {
	if len(raw) != pinLen || !allASCIIDigits(raw) {
		return "", errPINFormat
	}

	return PIN(raw), nil
}

func allASCIIDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return s != ""
}

// PINHash is what is stored: the salt text and HMAC(key, scrypt(pin, salt)).
type PINHash struct {
	Salt   string
	Digest []byte
}

func (k Keys) HashPIN(pin PIN, random io.Reader) (PINHash, error) {
	raw := make([]byte, saltBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return PINHash{}, fmt.Errorf("make PIN salt: %w", err)
	}

	salt := hex.EncodeToString(raw)

	derived, err := scryptPIN(pin, salt)
	if err != nil {
		return PINHash{}, err
	}

	return PINHash{Salt: salt, Digest: mac(k.pin, derived)}, nil
}

func (k Keys) VerifyPIN(pin PIN, stored PINHash) bool {
	derived, err := scryptPIN(pin, stored.Salt)
	if err != nil {
		return false
	}

	return hmac.Equal(mac(k.pin, derived), stored.Digest)
}

// ImportLegacyPIN wraps a legacy hash (hex scrypt output and its salt text) so the user keeps their PIN.
func (k Keys) ImportLegacyPIN(salt, hashHex string) (PINHash, error) {
	derived, err := hex.DecodeString(hashHex)
	if err != nil || len(derived) != scryptKeyLen {
		return PINHash{}, errors.New("legacy PIN hash: expected 128 hex characters")
	}

	return PINHash{Salt: salt, Digest: mac(k.pin, derived)}, nil
}

// The salt's hex text, not its bytes, goes to scrypt; this keeps legacy hashes valid.
func scryptPIN(pin PIN, salt string) ([]byte, error) {
	derived, err := scrypt.Key([]byte(pin), []byte(salt), scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("derive PIN: %w", err)
	}

	return derived, nil
}
