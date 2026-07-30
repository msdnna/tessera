// Account lifecycle (verify-email, forgot/reset password), personal access
// tokens, self-service profile and the global admin panel.
//
// Verify/reset tokens are stored as SHA-256 hashes and the raw value is only
// ever emailed (no-op mailer in tests), so the full flows are exercised by
// forging a token row directly in the DB with internal/auth — same generator
// the handlers use. The admin reset-link endpoint returns the raw link, so
// that flow needs no forging.
package main

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"tessera/internal/auth"
)

// insertUserToken forges a verify/reset token for a user directly in the DB
// (only the hash is stored by the API; the raw token goes out by email).
func insertUserToken(t *testing.T, userID, kind string) string {
	t.Helper()
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO user_tokens (user_id, kind, token_hash, expires_at)
		 VALUES ($1, $2, $3, now() + interval '1 hour')`, userID, kind, hash); err != nil {
		t.Fatalf("insert user token: %v", err)
	}
	return raw
}

func login(t *testing.T, email, password string) resp {
	t.Helper()
	return doReq(t, "", http.MethodPost, "/auth/login", map[string]any{"email": email, "password": password})
}

// meUser fetches /auth/me and returns the nested user object.
func meUser(t *testing.T, c *client) map[string]any {
	t.Helper()
	m := c.expect(t, c.get("/auth/me"), http.StatusOK)
	u, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatalf("me: no user object\n%v", m)
	}
	return u
}

// putMultipart is uploadFile for PUT routes (avatar upload).
func putMultipart(t *testing.T, c *client, path, field, filename string, content []byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPut, testServer.URL+"/api"+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data}
}

// tinyPNG carries a real PNG signature (http.DetectContentType → image/png).
var tinyPNG = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)

// ── verify-email ─────────────────────────────────────────────────────────────

func TestVerifyEmailFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)

	if v := meUser(t, c)["email_verified"]; v != false {
		t.Fatalf("fresh user email_verified = %v, want false", v)
	}

	// Garbage token → 400.
	r := doReq(t, "", http.MethodPost, "/auth/verify-email", map[string]any{"token": "nope"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("bad token: status %d, want 400", r.Status)
	}

	raw := insertUserToken(t, c.UserID, "verify")
	r = doReq(t, "", http.MethodPost, "/auth/verify-email", map[string]any{"token": raw})
	if r.Status != http.StatusNoContent {
		t.Fatalf("verify: status %d, want 204\n%s", r.Status, r.Body)
	}
	if v := meUser(t, c)["email_verified"]; v != true {
		t.Fatalf("email_verified = %v after verify, want true", v)
	}

	// Token is single-use.
	r = doReq(t, "", http.MethodPost, "/auth/verify-email", map[string]any{"token": raw})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("replayed token: status %d, want 400", r.Status)
	}

	// Resend for an already-verified user is still a 204 no-op.
	c.expect(t, c.post("/auth/resend-verification", nil), http.StatusNoContent)
}

// ── forgot / reset password ──────────────────────────────────────────────────

func TestForgotResetPasswordFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)

	// Always 200, even for unknown emails (no account enumeration).
	r := doReq(t, "", http.MethodPost, "/auth/forgot-password", map[string]any{"email": c.Email})
	if r.Status != http.StatusOK {
		t.Fatalf("forgot: status %d, want 200", r.Status)
	}
	r = doReq(t, "", http.MethodPost, "/auth/forgot-password", map[string]any{"email": "nobody@test.local"})
	if r.Status != http.StatusOK {
		t.Fatalf("forgot unknown email: status %d, want 200", r.Status)
	}

	// Garbage token / short password → 400.
	r = doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{"token": "nope", "new_password": "long-enough-pw"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("bad reset token: status %d, want 400", r.Status)
	}
	raw := insertUserToken(t, c.UserID, "reset")
	r = doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{"token": raw, "new_password": "short"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("short password: status %d, want 400", r.Status)
	}

	r = doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{"token": raw, "new_password": "reset-password-456"})
	if r.Status != http.StatusNoContent {
		t.Fatalf("reset: status %d, want 204\n%s", r.Status, r.Body)
	}

	if r := login(t, c.Email, "password-123"); r.Status != http.StatusUnauthorized {
		t.Fatalf("old password after reset: status %d, want 401", r.Status)
	}
	if r := login(t, c.Email, "reset-password-456"); r.Status != http.StatusOK {
		t.Fatalf("new password: status %d, want 200\n%s", r.Status, r.Body)
	}

	// Reset revokes refresh tokens (other sessions are logged out).
	r = doReq(t, "", http.MethodPost, "/auth/refresh", map[string]any{"refresh_token": c.Refresh})
	if r.Status != http.StatusUnauthorized {
		t.Fatalf("old refresh token after reset: status %d, want 401", r.Status)
	}

	// Token is single-use.
	r = doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{"token": raw, "new_password": "reset-password-789"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("replayed reset token: status %d, want 400", r.Status)
	}
}

// ── personal access tokens ───────────────────────────────────────────────────

func TestPATLifecycle(t *testing.T) {
	t.Parallel()
	c := signup(t)

	m := c.expect(t, c.post("/auth/tokens", map[string]any{"name": "it-token"}), http.StatusCreated)
	token, _ := m["token"].(string)
	if !strings.HasPrefix(token, "tsra_") {
		t.Fatalf("PAT plaintext = %q, want tsra_ prefix", token)
	}
	pat := m["pat"].(map[string]any)
	if lf := pat["last_four"].(string); lf != token[len(token)-4:] {
		t.Fatalf("last_four = %q, want %q", lf, token[len(token)-4:])
	}

	// The PAT works as a bearer credential on protected routes.
	patClient := &client{t: t, token: token}
	u := meUser(t, patClient)
	if u["id"] != c.UserID {
		t.Fatalf("PAT resolved to user %v, want %s", u["id"], c.UserID)
	}
	patClient.expect(t, patClient.get("/workspaces"), http.StatusOK)

	// Listing never returns the plaintext.
	list := c.get("/auth/tokens").listBody(t)
	if len(list) != 1 || list[0]["id"] != pat["id"] {
		t.Fatalf("list PATs: %v", list)
	}
	if _, ok := list[0]["token"]; ok {
		t.Fatalf("list leaks plaintext token: %v", list[0])
	}

	// Revoke → the PAT stops working; revoking again is an idempotent 204.
	c.expect(t, c.del("/auth/tokens/"+pat["id"].(string)), http.StatusNoContent)
	if r := patClient.get("/auth/me"); r.Status != http.StatusUnauthorized {
		t.Fatalf("revoked PAT: status %d, want 401", r.Status)
	}
	c.expect(t, c.del("/auth/tokens/"+pat["id"].(string)), http.StatusNoContent)
	if r := c.del("/auth/tokens/not-a-uuid"); r.Status != http.StatusBadRequest {
		t.Fatalf("revoke bad uuid: status %d, want 400", r.Status)
	}
}

// ── self-service profile / password / preferences / avatar ───────────────────

func TestMyProfileAndPreferences(t *testing.T) {
	t.Parallel()
	c := signup(t)

	m := c.expect(t, c.patch("/users/me", map[string]any{
		"name": "Новое имя", "bio": "коротко о себе", "company": "msdnna",
	}), http.StatusOK)
	if m["name"] != "Новое имя" || m["company"] != "msdnna" {
		t.Fatalf("profile update: %v", m)
	}
	// name is required (full-replace endpoint).
	if r := c.patch("/users/me", map[string]any{"bio": "x"}); r.Status != http.StatusBadRequest {
		t.Fatalf("profile without name: status %d, want 400", r.Status)
	}

	p := c.expect(t, c.put("/users/me/preferences", map[string]any{
		"language": "ru", "time_format": "12h", "date_format": "dd.MM.yyyy",
		"week_start": 0, "theme": "dark", "accent": "teal",
	}), http.StatusOK)
	if p["theme"] != "dark" || p["time_format"] != "12h" {
		t.Fatalf("preferences: %v", p)
	}
	if r := c.put("/users/me/preferences", map[string]any{
		"theme": "neon", "time_format": "24h",
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("invalid theme: status %d, want 400", r.Status)
	}
}

func TestChangeMyPassword(t *testing.T) {
	t.Parallel()
	c := signup(t)

	// Wrong current password → 403 (not 401 — the session itself is valid).
	r := c.put("/users/me/password", map[string]any{
		"current_password": "wrong", "new_password": "changed-password-1",
	})
	if r.Status != http.StatusForbidden {
		t.Fatalf("wrong current password: status %d, want 403", r.Status)
	}

	c.expect(t, c.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": "changed-password-1",
	}), http.StatusNoContent)

	if r := login(t, c.Email, "password-123"); r.Status != http.StatusUnauthorized {
		t.Fatalf("old password: status %d, want 401", r.Status)
	}
	if r := login(t, c.Email, "changed-password-1"); r.Status != http.StatusOK {
		t.Fatalf("new password: status %d, want 200\n%s", r.Status, r.Body)
	}
}

func TestMyAvatar(t *testing.T) {
	t.Parallel()
	c := signup(t)

	// Non-image content is rejected.
	if r := putMultipart(t, c, "/users/me/avatar", "avatar", "a.txt", []byte("not an image")); r.Status != http.StatusUnsupportedMediaType {
		t.Fatalf("text avatar: status %d, want 415", r.Status)
	}

	r := putMultipart(t, c, "/users/me/avatar", "avatar", "a.png", tinyPNG)
	m := c.expect(t, r, http.StatusOK)
	wantURL := "/api/users/" + c.UserID + "/avatar"
	if m["avatar_url"] != wantURL {
		t.Fatalf("avatar_url = %v, want %s", m["avatar_url"], wantURL)
	}

	// Served publicly (no token) with the stored content type.
	if r := doReq(t, "", http.MethodGet, "/users/"+c.UserID+"/avatar", nil); r.Status != http.StatusOK || !bytes.Equal(r.Body, tinyPNG) {
		t.Fatalf("get avatar: status %d, %d bytes", r.Status, len(r.Body))
	}

	c.expect(t, c.del("/users/me/avatar"), http.StatusNoContent)
	if r := doReq(t, "", http.MethodGet, "/users/"+c.UserID+"/avatar", nil); r.Status != http.StatusNotFound {
		t.Fatalf("avatar after delete: status %d, want 404", r.Status)
	}
}

// ── global admin panel ───────────────────────────────────────────────────────

func TestAdminPanel(t *testing.T) {
	t.Parallel()
	adm := signup(t)
	victim := signup(t)

	// The very first user registered after TRUNCATE becomes admin, and with
	// parallel tests that can be anyone — demote explicitly before asserting.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = false WHERE id = $1`, adm.UserID); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if r := adm.get("/admin/users"); r.Status != http.StatusForbidden {
		t.Fatalf("non-admin list users: status %d, want 403", r.Status)
	}

	// Promote directly in the DB — the API path for the very first admin.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = true WHERE id = $1`, adm.UserID); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	users := adm.get("/admin/users").listBody(t)
	var seen map[string]any
	for _, u := range users {
		if u["email"] == victim.Email {
			seen = u
		}
	}
	if seen == nil {
		t.Fatalf("admin list misses %s (%d users)", victim.Email, len(users))
	}
	if seen["active"] != true || seen["is_admin"] != false {
		t.Fatalf("victim flags: %v", seen)
	}

	// Deactivate → login blocked with 403; reactivate → login works again.
	adm.expect(t, adm.patch("/admin/users/"+victim.UserID+"/active", map[string]any{"active": false}), http.StatusNoContent)
	if r := login(t, victim.Email, "password-123"); r.Status != http.StatusForbidden {
		t.Fatalf("deactivated login: status %d, want 403", r.Status)
	}
	adm.expect(t, adm.patch("/admin/users/"+victim.UserID+"/active", map[string]any{"active": true}), http.StatusNoContent)
	if r := login(t, victim.Email, "password-123"); r.Status != http.StatusOK {
		t.Fatalf("reactivated login: status %d, want 200", r.Status)
	}

	// Grant + revoke the admin flag; self-change is refused.
	adm.expect(t, adm.patch("/admin/users/"+victim.UserID+"/admin", map[string]any{"admin": true}), http.StatusNoContent)
	victim.expect(t, victim.get("/admin/users"), http.StatusOK)
	adm.expect(t, adm.patch("/admin/users/"+victim.UserID+"/admin", map[string]any{"admin": false}), http.StatusNoContent)
	if r := adm.patch("/admin/users/"+adm.UserID+"/admin", map[string]any{"admin": false}); r.Status != http.StatusBadRequest {
		t.Fatalf("self admin change: status %d, want 400", r.Status)
	}
	if r := adm.patch("/admin/users/"+adm.UserID+"/active", map[string]any{"active": false}); r.Status != http.StatusBadRequest {
		t.Fatalf("self active change: status %d, want 400", r.Status)
	}

	// Reset link returns the raw token → full reset flow without email.
	m := adm.expect(t, adm.post("/admin/users/"+victim.UserID+"/reset-link", nil), http.StatusOK)
	link, _ := m["link"].(string)
	_, token, found := strings.Cut(link, "token=")
	if !found || token == "" {
		t.Fatalf("reset link: %q", link)
	}
	r := doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{"token": token, "new_password": "admin-reset-pw-1"})
	if r.Status != http.StatusNoContent {
		t.Fatalf("reset via admin link: status %d, want 204\n%s", r.Status, r.Body)
	}
	if r := login(t, victim.Email, "admin-reset-pw-1"); r.Status != http.StatusOK {
		t.Fatalf("login after admin reset: status %d, want 200", r.Status)
	}

	// Unknown user / bad uuid.
	if r := adm.post("/admin/users/"+dummyID+"/reset-link", nil); r.Status != http.StatusNotFound {
		t.Fatalf("reset-link unknown user: status %d, want 404", r.Status)
	}
	if r := adm.post("/admin/users/not-a-uuid/reset-link", nil); r.Status != http.StatusBadRequest {
		t.Fatalf("reset-link bad uuid: status %d, want 400", r.Status)
	}

	// The demoted victim is locked out of the panel again.
	if r := victim.get("/admin/users"); r.Status != http.StatusForbidden {
		t.Fatalf("demoted admin list: status %d, want 403", r.Status)
	}
}
