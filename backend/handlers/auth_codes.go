package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/middleware"
)

// One-time codes (#3035): the six digits a person types into the «Проверьте
// почту» cells, either to prove the address the account already has
// (kind verify_code, #3057) or to prove a new one before it replaces the old
// (kind email_change, #3058).
//
// A code is short by design, so everything around it is what keeps it safe:
//
//   - it is stored as an HMAC under the server secret with a per-code salt, never
//     in the clear — a leaked user_tokens table yields no usable codes, and the
//     salt keeps two identical codes from colliding on the unique token_hash;
//   - it lives codeTTL, and gets codeMaxAttempts tries, counted atomically
//     *before* the comparison so parallel guesses cannot exceed the budget;
//   - only the newest code is live: issuing one supersedes the previous;
//   - issuing is throttled per account — a pause between sends and an hourly
//     cap — so attempts × resends stays a few dozen guesses an hour against a
//     million values;
//   - every code door sits behind a session: the code proves a mailbox *to the
//     account that asked for it*, and is worthless to anyone else.
//
// The email-change door must not tell its caller whether the new address is
// already registered. When it is, a code row is still created (with a hash no
// input can match) and the address's owner gets a warning instead of a code —
// the requester sees exactly what they would see for a free address.
const (
	codeTTL         = 15 * time.Minute
	codeMaxAttempts = 5
	codeResendPause = 60 * time.Second
	codeHourlyCap   = 5
	codeDigits      = 6

	kindVerifyCode  = "verify_code"
	kindEmailChange = "email_change"
)

var codeKinds = []string{kindVerifyCode, kindEmailChange}

// newOneTimeCode draws a uniformly random six-digit code.
func newOneTimeCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}

// codeHash seals a code for storage: "c1:<salt>:<hmac>". The account and the
// kind are inside the MAC, so a stored hash cannot be replayed against another
// account or used as the other kind of code.
func (h *AuthHandler) codeHash(userID uuid.UUID, kind, code string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	s := hex.EncodeToString(salt)
	return "c1:" + s + ":" + h.codeMAC(s, userID, kind, code), nil
}

func (h *AuthHandler) codeMAC(salt string, userID uuid.UUID, kind, code string) string {
	m := hmac.New(sha256.New, []byte(h.secret))
	m.Write([]byte(salt + "|" + userID.String() + "|" + kind + "|" + code))
	return hex.EncodeToString(m.Sum(nil))
}

// codeMatches compares a typed code against a stored hash in constant time.
func (h *AuthHandler) codeMatches(stored string, userID uuid.UUID, kind, code string) bool {
	parts := strings.SplitN(stored, ":", 3)
	if len(parts) != 3 || parts[0] != "c1" {
		return false
	}
	want := h.codeMAC(parts[1], userID, kind, code)
	return hmac.Equal([]byte(want), []byte(parts[2]))
}

// normalizeCode accepts a code the way people paste it — with spaces or a dash
// in the middle — and reports whether what is left is six digits.
func normalizeCode(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == ' ':
		default:
			return "", false
		}
	}
	s := b.String()
	return s, len(s) == codeDigits
}

// codeWait is how long the account must wait before another code may be sent:
// zero when it may go now.
func (h *AuthHandler) codeWait(ctx context.Context, userID uuid.UUID, now time.Time) (time.Duration, error) {
	r, err := h.q.RecentUserCodes(ctx, db.RecentUserCodesParams{UserID: userID, Since: now.Add(-time.Hour)})
	if err != nil {
		return 0, err
	}
	var wait time.Duration
	if r.N > 0 {
		if w := r.LastSent.Add(codeResendPause).Sub(now); w > wait {
			wait = w
		}
	}
	if r.N >= codeHourlyCap {
		if w := r.FirstSent.Add(time.Hour).Sub(now); w > wait {
			wait = w
		}
	}
	return wait, nil
}

// issueCode supersedes every live code of the account and stores a new one.
// matchable=false stores a hash no typed input can match — the email-change
// answer for an address that is taken (see the top of this file). Returns the
// raw code ("" when not matchable).
func (h *AuthHandler) issueCode(ctx context.Context, userID uuid.UUID, kind string, target *string, matchable bool) (string, error) {
	if err := h.q.SupersedeUserCodes(ctx, db.SupersedeUserCodesParams{UserID: userID, Kinds: codeKinds}); err != nil {
		return "", err
	}
	code, err := newOneTimeCode()
	if err != nil {
		return "", err
	}
	hashed := code
	if !matchable {
		// Not digits, so normalizeCode never lets a typed value reach it.
		hashed = "x" + uuid.NewString()
	}
	hash, err := h.codeHash(userID, kind, hashed)
	if err != nil {
		return "", err
	}
	if _, err := h.q.CreateUserCode(ctx, db.CreateUserCodeParams{
		UserID: userID, Kind: kind, TokenHash: hash, ExpiresAt: time.Now().Add(codeTTL), Email: target,
	}); err != nil {
		return "", err
	}
	if !matchable {
		return "", nil
	}
	return code, nil
}

