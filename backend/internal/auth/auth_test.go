package auth

import (
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

// The anti-enumeration measure in handlers.Login only works if bcrypt against
// DummyHash costs the same as bcrypt against a stored hash — i.e. same cost
// factor. A dummy hash at a lower cost would still leave a measurable timing
// gap between "no such email" and "wrong password".
func TestDummyHashMatchesRealCost(t *testing.T) {
	stored, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	storedCost, err := bcrypt.Cost([]byte(stored))
	if err != nil {
		t.Fatalf("cost of a stored hash: %v", err)
	}
	dummy := DummyHash()
	dummyCost, err := bcrypt.Cost([]byte(dummy))
	if err != nil {
		t.Fatalf("DummyHash is not a valid bcrypt hash: %v", err)
	}
	if dummyCost != storedCost {
		t.Fatalf("DummyHash cost = %d, want %d (same as stored hashes)", dummyCost, storedCost)
	}
	if CheckPassword(dummy, "correct horse battery staple") {
		t.Fatal("DummyHash must not accept a password anyone might actually use")
	}
	if again := DummyHash(); again != dummy {
		t.Fatal("DummyHash must be stable across calls")
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
