package auth

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"time"
)

// deviceLife matches the device cookie's lifetime; an older token counts as none.
const deviceLife = 365 * 24 * time.Hour

const (
	deviceIssuedBytes = 8
	deviceNonceBytes  = 16
	deviceMACBytes    = 32
	deviceTokenBytes  = deviceIssuedBytes + deviceNonceBytes + deviceMACBytes
)

// DeviceToken is what a browser keeps after signing in to an account (OWASP's device cookie).
// A later sign-in to that account that brings it is counted against the device's own lock
// rather than the username's, so guessing from elsewhere cannot lock the owner out. It grants
// no access: it holds the issue time, a random nonce and an HMAC over them, the username
// digest and the PIN's salt, so nothing is stored for it, it names no account, and a new PIN voids it.
type DeviceToken string

func (k Keys) newDeviceToken(usernameDigest []byte, pin PINHash, now time.Time, random io.Reader) (DeviceToken, error) {
	raw := make([]byte, deviceIssuedBytes, deviceTokenBytes)
	binary.BigEndian.PutUint64(raw, uint64(now.Unix()))

	nonce := make([]byte, deviceNonceBytes)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return "", fmt.Errorf("make device token: %w", err)
	}

	raw = append(raw, nonce...)
	raw = append(raw, k.deviceMAC(usernameDigest, pin, raw)...)

	return DeviceToken(base64.RawURLEncoding.EncodeToString(raw)), nil
}

// deviceID returns the nonce that names this device's lock, if t was issued for this
// username and its current PIN within deviceLife.
func (k Keys) deviceID(t DeviceToken, usernameDigest []byte, pin PINHash, now time.Time) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(string(t))
	if err != nil || len(raw) != deviceTokenBytes {
		return "", false
	}

	signed, sum := raw[:deviceIssuedBytes+deviceNonceBytes], raw[deviceIssuedBytes+deviceNonceBytes:]
	if !hmac.Equal(sum, k.deviceMAC(usernameDigest, pin, signed)) {
		return "", false
	}

	issued := time.Unix(int64(binary.BigEndian.Uint64(signed)), 0)
	if now.Sub(issued) >= deviceLife {
		return "", false
	}

	return string(signed[deviceIssuedBytes:]), true
}

func (k Keys) deviceMAC(usernameDigest []byte, pin PINHash, signed []byte) []byte {
	return mac(k.device, slices.Concat(usernameDigest, []byte(pin.Salt), signed))
}