func (h *AuthHandler) appURL(path string) string {
	return strings.TrimRight(h.publicURL, "/") + path
}

// beginEmailChange issues the code for moving the account to newEmail and sends
// the letter that belongs to it: the code to a free address, a warning to the
// owner of a taken one. Both leave the same trace for the requester.
func (h *AuthHandler) beginEmailChange(c *gin.Context, user db.User, newEmail string) error {
	_, err := h.q.GetUserByEmail(c, newEmail)
	switch {
	case err == nil:
		if _, err := h.issueCode(c, user.ID, kindEmailChange, &newEmail, false); err != nil {
			return err
		}
		subject, body := mail.Compose(mail.KindEmailInUse, emailLang(c, h.q, newEmail), mail.Vars{Link: h.appURL("/login")})
		mail.SendAsync(h.mailer, newEmail, subject, body)
		return nil
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	code, err := h.issueCode(c, user.ID, kindEmailChange, &newEmail, true)
	if err != nil {
		return err
	}
	subject, body := mail.Compose(mail.KindEmailChange, h.userLang(c, user.ID), mail.Vars{
		Link: h.appURL("/verify-email"), Code: code, CodeMinutes: int(codeTTL / time.Minute),
	})
	mail.SendAsync(h.mailer, newEmail, subject, body)
	return nil
}

// verificationState is what the «Проверьте почту» screen draws: where the live
// code went, how long it lives, how many tries are left and when the next send
// is allowed. The same shape answers every code door, so the screen never has to
// make a second request to redraw.
func (h *AuthHandler) verificationState(ctx context.Context, user db.User) (gin.H, error) {
	now := time.Now()
	out := gin.H{
		"email":         user.Email,
		"verified":      user.EmailVerified,
		"target":        user.Email,
		"change":        false,
		"expires_at":    nil,
		"attempts_left": 0,
		"resend_in":     0,
		"mail":          h.mailer != nil && h.mailer.Enabled(),
		"has_password":  user.PasswordHash != "",
	}
	live, err := h.q.GetLiveUserCode(ctx, db.GetLiveUserCodeParams{UserID: user.ID, Kinds: codeKinds})
	switch {
	case err == nil:
		if live.Kind == kindEmailChange && live.Email != nil {
			out["target"] = *live.Email
			out["change"] = true
		}
		out["expires_at"] = live.ExpiresAt
		out["attempts_left"] = max(0, codeMaxAttempts-int(live.Attempts))
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	wait, err := h.codeWait(ctx, user.ID, now)
	if err != nil {
		return nil, err
	}
	out["resend_in"] = int((wait + time.Second - 1) / time.Second)
	return out, nil
}

// answerVerification writes the state, or a 500 when it cannot be read.
func (h *AuthHandler) answerVerification(c *gin.Context, status int, user db.User) {
	st, err := h.verificationState(c, user)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(status, st)
}

// tooSoon answers 429 with the seconds left, when another code may not go yet.
func (h *AuthHandler) tooSoon(c *gin.Context, userID uuid.UUID) bool {
	wait, err := h.codeWait(c, userID, time.Now())
	if err != nil {
		fail(c, err)
		return true
	}
	if wait <= 0 {
		return false
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": "too many requests", "retry_after": int((wait + time.Second - 1) / time.Second),
	})
	return true
}

// Verification reports the code state of the current account (GET /auth/verification).
func (h *AuthHandler) Verification(c *gin.Context) {
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	h.answerVerification(c, http.StatusOK, user)
}

// ResendVerification sends a fresh code (and, for the account's own address, a
// fresh link) to wherever the live code goes: the pending new address while a
// change is under way, the account's address otherwise. Throttled — see
// codeWait.
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	live, lerr := h.q.GetLiveUserCode(c, db.GetLiveUserCodeParams{UserID: user.ID, Kinds: codeKinds})
	changing := lerr == nil && live.Kind == kindEmailChange && live.Email != nil
	if !changing && user.EmailVerified {
		// Nothing to prove — the same 204 no-op this route always gave.
		c.Status(http.StatusNoContent)
		return
	}
	if h.tooSoon(c, user.ID) {
		return
	}
	if changing {
		if err := h.beginEmailChange(c, user, *live.Email); err != nil {
			fail(c, err)
			return
		}
	} else {
		soft(c, "DeleteUserTokensOfKind", h.q.DeleteUserTokensOfKind(c, db.DeleteUserTokensOfKindParams{UserID: user.ID, Kind: "verify"}))
		h.sendVerification(c, user)
	}
	h.answerVerification(c, http.StatusOK, user)
}

