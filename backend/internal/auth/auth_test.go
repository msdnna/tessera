package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	secret := "test-secret-at-least-32-characters-long!!"
	id := uuid.New()

	tok, err := NewAccessToken(secret, id)
	if err != nil {
		t.Fatalf("NewAccessToken: %v", err)
	}

	got, err := ParseAccessToken(secret, tok)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if got != id {
		t.Fatalf("subject mismatch: got %v want %v", got, id)
	}
}

func TestAccessTokenWrongSecret(t *testing.T) {
	tok, err := NewAccessToken("secret-one-at-least-32-characters-xxxx", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAccessToken("secret-two-at-least-32-characters-yyyy", tok); err == nil {
		t.Fatal("expected error for token signed with a different secret")
	}
}

func TestRefreshTokenHashing(t *testing.T) {
	tok, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" || hash == "" {
		t.Fatal("empty token or hash")
	}
	if hash == tok {
		t.Fatal("hash must differ from raw token")
	}
	if HashRefreshToken(tok) != hash {
		t.Fatal("HashRefreshToken not deterministic with NewRefreshToken")
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("CheckPassword rejected the correct password")
	}
	if CheckPassword(hash, "wrong password") {
		t.Fatal("CheckPassword accepted a wrong password")
	}
}
