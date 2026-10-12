package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestSealerRoundTrip(t *testing.T) {
	s, err := NewSealer("some-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	const token = "glpat-xxxxxxxxxxxxxxxxxxxx"
	enc, err := s.Encrypt(token)
	if err != nil {
		t.Fatal(err)
	}
	if enc == token {
		t.Fatal("ciphertext equals plaintext")
	}
	got, err := s.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != token {
		t.Fatalf("decrypt = %q, want %q", got, token)
	}
}

func TestSealerWrongKeyFails(t *testing.T) {
	a, _ := NewSealer("key-a")
	b, _ := NewSealer("key-b")
	enc, _ := a.Encrypt("secret")
	if _, err := b.Decrypt(enc); err == nil {
		t.Fatal("expected decrypt with wrong key to fail")
	}
}

func TestNewSealerRejectsEmpty(t *testing.T) {
	if _, err := NewSealer(""); err == nil {
		t.Fatal("expected error for empty passphrase")
	}
}

// The key derivation is part of the stored data's format: every token already
// sealed at rest was sealed under SHA-256(ENCRYPTION_KEY). Seal one by hand the
// way the sealer always has and make sure it still opens — a "stronger" KDF
// swapped in here would strand every existing ciphertext.
func TestSealerKeepsItsKeyDerivation(t *testing.T) {
	const keyMaterial, plain = "deployed-encryption-key", "glpat-legacy"
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	legacy := base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil))

	s, err := NewSealer(keyMaterial)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Decrypt(legacy)
	if err != nil || got != plain {
		t.Fatalf("Decrypt(legacy) = %q, %v; want %q", got, err, plain)
	}
}
