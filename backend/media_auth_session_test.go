// The image credential has to end when the session that minted it does (#2932).
//
// The media cookie is a 30-day JWT carrying nothing but a subject, an audience
// and an expiry, and it is handed out beside the refresh cookie on every login
// and every rotation — "the cookie is meant to last exactly as long as the
// session that minted it" (internal/auth/token.go). Nothing enforced that: the
// only thing ever looked up about it was whether the account is still switched
// on (media_auth_deactivation_test.go), so the two gestures that end a session —
// changing the password and logging out — left a copy of the cookie reading
// inline images for the rest of the month.
//
// Changing the password is the door a user reaches for on noticing someone else
// is in the account; ChangeMyPassword spells out what it revokes (the refresh
// chain, the sockets) and what it deliberately spares (personal access tokens).
// The media cookie was in neither list.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// cookieSession is a browser-shaped session: the credentials a cookie-mode
// client walks away from /auth/register with, plus what it takes to change that
// account's password afterwards.
type cookieSession struct {
	media    *http.Cookie
	refresh  *http.Cookie
	token    string
	email    string
	password string
	userID   string
}

// registerCookieSession signs up a fresh user in cookie mode, like the SPA does,
// and keeps hold of the password so the test can change it.
func registerCookieSession(t *testing.T) cookieSession {
	t.Helper()
	n := userSeq.Add(1)
	s := cookieSession{
		email:    fmt.Sprintf("media-session-%d@test.local", n),
		password: "password-123",
	}
	r := authReq(t, http.MethodPost, "/auth/register", map[string]any{
		"email": s.email, "name": "Media Session", "password": s.password,
	}, cookieMode)
	if r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("register: status %d\n%s", r.Status, r.Body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("register decode: %v\n%s", err, r.Body)
	}
	s.token, s.userID = out.AccessToken, out.User.ID
	if s.media = mediaCookie(r); s.media == nil {
		t.Fatalf("cookie-mode register set no media cookie\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	if s.refresh = refreshCookie(r); s.refresh == nil {
		t.Fatalf("cookie-mode register set no refresh cookie\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	return s
}

// someMediaURL uploads an image through an unrelated account and returns its
// URL: what is on trial is the caller's credential, not their claim to this
// particular file.
func someMediaURL(t *testing.T) string {
	t.Helper()
	owner := signup(t)
	m := owner.expect(t, uploadFile(t, owner, "/uploads", "file", "снимок.png", []byte(pngBody)), http.StatusCreated)
	url, _ := m["url"].(string)
	if url == "" {
		t.Fatalf("upload returned no url: %v", m)
	}
	return url
}

// TestMediaCookieDiesWithChangedPassword is the intruder case: a copy of the
// cookie taken during the break-in must stop working the moment the password
// behind it is replaced, exactly as the intruder's refresh token and open socket
// already do.
func TestMediaCookieDiesWithChangedPassword(t *testing.T) {
	t.Parallel()
	srv := mediaAuthServer(t)
	url := someMediaURL(t)

	victim := registerCookieSession(t)
	stolen := victim.media // the copy that walked out of the session

	if got := mediaGet(t, srv, url, nil, stolen); got != http.StatusOK {
		t.Fatalf("media cookie of a live session: status %d, want 200", got)
	}

	// Cookie mode, because that is the client this credential exists for: the SPA
	// changes its password on the same session the media cookie rides on.
	r := authReq(t, http.MethodPut, "/users/me/password", map[string]any{
		"current_password": victim.password, "new_password": "changed-password-1",
	}, map[string]string{"Authorization": "Bearer " + victim.token, "X-Auth-Mode": "cookie"})
	if r.Status != http.StatusOK {
		t.Fatalf("change password: status %d\n%s", r.Status, r.Body)
	}

	if got := mediaGet(t, srv, url, nil, stolen); got != http.StatusUnauthorized {
		t.Fatalf("media cookie after the password behind it was changed: status %d, want 401 — "+
			"the refresh chain and the sockets go down with the old password, and this credential outlives both by 30 days", got)
	}

	// And the device that performed the change keeps its pictures: the response
	// hands back a session, and the image credential has to come with it.
	fresh := mediaCookie(r)
	if fresh == nil {
		t.Fatalf("password change returned no replacement media cookie\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	if got := mediaGet(t, srv, url, nil, fresh); got != http.StatusOK {
		t.Fatalf("media cookie handed back by the password change: status %d, want 200", got)
	}
}

// TestMediaCookieDiesWithLogout is the same credential read from the ordinary
// end of a session. Logout expires the cookie in the browser that asked for it,
// which says nothing about a copy taken elsewhere — the server has to refuse it.
func TestMediaCookieDiesWithLogout(t *testing.T) {
	t.Parallel()
	srv := mediaAuthServer(t)
	url := someMediaURL(t)

	s := registerCookieSession(t)
	if got := mediaGet(t, srv, url, nil, s.media); got != http.StatusOK {
		t.Fatalf("media cookie of a live session: status %d, want 200", got)
	}

	if r := authReq(t, http.MethodPost, "/auth/logout", nil, cookieMode, s.refresh); r.Status != http.StatusNoContent {
		t.Fatalf("logout: status %d\n%s", r.Status, r.Body)
	}
	if got := mediaGet(t, srv, url, nil, s.media); got != http.StatusUnauthorized {
		t.Fatalf("media cookie after logout: status %d, want 401", got)
	}
}

// TestMediaCookieSurvivesRotation is the counterweight: the session binding must
// not cost an ordinary long-lived browser its images. /auth/refresh rotates the
// refresh token and reissues the media cookie in the same response, so the one
// the browser ends up holding is always the live one.
func TestMediaCookieSurvivesRotation(t *testing.T) {
	t.Parallel()
	srv := mediaAuthServer(t)
	url := someMediaURL(t)

	s := registerCookieSession(t)
	r := authReq(t, http.MethodPost, "/auth/refresh", nil, cookieMode, s.refresh)
	if r.Status != http.StatusOK {
		t.Fatalf("refresh: status %d\n%s", r.Status, r.Body)
	}
	rotated := mediaCookie(r)
	if rotated == nil {
		t.Fatalf("refresh set no media cookie\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	if got := mediaGet(t, srv, url, nil, rotated); got != http.StatusOK {
		t.Fatalf("media cookie reissued by a rotation: status %d, want 200", got)
	}
}
