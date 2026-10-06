// A recorded GitLab identity must not authenticate against a different GitLab
// instance.
//
// "Continue with GitLab" resolves the Tessera account from the numeric GitLab
// user id, and that number is per-instance: every GitLab hands out 1, 2, 3, …
// from its own sequence. So the id alone does not name a person — only the pair
// (instance, id) does, which is exactly why oauth_identities carries
// gl_base_url. When the instance-wide provider row is pointed somewhere else —
// a move from gitlab.com to self-hosted, a restore whose ids restarted, a
// staging instance pointed at by mistake — the stored ids go on matching, and
// whoever holds the matching number on the new instance signs straight into the
// account that number used to mean.
//
// The fake GitLab is the same two endpoints the login flow touches
// (newFakeIDP); what this test needs is two of them.
package main

import (
	"context"
	"fmt"
	"net/url"
	"testing"
)

// TestOAuthIdentityDoesNotCrossInstances walks the whole move: a user signs in
// while the instance points at one GitLab, the admin repoints it at another, and
// an unrelated person holding the same numeric id there signs in.
func TestOAuthIdentityDoesNotCrossInstances(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	glID := 930000 + n // the same number on both instances

	// Instance A: the incumbent signs in and gets an account, so the identity row
	// (gitlab, glID) → that account exists and names instance A.
	firstEmail := fmt.Sprintf("gl-inst-a-%d@test.local", n)
	configureIDP(t, newFakeIDP(t, glID, fmt.Sprintf("gl-inst-a-%d", n), firstEmail).URL)
	frag := gitlabSignIn(t)
	tok := frag.Get("access_token")
	if tok == "" {
		t.Fatalf("первый вход через инстанс A не прошёл: %v", frag)
	}
	incumbent, _ := whoami(t, tok)["id"].(string)
	if incumbent == "" {
		t.Fatalf("первый вход не вернул учётку: %v", frag)
	}

	// The move. Same provider row, another GitLab — and on it, user number glID
	// is somebody else entirely: another login, another address.
	strangerEmail := fmt.Sprintf("gl-inst-b-%d@test.local", n)
	configureIDP(t, newFakeIDP(t, glID, fmt.Sprintf("gl-inst-b-%d", n), strangerEmail).URL)

	frag = gitlabSignIn(t)
	tok = frag.Get("access_token")
	if tok == "" {
		t.Fatalf("вход через инстанс B не прошёл вовсе: %v", frag)
	}
	got := whoami(t, tok)
	if id, _ := got["id"].(string); id == incumbent {
		t.Fatalf("захват: пользователь №%d с ДРУГОГО инстанса GitLab (%s) вошёл в учётку %s, "+
			"заведённую под тем же номером на прежнем инстансе (%s)",
			glID, strangerEmail, incumbent, firstEmail)
	}
	// Nothing was matched, so the stranger is simply a new account on their own
	// address — the ordinary auto-provision path.
	if email, _ := got["email"].(string); email != strangerEmail {
		t.Fatalf("учётка для пользователя инстанса B заведена не на свой адрес: %q, want %q",
			email, strangerEmail)
	}
}

// TestOAuthIdentitySurvivesSameInstanceRename is the counterweight: the guard
// must not lock people out when the URL of the *same* instance changes shape —
// a cosmetic https:// or a DNS rename. The id stops matching, but the account
// that GitLab auto-provisioned holds a pre-verified address (CreateOAuthUser),
// so the email leg links it back and the user lands where they were.
func TestOAuthIdentitySurvivesSameInstanceRename(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	glID := 940000 + n
	email := fmt.Sprintf("gl-rename-%d@test.local", n)
	username := fmt.Sprintf("gl-rename-%d", n)

	idp := newFakeIDP(t, glID, username, email)
	configureIDP(t, idp.URL)
	frag := gitlabSignIn(t)
	tok := frag.Get("access_token")
	if tok == "" {
		t.Fatalf("первый вход не прошёл: %v", frag)
	}
	before, _ := whoami(t, tok)["id"].(string)

	// Same GitLab, same person, another spelling of the address. httptest hands
	// out 127.0.0.1; "localhost" is the same server under another name.
	renamed := "http://localhost:" + portOf(t, idp.URL)
	configureIDP(t, renamed)

	frag = gitlabSignIn(t)
	tok = frag.Get("access_token")
	if tok == "" {
		t.Fatalf("после переименования того же инстанса вход не прошёл: %v", frag)
	}
	if after, _ := whoami(t, tok)["id"].(string); after != before {
		t.Fatalf("переименование инстанса увело пользователя в другую учётку: %s → %s", before, after)
	}
	// And the row now names the new spelling, so the next login matches by id again.
	var stored string
	if err := testPool.QueryRow(context.Background(),
		`SELECT gl_base_url FROM oauth_identities WHERE provider = 'gitlab' AND provider_user_id = $1`,
		fmt.Sprint(glID)).Scan(&stored); err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if stored != renamed {
		t.Fatalf("gl_base_url не обновился на новое имя инстанса: %q, want %q", stored, renamed)
	}
}

// portOf pulls the port out of an httptest base URL.
func portOf(t *testing.T, baseURL string) string {
	t.Helper()
	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse %q: %v", baseURL, err)
	}
	if u.Port() == "" {
		t.Fatalf("no port in %q", baseURL)
	}
	return u.Port()
}