// StartEmailChange begins moving the account to a new address (#3058). Nothing
// changes until the code sent there is typed back; the old address is told
// once it has.
//
// The current password is asked for when the account has one and its address
// is proven: that is the account worth stealing, and a session alone (a laptop
// left open) must not be enough to redirect its mail and then its password
// recovery. A fresh, still-unverified sign-up correcting a typo has nothing to
// protect yet and is not asked; an account with no password at all (GitLab
// sign-in) has nothing to ask for.
func (h *AuthHandler) StartEmailChange(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	newEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if newEmail == strings.ToLower(user.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this is already your email"})
		return
	}
	if user.PasswordHash != "" && user.EmailVerified && !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusForbidden, gin.H{"error": "current password is incorrect"})
		return
	}
	if h.tooSoon(c, user.ID) {
		return
	}
	if err := h.beginEmailChange(c, user, newEmail); err != nil {
		fail(c, err)
		return
	}
	h.answerVerification(c, http.StatusOK, user)
}

// CancelEmailChange drops a pending change; the account keeps its address.
func (h *AuthHandler) CancelEmailChange(c *gin.Context) {
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if err := h.q.SupersedeUserCodes(c, db.SupersedeUserCodesParams{UserID: user.ID, Kinds: []string{kindEmailChange}}); err != nil {
		fail(c, err)
		return
	}
	h.answerVerification(c, http.StatusOK, user)
}

// VerifyCode checks a typed code against the account's live one (#3057/#3058).
// A match on a verify_code proves the account's address; a match on an
// email_change moves the account to the address it was sent to.
func (h *AuthHandler) VerifyCode(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	code, ok := normalizeCode(req.Code)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}
	live, err := h.q.GetLiveUserCode(c, db.GetLiveUserCodeParams{UserID: uid, Kinds: codeKinds})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			fail(c, err)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "code expired"})
		return
	}
	attempts, err := h.q.SpendUserCodeAttempt(c, db.SpendUserCodeAttemptParams{ID: live.ID, MaxAttempts: codeMaxAttempts})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			fail(c, err)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many attempts", "attempts_left": 0})
		return
	}
	if !h.codeMatches(live.TokenHash, uid, live.Kind, code) {
		left := max(0, codeMaxAttempts-int(attempts))
		msg := "invalid code"
		if left == 0 {
			msg = "too many attempts"
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg, "attempts_left": left})
		return
	}
	soft(c, "MarkUserTokenUsed", h.q.MarkUserTokenUsed(c, live.ID))
	user, err := h.q.GetUserByID(c, uid)
	if err != nil {
		fail(c, err)
		return
	}

	if live.Kind == kindEmailChange && live.Email != nil {
		user, ok = h.applyEmailChange(c, user, *live.Email)
		if !ok {
			return
		}
	} else if !user.EmailVerified {
		if err := h.q.MarkEmailVerified(c, uid); err != nil {
			fail(c, err)
			return
		}
		user.EmailVerified = true
		// The link sent alongside the code has nothing left to do.
		soft(c, "DeleteUserTokensOfKind", h.q.DeleteUserTokensOfKind(c, db.DeleteUserTokensOfKindParams{UserID: uid, Kind: "verify"}))
	}
	// The mailbox is proven now — whatever is still addressed to it applies.
	h.acceptPendingInvitations(c, user)
	c.JSON(http.StatusOK, gin.H{"user": buildUserDTO(c, h.q, user)})
}

// applyEmailChange moves the account to a proven address. Every token that went
// to the old mailbox dies with it — a verification link would otherwise mark the
// *new* address proven, and a recovery link would hand the account to whoever
// still reads the old one. The old address is told, when it was ever proven to
// be the account's: an unverified one may be the very typo being corrected, and
// a stranger has no business hearing about this account.
func (h *AuthHandler) applyEmailChange(c *gin.Context, user db.User, newEmail string) (db.User, bool) {
	oldEmail, oldVerified := user.Email, user.EmailVerified
	updated, err := h.q.ChangeUserEmail(c, db.ChangeUserEmailParams{ID: user.ID, Email: newEmail})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Taken between the request and the code — said the way every other
			// dead code is, so this door does not become the probe either.
			c.JSON(http.StatusBadRequest, gin.H{"error": "code expired"})
			return db.User{}, false
		}
		fail(c, err)
		return db.User{}, false
	}
	soft(c, "DeleteUserTokensOfKinds", h.q.DeleteUserTokensOfKinds(c, db.DeleteUserTokensOfKindsParams{
		UserID: user.ID, Kinds: []string{"verify", "reset", kindVerifyCode, kindEmailChange},
	}))
	if oldVerified {
		subject, body := mail.Compose(mail.KindEmailChanged, h.userLang(c, user.ID), mail.Vars{
			Email: newEmail, Link: h.appURL("/forgot-password"),
		})
		mail.SendAsync(h.mailer, oldEmail, subject, body)
	}
	return updated, true
}
