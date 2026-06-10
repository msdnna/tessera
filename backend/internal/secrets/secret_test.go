package secrets

import "testing"

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
