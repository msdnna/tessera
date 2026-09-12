// Pre-registration account hijack through OAuth email linking.
//
// "Continue with GitLab" finds the Tessera account for a GitLab identity, and
// when the identity is new it falls back to matching by email address. GitLab
// vouches for its side of that address — but the local side is whatever someone
// typed at registration, which nobody ever checked. So the match has to require
// the local account to have proven the mailbox too, otherwise the first person
// to register with a colleague's address inherits their GitLab sign-in (and,
// through org_map, their workspaces).
//
// The fake GitLab here only needs the two endpoints the login flow touches:
// /oauth/token and /api/v4/user.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
)

// newFakeIDP stands in for the self-hosted GitLab: any code exchanges for a
// token, and that token identifies exactly one user.
func newFakeIDP(t *testing.T, glID int64, username, email string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-access-token"})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": glID, "username": username, "email": email, "name": "GitLab User",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// configureIDP points the instance-wide GitLab OAuth app at the fake. The
// provider row is a global singleton, so every caller must be a sequential test
// and clears the row afterwards.
func configureIDP(t *testing.T, baseURL string) {
	t.Helper()
	admin := signup(t)
	makeAdmin(t, admin)
	t.Cleanup(func() {
		// Back to pristine, secret included: the parallel config test that resumes
		// after the sequential ones asserts the row reads as unconfigured.
		_, _ = testPool.Exec(context.Background(),
			`UPDATE oauth_providers SET gl_base_url = '', enabled = false, client_id = '', client_secret_enc = ''
			 WHERE provider = 'gitlab'`)
	})
	admin.expect(t, admin.put("/admin/oauth/gitlab", map[string]any{
		"gl_base_url": baseURL, "client_id": "tessera-app", "client_secret": "s3cret", "enabled": true,
	}), http.StatusOK)
}

// gitlabSignIn drives the whole browser leg — authorize, then callback with the
// state the authorize redirect minted — and returns the fragment/query of the
// final redirect. A cookie jar carries the CSRF state cookie between the two.
func gitlabSignIn(t *testing.T) url.Values {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	cl := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := cl.Get(testServer.URL + "/api/auth/gitlab/authorize")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	_ = res.Body.Close()
	authorizeTo, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize redirect: %v", err)
	}
	state := authorizeTo.Query().Get("state")
	if state == "" {
		t.Fatalf("authorize did not mint a state: %s", res.Header.Get("Location"))
	}
	res, err = cl.Get(testServer.URL + "/api/auth/gitlab/callback?code=any&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	_ = res.Body.Close()
	back, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse callback redirect: %v", err)
	}
	out, err := url.ParseQuery(back.Fragment)
	if err != nil {
		t.Fatalf("parse fragment: %v", err)
	}
	for k, v := range back.Query() {
		out[k] = v
	}
	return out
}

// whoami resolves the account an access token belongs to.
func whoami(t *testing.T, token string) map[string]any {
	t.Helper()
	r := doReq(t, token, http.MethodGet, "/auth/me", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("/auth/me: status %d\n%s", r.Status, r.Body)
	}
	user, _ := r.mapBody(t)["user"].(map[string]any)
	if user == nil {
		t.Fatalf("/auth/me without a user: %s", r.Body)
	}
	return user
}

// The attacker registers first with the victim's address — unverified, because
// typing an address is not owning it — and must not catch the victim's GitLab
// sign-in.
func TestOAuthDoesNotLinkUnverifiedLocalAccount(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	victimEmail := fmt.Sprintf("gl-victim-%d@test.local", n)

	attacker := registerWith(t, victimEmail, "")
	configureIDP(t, newFakeIDP(t, 900000+n, fmt.Sprintf("gl-victim-%d", n), victimEmail).URL)

	frag := gitlabSignIn(t)
	if tok := frag.Get("access_token"); tok != "" {
		if got := whoami(t, tok)["id"]; got == attacker.UserID {
			t.Fatalf("захват: вход через GitLab отдал сессию в чужую локальную учётку %s (%s)",
				attacker.UserID, victimEmail)
		}
		t.Fatalf("вход прошёл, хотя адрес занят неподтверждённой учёткой: %v", frag)
	}
	if reason := frag.Get("oauth_error"); reason != "email_taken" {
		t.Fatalf("oauth_error = %q, want email_taken (%v)", reason, frag)
	}
	// The attacker's own account is untouched — no GitLab identity was attached.
	var linked int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_identities WHERE user_id = $1::uuid`, attacker.UserID).Scan(&linked); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if linked != 0 {
		t.Fatalf("к чужой учётке привязали GitLab-личность: %d", linked)
	}
}

// The legitimate case still works: an account that proved the mailbox links to
// the GitLab identity with the same address and signs in.
func TestOAuthLinksVerifiedLocalAccount(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	email := fmt.Sprintf("gl-owner-%d@test.local", n)

	owner := registerWith(t, email, "")
	verifyEmailOf(t, owner)
	configureIDP(t, newFakeIDP(t, 910000+n, fmt.Sprintf("gl-owner-%d", n), email).URL)

	frag := gitlabSignIn(t)
	tok := frag.Get("access_token")
	if tok == "" {
		t.Fatalf("подтверждённая учётка не вошла через GitLab: %v", frag)
	}
	if got := whoami(t, tok)["id"]; got != owner.UserID {
		t.Fatalf("вход отдал сессию не той учётке: %v, want %s", got, owner.UserID)
	}
}

// An address nobody holds locally still auto-provisions an account, as before.
func TestOAuthProvisionsUnknownEmail(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	email := fmt.Sprintf("gl-fresh-%d@test.local", n)
	configureIDP(t, newFakeIDP(t, 920000+n, fmt.Sprintf("gl-fresh-%d", n), email).URL)

	frag := gitlabSignIn(t)
	tok := frag.Get("access_token")
	if tok == "" {
		t.Fatalf("новый GitLab-пользователь не завёлся: %v", frag)
	}
	if got := whoami(t, tok)["email"]; got != email {
		t.Fatalf("завели учётку не на тот адрес: %v, want %s", got, email)
	}
}
