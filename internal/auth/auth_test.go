package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func testKeys(t *testing.T) Keys {
	t.Helper()

	keys, err := DeriveKeys(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}

	return keys
}

func TestParseUsernameRules(t *testing.T) {
	for _, ok := range []string{"ann_01", "Ann1", strings.Repeat("a", 20), "abcd"} {
		if _, err := ParseUsername(ok); err != nil {
			t.Errorf("ParseUsername(%q): %v, want accepted", ok, err)
		}
	}

	for _, bad := range []string{"", "abc", strings.Repeat("a", 21), "1234", "ann-01", "ann 01", "แอนนา"} {
		if _, err := ParseUsername(bad); err == nil {
			t.Errorf("ParseUsername(%q) accepted, want refused", bad)
		}
	}
}

func TestUsernamesIgnoreCase(t *testing.T) {
	keys := testKeys(t)
	upper, _ := ParseUsername("Ann1")
	lower, _ := ParseUsername("ann1")
	other, _ := ParseUsername("ann2")

	if !bytes.Equal(keys.UsernameDigest(upper), keys.UsernameDigest(lower)) {
		t.Fatal("Ann1 and ann1 have different digests, want one account")
	}

	if bytes.Equal(keys.UsernameDigest(lower), keys.UsernameDigest(other)) {
		t.Fatal("ann1 and ann2 share a digest")
	}
}

func TestAPINIsExactlyFourDigits(t *testing.T) {
	if _, err := ParsePIN("0042"); err != nil {
		t.Fatalf("ParsePIN(0042): %v", err)
	}

	for _, bad := range []string{"", "123", "12345", "123456", "12a4", "١٢٣٤", " 1234"} {
		if _, err := ParsePIN(bad); err == nil {
			t.Errorf("ParsePIN(%q) accepted, want exactly 4 digits", bad)
		}
	}
}

func digestOf(t *testing.T, keys Keys, raw string) []byte {
	t.Helper()

	u, err := ParseUsername(raw)
	if err != nil {
		t.Fatalf("parse username %q: %v", raw, err)
	}

	return keys.UsernameDigest(u)
}

func TestAPINHashVerifiesOnlyItsPIN(t *testing.T) {
	keys := testKeys(t)
	ann := digestOf(t, keys, "ann_1")
	pin, _ := ParsePIN("4821")

	hash, err := keys.HashPIN(pin, ann, rand.Reader)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if !keys.VerifyPIN(pin, ann, hash) {
		t.Fatal("the right PIN did not verify")
	}

	wrong, _ := ParsePIN("4822")
	if keys.VerifyPIN(wrong, ann, hash) {
		t.Fatal("a wrong PIN verified")
	}

	other, _ := DeriveKeys(bytes.Repeat([]byte{8}, 32))
	if other.VerifyPIN(pin, ann, hash) {
		t.Fatal("the PIN verified with another key, want the key outside the database to be required")
	}
}

func TestAPINHashCopiedToAnotherUsernameDoesNotVerify(t *testing.T) {
	keys := testKeys(t)
	pin, _ := ParsePIN("4821")

	mallorys, err := keys.HashPIN(pin, digestOf(t, keys, "mallory"), rand.Reader)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if keys.VerifyPIN(pin, digestOf(t, keys, "ann_1"), mallorys) {
		t.Fatal("a PIN hash copied onto another username verified, want it bound to its own username")
	}
}

// Made with Node's own call: node -e 'require("node:crypto").scrypt("4821", salt, 64, ...)' on Node 22.
const (
	legacySalt = "00112233445566778899aabbccddeeff"
	legacyHash = "8c3230cd69d5aff30375aba10320f96f930eba068def013e6fb4a0c1c80a1a7db7fe77331cbff2ea1083fa6a65432c8aad58fdd6bfef9636c22d8ad1dfabd5bb"
)

func TestScryptMatchesTheLegacyVector(t *testing.T) {
	derived, err := scryptPIN("4821", legacySalt)
	if err != nil || hex.EncodeToString(derived) != legacyHash {
		t.Fatalf("scrypt(4821, salt) = %x, %v; want the legacy %s", derived, err, legacyHash)
	}
}

func TestLegacyPINHashesStillVerifyAfterImport(t *testing.T) {
	keys := testKeys(t)
	ann := digestOf(t, keys, "ann_1")

	imported, err := keys.ImportLegacyPIN(ann, legacySalt, legacyHash)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	pin, _ := ParsePIN("4821")
	if !keys.VerifyPIN(pin, ann, imported) {
		t.Fatal("a legacy user's PIN does not verify after import")
	}

	wrong, _ := ParsePIN("1111")
	if keys.VerifyPIN(wrong, ann, imported) {
		t.Fatal("a wrong PIN verified against an imported hash")
	}

	if keys.VerifyPIN(pin, digestOf(t, keys, "bob_1"), imported) {
		t.Fatal("an imported hash verified for another username")
	}

	if bytes.Contains(imported.Digest, []byte(legacyHash)) {
		t.Fatal("the stored digest still holds the legacy raw hash")
	}
}

func TestTokensAreRandomAndStoredOnlyAsKeyedDigests(t *testing.T) {
	keys := testKeys(t)

	a, err := NewToken(rand.Reader, 7)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	b, _ := NewToken(rand.Reader, 7)

	if a == b || len(a) < 40 {
		t.Fatalf("tokens %q and %q, want long and different", a, b)
	}

	if bytes.Contains(keys.TokenDigest(a), []byte(a)) || !bytes.Equal(keys.TokenDigest(a), keys.TokenDigest(a)) {
		t.Fatal("token digest leaks the token or is not stable")
	}

	plain := sha256.Sum256([]byte(a))
	other, _ := DeriveKeys(bytes.Repeat([]byte{8}, 32))

	if bytes.Equal(keys.TokenDigest(a), plain[:]) || bytes.Equal(keys.TokenDigest(a), other.TokenDigest(a)) {
		t.Fatal("the token digest can be made without the key, so anyone writing SQL could mint a session")
	}
}

func TestDeriveKeysNeedsEnoughSecret(t *testing.T) {
	if _, err := DeriveKeys(make([]byte, 31)); err == nil {
		t.Fatal("a 31-byte secret was accepted, want at least 32 bytes")
	}
}
