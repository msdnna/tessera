// Confirmation codes and the address change (#3035: #3057, #3058).
//
// The six digits are short on purpose, so these tests are mostly about what
// surrounds them: the hash at rest, the attempt budget, supersession, the
// resend pause and cap, and an address change that neither leaks who is
// registered nor leaves the old mailbox holding live links.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// ageCodes moves a user's codes into the past, standing in for the resend
// pause going by.
func ageCodes(t *testing.T, userID, by string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE user_tokens SET created_at = created_at - $2::interval
		 WHERE user_id = $1 AND kind IN ('verify_code', 'email_change')`, userID, by); err != nil {
		t.Fatalf("age codes: %v", err)
	}
}

// verifyByCode confirms the user's address with the code from their sign-up
// letter.
func verifyByCode(t *testing.T, c *client) {
	t.Helper()
	code := testMail.waitCode(t, c.Email)
	c.expect(t, c.post("/auth/verify-code", map[string]any{"code": code}), http.StatusOK)
}

// A wrong guess counts and says what is left; the right code proves the
// address, and the code never sits in the database in the clear.
func TestVerifyCodeConfirmsTheAddress(t *testing.T) {
	t.Parallel()
	u := signup(t)
	letter := testMail.waitLetter(t, u.Email, 0)
	code := codeIn(t, letter)
	if !strings.Contains(letter.body, "/verify-email?token=") {
		t.Fatalf("the letter must still carry the link:\n%s", letter.body)
	}

	var stored string
	if err := testPool.QueryRow(context.Background(),
		`SELECT token_hash FROM user_tokens WHERE user_id = $1 AND kind = 'verify_code'`, u.UserID).Scan(&stored); err != nil {
		t.Fatalf("read code row: %v", err)
	}
	if strings.Contains(stored, code) || !strings.HasPrefix(stored, "c1:") {
		t.Fatalf("code must be stored as a salted MAC, got %q", stored)
	}

	st := u.expect(t, u.get("/auth/verification"), http.StatusOK)
	if st["target"] != u.Email || st["verified"] != false || st["attempts_left"].(float64) != 5 {
		t.Fatalf("unexpected state %v", st)
	}
	if st["resend_in"].(float64) <= 0 {
		t.Fatalf("a code was just sent, resend must wait: %v", st)
	}

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	r := u.post("/auth/verify-code", map[string]any{"code": wrong})
	m := u.expect(t, r, http.StatusBadRequest)
	if m["error"] != "invalid code" || m["attempts_left"].(float64) != 4 {
		t.Fatalf("wrong code: %v", m)
	}
	// Pasted with a space in the middle, the way mail clients copy it.
	m = u.expect(t, u.post("/auth/verify-code", map[string]any{"code": code[:3] + " " + code[3:]}), http.StatusOK)
	if m["user"].(map[string]any)["email_verified"] != true {
		t.Fatalf("verified flag not set: %v", m)
	}
	// Spent: the same code does not work twice.
	u.expect(t, u.post("/auth/verify-code", map[string]any{"code": code}), http.StatusBadRequest)
}

// Five tries per code, and the budget holds under parallel guessing.
func TestVerifyCodeAttemptBudget(t *testing.T) {
	t.Parallel()
	u := signup(t)
	code := codeIn(t, testMail.waitLetter(t, u.Email, 0))
	wrong := fmt.Sprintf("%06d", (atoiOr(code)+1)%1_000_000)

	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = doReq(t, u.token, http.MethodPost, "/auth/verify-code", map[string]any{"code": wrong})
		}()
	}
	wg.Wait()
	var attempts int
	if err := testPool.QueryRow(context.Background(),
		`SELECT attempts FROM user_tokens WHERE user_id = $1 AND kind = 'verify_code'`, u.UserID).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != 5 {
		t.Fatalf("attempts = %d, want exactly the budget of 5", attempts)
	}
	m := u.expect(t, u.post("/auth/verify-code", map[string]any{"code": code}), http.StatusBadRequest)
	if m["error"] != "too many attempts" {
		t.Fatalf("the right code after the budget must be refused, got %v", m)
	}
}

func atoiOr(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// An expired code is dead, and junk input never reaches the comparison.
func TestVerifyCodeExpiresAndRejectsJunk(t *testing.T) {
	t.Parallel()
	u := signup(t)
	code := codeIn(t, testMail.waitLetter(t, u.Email, 0))
	for _, junk := range []string{"12345", "1234567", "12a456", "x" + code} {
		m := u.expect(t, u.post("/auth/verify-code", map[string]any{"code": junk}), http.StatusBadRequest)
		if m["error"] != "invalid code" {
			t.Fatalf("junk %q: %v", junk, m)
		}
	}
	if _, err := testPool.Exec(context.Background(),
		`UPDATE user_tokens SET expires_at = now() - interval '1 second' WHERE user_id = $1 AND kind = 'verify_code'`, u.UserID); err != nil {
		t.Fatal(err)
	}
	m := u.expect(t, u.post("/auth/verify-code", map[string]any{"code": code}), http.StatusBadRequest)
	if m["error"] != "code expired" {
		t.Fatalf("expired code: %v", m)
	}
	// No session, no door.
	if r := doReq(t, "", http.MethodPost, "/auth/verify-code", map[string]any{"code": code}); r.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous verify-code: %d", r.Status)
	}
}

// A resend waits out the pause, replaces the previous code, and stops at the
// hourly cap.
func TestResendCodePauseSupersedeAndCap(t *testing.T) {
	t.Parallel()
	u := signup(t)
	first := codeIn(t, testMail.waitLetter(t, u.Email, 0))

	m := u.expect(t, u.post("/auth/resend-verification", nil), http.StatusTooManyRequests)
	if m["retry_after"].(float64) <= 0 {
		t.Fatalf("429 must say how long to wait: %v", m)
	}
	ageCodes(t, u.UserID, "2 minutes")
	u.expect(t, u.post("/auth/resend-verification", nil), http.StatusOK)
	second := codeIn(t, testMail.waitLetter(t, u.Email, 1))
	if first != second {
		m = u.expect(t, u.post("/auth/verify-code", map[string]any{"code": first}), http.StatusBadRequest)
		if m["error"] != "invalid code" {
			t.Fatalf("superseded code must not work: %v", m)
		}
	}

	// Three more sends fit the hour (five in all), the sixth does not.
	for i := 2; i < 5; i++ {
		ageCodes(t, u.UserID, "2 minutes")
		u.expect(t, u.post("/auth/resend-verification", nil), http.StatusOK)
	}
	ageCodes(t, u.UserID, "2 minutes")
	m = u.expect(t, u.post("/auth/resend-verification", nil), http.StatusTooManyRequests)
	if m["retry_after"].(float64) < 60 {
		t.Fatalf("the hourly cap must wait for the oldest send to age out: %v", m)
	}
}

// The address change: password for a proven account, the code to the new
// address, the notice to the old one, and nothing the old mailbox received
// keeps working.
func TestEmailChangeMovesTheAccount(t *testing.T) {
	t.Parallel()
	u := signup(t)
	verifyByCode(t, u)
	newEmail := fmt.Sprintf("moved-%d@test.local", userSeq.Add(1))

	// A recovery link the old mailbox already holds.
	doReq(t, "", http.MethodPost, "/auth/forgot-password", map[string]any{"email": u.Email})
	resetLetter := testMail.waitLetter(t, u.Email, 1)
	resetTok := resetLetter.body[strings.Index(resetLetter.body, "token=")+len("token="):]
	resetTok = strings.Fields(resetTok)[0]
	u.expect(t, doReq(t, "", http.MethodPost, "/auth/reset-password/check", map[string]any{"token": resetTok}), http.StatusOK)

	u.expect(t, u.post("/auth/email-change", map[string]any{"email": newEmail}), http.StatusForbidden)
	u.expect(t, u.post("/auth/email-change", map[string]any{"email": newEmail, "password": "nope-nope-1"}), http.StatusForbidden)
	u.expect(t, u.post("/auth/email-change", map[string]any{"email": u.Email, "password": "password-123"}), http.StatusBadRequest)
	ageCodes(t, u.UserID, "2 minutes")
	st := u.expect(t, u.post("/auth/email-change", map[string]any{"email": strings.ToUpper(newEmail), "password": "password-123"}), http.StatusOK)
	if st["target"] != newEmail || st["change"] != true || st["email"] != u.Email {
		t.Fatalf("pending change state: %v", st)
	}
	code := codeIn(t, testMail.waitLetter(t, newEmail, 0))
	// Nothing has moved yet: the old address still signs in.
	doReqExpect(t, http.MethodPost, "/auth/login", map[string]any{"email": u.Email, "password": "password-123"}, http.StatusOK)

	m := u.expect(t, u.post("/auth/verify-code", map[string]any{"code": code}), http.StatusOK)
	got := m["user"].(map[string]any)
	if got["email"] != newEmail || got["email_verified"] != true {
		t.Fatalf("account not moved: %v", got)
	}
	notice := testMail.waitLetter(t, u.Email, 2)
	if !strings.Contains(notice.body, newEmail) {
		t.Fatalf("the old address must be told where the account went:\n%s", notice.body)
	}
	doReqExpect(t, http.MethodPost, "/auth/login", map[string]any{"email": newEmail, "password": "password-123"}, http.StatusOK)
	doReqExpect(t, http.MethodPost, "/auth/login", map[string]any{"email": u.Email, "password": "password-123"}, http.StatusUnauthorized)
	// The recovery link sent to the old mailbox died with the move.
	doReqExpect(t, http.MethodPost, "/auth/reset-password/check", map[string]any{"token": resetTok}, http.StatusBadRequest)
}

// An address that belongs to someone else answers exactly like a free one; its
// owner hears about the attempt, and no code can ever take it.
func TestEmailChangeToTakenAddressLeaksNothing(t *testing.T) {
	t.Parallel()
	u := signup(t)
	other := signup(t)
	testMail.waitLetter(t, other.Email, 0) // their own sign-up letter first
	before := 1

	// Unverified account: the typo-fixing case needs no password.
	ageCodes(t, u.UserID, "2 minutes")
	st := u.expect(t, u.post("/auth/email-change", map[string]any{"email": other.Email}), http.StatusOK)
	if st["target"] != other.Email || st["change"] != true {
		t.Fatalf("a taken address must look like a free one: %v", st)
	}
	warning := testMail.waitLetter(t, other.Email, before)
	if sixDigits.MatchString(warning.body) {
		t.Fatalf("the owner's warning must not carry a code:\n%s", warning.body)
	}
	for _, guess := range []string{"000000", "123456", "999999"} {
		m := u.expect(t, u.post("/auth/verify-code", map[string]any{"code": guess}), http.StatusBadRequest)
		if m["error"] != "invalid code" {
			t.Fatalf("guess %s: %v", guess, m)
		}
	}
	me := u.expect(t, u.get("/auth/me"), http.StatusOK)["user"].(map[string]any)
	if me["email"] != u.Email {
		t.Fatalf("account moved onto a taken address: %v", me)
	}
	// Cancelling leaves the account where it was, with no live change.
	st = u.expect(t, u.del("/auth/email-change"), http.StatusOK)
	if st["change"] != false {
		t.Fatalf("cancel: %v", st)
	}
}

// A fresh sign-up fixing a typo: the old verification link must not prove the
// new address, and the code that does lands the account verified.
func TestEmailChangeKillsTheOldVerificationLink(t *testing.T) {
	t.Parallel()
	u := signup(t)
	link := testMail.waitLetter(t, u.Email, 0).body
	tok := strings.Fields(link[strings.Index(link, "token=")+len("token="):])[0]
	newEmail := fmt.Sprintf("typo-fixed-%d@test.local", userSeq.Add(1))
	// The pause is per address: fixing a typo right after sign-up goes out at
	// once, while asking for the same new address again has to wait.
	u.expect(t, u.post("/auth/email-change", map[string]any{"email": newEmail}), http.StatusOK)
	u.expect(t, u.post("/auth/email-change", map[string]any{"email": newEmail}), http.StatusTooManyRequests)
	u.expect(t, u.post("/auth/resend-verification", nil), http.StatusTooManyRequests)
	code := codeIn(t, testMail.waitLetter(t, newEmail, 0))
	u.expect(t, u.post("/auth/verify-code", map[string]any{"code": code}), http.StatusOK)
	doReqExpect(t, http.MethodPost, "/auth/verify-email", map[string]any{"token": tok}, http.StatusBadRequest)
	// The old, never-proven address is not written to about the move.
	testMail.noLetter(t, u.Email, 1)
}

// The recovery link's deadline, read without spending it (#3060).
func TestResetLinkCheck(t *testing.T) {
	t.Parallel()
	u := signup(t)
	doReq(t, "", http.MethodPost, "/auth/forgot-password", map[string]any{"email": u.Email})
	body := testMail.waitLetter(t, u.Email, 1).body
	tok := strings.Fields(body[strings.Index(body, "token=")+len("token="):])[0]

	m := doReqExpect(t, http.MethodPost, "/auth/reset-password/check", map[string]any{"token": tok}, http.StatusOK)
	if _, ok := m["expires_at"].(string); !ok {
		t.Fatalf("check must return the deadline: %v", m)
	}
	// Checking twice spends nothing.
	doReqExpect(t, http.MethodPost, "/auth/reset-password/check", map[string]any{"token": tok}, http.StatusOK)
	doReqExpect(t, http.MethodPost, "/auth/reset-password", map[string]any{"token": tok, "new_password": "password-456"}, http.StatusNoContent)
	m = doReqExpect(t, http.MethodPost, "/auth/reset-password/check", map[string]any{"token": tok}, http.StatusBadRequest)
	if m["error"] != "invalid or expired token" {
		t.Fatalf("spent link: %v", m)
	}
	doReqExpect(t, http.MethodPost, "/auth/reset-password/check", map[string]any{"token": "nonsense"}, http.StatusBadRequest)
}

// doReqExpect is doReq + a status check, for anonymous calls.
func doReqExpect(t *testing.T, method, path string, body any, want int) map[string]any {
	t.Helper()
	c := &client{t: t}
	return c.expect(t, doReq(t, "", method, path, body), want)
}
