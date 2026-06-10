// Package secrets provides authenticated symmetric encryption for secrets
// stored at rest (e.g. GitLab personal access tokens). It uses AES-256-GCM with
// a key derived from a configured passphrase via SHA-256, so any passphrase
// length is accepted.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Sealer encrypts and decrypts short secrets with a fixed key.
type Sealer struct {
	gcm cipher.AEAD
}

// NewSealer derives a 256-bit key from passphrase (SHA-256) and prepares an
// AES-GCM cipher. passphrase must be non-empty.
func NewSealer(passphrase string) (*Sealer, error) {
	if passphrase == "" {
		return nil, errors.New("crypto: empty passphrase")
	}
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{gcm: gcm}, nil
}

// Encrypt seals plaintext and returns base64(nonce||ciphertext).
func (s *Sealer) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. It fails on a wrong key or tampered ciphertext.
func (s *Sealer) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	ns := s.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := s.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
