package auth

import (
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash at the default cost.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// dummyHash is a real bcrypt hash of a throwaway secret at the same cost as any
// stored one. Comparing an unknown password against it costs a full bcrypt
// round, which is the whole point: when a login lookup finds no row, the handler
// still runs bcrypt against this, so the missing-user response is
// indistinguishable by timing from the wrong-password one. Without it the
// missing-user path returned in microseconds (no bcrypt at all), letting an
// attacker enumerate emails with a stopwatch.
//
// Computed lazily rather than at init: it costs a bcrypt round, and processes
// that never serve a login (the migration runner, tooling) should not pay it.
var dummyHash = sync.OnceValue(func() string {
	h, err := bcrypt.GenerateFromPassword([]byte("tessera-dummy-account-not-found"), bcrypt.DefaultCost)
	if err != nil {
		// GenerateFromPassword errors only on an invalid cost; DefaultCost is valid.
		panic(err)
	}
	return string(h)
})

// DummyHash returns a valid bcrypt hash not tied to any account, so a caller can
// run bcrypt even when no real hash exists and keep response timing uniform.
// See handlers.Login.
func DummyHash() string { return dummyHash() }
