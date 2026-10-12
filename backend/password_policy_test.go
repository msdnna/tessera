// A password has an upper bound nobody declared (#2932).
//
// bcrypt refuses anything over 72 *bytes* — x/crypto returns ErrPasswordTooLong
// rather than truncating — and all three doors that set a password (register,
// reset-from-email, change-in-session) handed that error straight to the 500
// branch. The user read "internal error" and had no way to know the passphrase
// was the problem, while the log filled with a failure that is not ours.
//
// Bytes, not characters, is what makes it reachable: a Russian passphrase of 37
// letters is 74 bytes, well inside what anyone would call a normal password.
//
// These pin the two halves of the contract — over the ceiling is a 400 the user
// can act on, and exactly at the ceiling still works end to end — plus the lower
// bound's unit, which is runes (the validator counts them that way) and must not
// quietly become bytes when the ceiling is added.
package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// registerPassword registers a fresh user with the given password and returns
// the raw response, so a rejected password is a value to assert on rather than a
// fatal inside a helper.
func registerPassword(t *testing.T, label, password string) resp {
	t.Helper()
	n := userSeq.Add(1)
	email := fmt.Sprintf("pw-%s-%d@test.local", label, n)
	return doReq(t, "", http.MethodPost, "/auth/register", map[string]any{
		"email": email, "name": "PW User", "password": password,
	})
}

// cyrillic is a passphrase of n Cyrillic letters — 2*n bytes, which is the whole
// reason the ceiling is reachable without anyone typing anything unusual.
func cyrillic(n int) string { return strings.Repeat("п", n) }

// TestRegisterRejectsOverlongPasswordWithoutFiveHundred is the reachable case:
// a passphrase a password manager or a Russian speaker would produce.
func TestRegisterRejectsOverlongPasswordWithoutFiveHundred(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		password string
	}{
		{"ascii 73 bytes", strings.Repeat("a", 73)},
		{"cyrillic 37 letters, 74 bytes", cyrillic(37)},
		{"long generated passphrase", strings.Repeat("correct-horse-", 8)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := registerPassword(t, "long", tc.password)
			if r.Status != http.StatusBadRequest {
				t.Fatalf("register with %d-byte password: status %d, want 400\n%s",
					len(tc.password), r.Status, r.Body)
			}
			// The body must say something about the password. "internal error"
			// is the exact failure this test exists to keep out: it tells the
			// user nothing and points the operator at the wrong side.
			if strings.Contains(string(r.Body), "internal error") {
				t.Fatalf("overlong password answered with the generic 500 text: %s", r.Body)
			}
		})
	}
}

// TestPasswordAtBcryptCeilingStillWorks is the negative control. The fix must
// refuse what bcrypt refuses and not one byte less — a ceiling set at 64 or at
// 72 runes would pass the test above while locking out passwords that work.
func TestPasswordAtBcryptCeilingStillWorks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		password string
	}{
		{"ascii 72 bytes", strings.Repeat("a", 72)},
		{"cyrillic 36 letters, 72 bytes", cyrillic(36)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := registerPassword(t, "ceil", tc.password)
			if r.Status != http.StatusOK && r.Status != http.StatusCreated {
				t.Fatalf("register with a 72-byte password: status %d\n%s", r.Status, r.Body)
			}
			// Registering is half of it — the stored hash has to accept the same
			// bytes back, otherwise the account is created and unreachable.
			email := r.mapBody(t)["user"].(map[string]any)["email"].(string)
			if lr := login(t, email, tc.password); lr.Status != http.StatusOK {
				t.Fatalf("login with a 72-byte password: status %d\n%s", lr.Status, lr.Body)
			}
		})
	}
}

// TestPasswordMinimumStaysInRunes guards the lower bound's unit while the upper
// one is added. Eight Cyrillic letters are 16 bytes and 8 runes: a minimum
// retold in bytes would keep accepting it, so the seven-letter case is what
// actually holds the line.
func TestPasswordMinimumStaysInRunes(t *testing.T) {
	t.Parallel()

	if r := registerPassword(t, "min-ok", cyrillic(8)); r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("8 Cyrillic letters (8 runes, 16 bytes): status %d, want accepted\n%s", r.Status, r.Body)
	}
	if r := registerPassword(t, "min-short", cyrillic(7)); r.Status != http.StatusBadRequest {
		t.Fatalf("7 Cyrillic letters (7 runes, 14 bytes): status %d, want 400\n%s", r.Status, r.Body)
	}
}

// TestChangePasswordRejectsOverlongPassword covers the in-session door, and the
// part that matters beyond the status code: a refused change must leave the
// account exactly as it was. The handler revokes every session before it is done
// hashing, so a 500 halfway through is not only an unhelpful message.
func TestChangePasswordRejectsOverlongPassword(t *testing.T) {
	t.Parallel()
	c := signup(t)

	r := c.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": cyrillic(37),
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("change to a 74-byte password: status %d, want 400\n%s", r.Status, r.Body)
	}
	if strings.Contains(string(r.Body), "internal error") {
		t.Fatalf("overlong password answered with the generic 500 text: %s", r.Body)
	}
	if lr := login(t, c.Email, "password-123"); lr.Status != http.StatusOK {
		t.Fatalf("the old password stopped working after a refused change: status %d\n%s", lr.Status, lr.Body)
	}
}

// TestResetPasswordRejectsOverlongPassword covers the third door. The reset
// token is single-use, so a request that fails on the password must not spend
// it — otherwise the user's one chance to get back in is burned by a typo.
func TestResetPasswordRejectsOverlongPassword(t *testing.T) {
	t.Parallel()
	c := signup(t)
	raw := insertUserToken(t, c.UserID, "reset")

	r := doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{
		"token": raw, "new_password": cyrillic(37),
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("reset to a 74-byte password: status %d, want 400\n%s", r.Status, r.Body)
	}
	if strings.Contains(string(r.Body), "internal error") {
		t.Fatalf("overlong password answered with the generic 500 text: %s", r.Body)
	}

	// The token survived, so the second attempt — with a password that fits —
	// still works.
	r = doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{
		"token": raw, "new_password": "reset-password-456",
	})
	if r.Status != http.StatusNoContent {
		t.Fatalf("reset after a refused one: status %d, want 204\n%s", r.Status, r.Body)
	}
	if lr := login(t, c.Email, "reset-password-456"); lr.Status != http.StatusOK {
		t.Fatalf("new password does not log in: status %d\n%s", lr.Status, lr.Body)
	}
}
