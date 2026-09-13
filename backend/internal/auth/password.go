package auth

import (
	"errors"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordBytes is bcrypt's own ceiling, re-exported so the handlers can
// refuse a password before hashing it rather than after.
//
// It is bytes, not characters, and that is what makes it reachable: a Russian
// passphrase of 37 letters is 74 bytes. x/crypto returns ErrPasswordTooLong
// instead of truncating, so anything over this arrived at the callers as an
// error indistinguishable from a real failure and became a 500 (#2932).
//
// MinPasswordRunes is the other end, in the unit the request binding already
// counts (`binding:"min=8"` is runes). Both live here so the two doors that set
// a password and the one that resets it cannot drift apart.
const (
	MaxPasswordBytes = 72
	MinPasswordRunes = 8
)

// ErrPasswordTooLong is returned by CheckPasswordPolicy for a password bcrypt
// cannot hash. Named rather than inlined so a caller can tell it apart from a
// password that is merely too short.
var ErrPasswordTooLong = errors.New("password exceeds 72 bytes")

// ErrPasswordTooShort is returned by CheckPasswordPolicy below the minimum.
var ErrPasswordTooShort = errors.New("password is shorter than 8 characters")

// CheckPasswordPolicy reports whether a new password can be stored.
//
// Both bounds are checked here even though the binding tag already covers the
// lower one: ResetPassword and ChangeMyPassword share this, and a rule enforced
// in two places with two different units is how the upper bound went missing in
// the first place.
func CheckPasswordPolicy(plain string) error {
	if utf8.RuneCountInString(plain) < MinPasswordRunes {
		return ErrPasswordTooShort
	}
	if len(plain) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

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
