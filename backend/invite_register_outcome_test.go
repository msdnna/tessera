// What registration tells a client about the invitation token it presented.
//
// The account is created whatever becomes of the token, and the session that
// comes back looks the same either way — so a newcomer whose invitation was
// addressed to another mailbox, or whose link had already been spent, used to
// land on the board with a personal workspace, no membership and nothing said.
// The link is routinely pasted into a chat rather than mailed (that is what
// CreateInvitation returns it for when SMTP is off), so the invited address is
// the one thing the invitee cannot look up before typing their own (#2932).
//
// Asserted here: the register reply names the outcome, and names it correctly
// for each of the ways a token can fail to grant anything.
package main

import (
	"fmt"
	"net/http"
	"testing"
)

// registerOutcome registers an email with a token and returns the reply's
// `invite` field — "" when the server said nothing about one.
func registerOutcome(t *testing.T, email, inviteToken string) string {
	t.Helper()
	body := map[string]any{"email": email, "name": "Invited User", "password": "password-123"}
	if inviteToken != "" {
		body["invite_token"] = inviteToken
	}
	r := doReq(t, "", http.MethodPost, "/auth/register", body)
	if r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("register %s: status %d\n%s", email, r.Status, r.Body)
	}
	out, _ := r.mapBody(t)["invite"].(string)
	return out
}

// newInvitation invites a fresh address to a fresh workspace and returns the
// address and the raw token from the link.
func newInvitation(t *testing.T, role string) (email, token string) {
	t.Helper()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство исхода приглашения")
	email = fmt.Sprintf("outcome-%d@test.local", userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": role}), http.StatusCreated)
	return email, invitationToken(t, inv)
}

// The invitee who typed the invited address is told the invitation took.
func TestRegisterReportsInviteApplied(t *testing.T) {
	t.Parallel()
	email, token := newInvitation(t, "member")

	if got := registerOutcome(t, email, token); got != "applied" {
		t.Fatalf("исход приглашения при совпавшем адресе = %q, want applied", got)
	}
}

// The common miss: the link was pasted into a chat, so the invitee never saw the
// invited mailbox and signed up with the address they always use. Registration
// succeeds and grants nothing — and that has to be said out loud, because the
// only copy of the token is back in the message.
func TestRegisterReportsInviteEmailMismatch(t *testing.T) {
	t.Parallel()
	_, token := newInvitation(t, "member")

	other := fmt.Sprintf("mismatch-%d@test.local", userSeq.Add(1))
	if got := registerOutcome(t, other, token); got != "email_mismatch" {
		t.Fatalf("исход приглашения на чужой адрес = %q, want email_mismatch", got)
	}
}

// A token already spent (or revoked, or a week old) is not an address problem:
// switching accounts would not help, a new invitation is the only way in.
func TestRegisterReportsInviteInvalid(t *testing.T) {
	t.Parallel()
	email, token := newInvitation(t, "member")

	if got := registerOutcome(t, email, token); got != "applied" {
		t.Fatalf("первый приём токена = %q, want applied", got)
	}
	replay := fmt.Sprintf("replay-%d@test.local", userSeq.Add(1))
	if got := registerOutcome(t, replay, token); got != "invalid" {
		t.Fatalf("исход повторно предъявленного токена = %q, want invalid", got)
	}
	unknown := fmt.Sprintf("unknown-%d@test.local", userSeq.Add(1))
	if got := registerOutcome(t, unknown, "not-a-token"); got != "invalid" {
		t.Fatalf("исход выдуманного токена = %q, want invalid", got)
	}
}

// A plain sign-up says nothing about invitations — the field is absent, so the
// register form has no reason to send anyone to the accept page.
func TestRegisterWithoutTokenReportsNoInvite(t *testing.T) {
	t.Parallel()
	plain := fmt.Sprintf("plain-%d@test.local", userSeq.Add(1))

	if got := registerOutcome(t, plain, ""); got != "" {
		t.Fatalf("обычная регистрация вернула исход приглашения %q, want пусто", got)
	}
}
