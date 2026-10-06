// Pre-registration invite hijack.
//
// An invitation is addressed to an email and stays pending for a week. The only
// proof that the addressee owns that mailbox is the token emailed to them — so
// registration must not hand out the membership to whoever types the email
// first. Two guards are asserted here:
//
//   - registering with someone else's invited email grants nothing;
//   - the invitee still lands in the workspace, by presenting the token from
//     the email (at register time) or by verifying the address afterwards.
package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"tessera/internal/auth"
)

// registerWith registers a specific email with an optional invitation token,
// returning the authenticated client.
func registerWith(t *testing.T, email, inviteToken string) *client {
	t.Helper()
	body := map[string]any{"email": email, "name": "Invited User", "password": "password-123"}
	if inviteToken != "" {
		body["invite_token"] = inviteToken
	}
	r := doReq(t, "", http.MethodPost, "/auth/register", body)
	if r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("register %s: status %d\n%s", email, r.Status, r.Body)
	}
	m := r.mapBody(t)
	tok, _ := m["access_token"].(string)
	user, _ := m["user"].(map[string]any)
	id, _ := user["id"].(string)
	return &client{t: t, token: tok, UserID: id, Email: email}
}

// verifyEmailOf consumes the user's pending verification token straight from the
// database — the mailer is a no-op under test, so there is no inbox to read.
func verifyEmailOf(t *testing.T, c *client) {
	t.Helper()
	// The DB keeps only the hash; the raw token lived in the email the no-op
	// mailer swallowed. So issue an equivalent one whose raw value we know and
	// drive the very same handler the emailed link would.
	raw := issueVerifyToken(t, c.UserID)
	r := doReq(t, "", http.MethodPost, "/auth/verify-email", map[string]any{"token": raw})
	if r.Status != http.StatusNoContent {
		t.Fatalf("verify-email: status %d\n%s", r.Status, r.Body)
	}
}

// issueVerifyToken inserts a verification token for the user and returns its raw
// value, standing in for the link the mailer would have delivered.
func issueVerifyToken(t *testing.T, userID string) string {
	t.Helper()
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("token pair: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO user_tokens (user_id, kind, token_hash, expires_at)
		 VALUES ($1, 'verify', $2, now() + interval '1 hour')`, userID, hash); err != nil {
		t.Fatalf("insert verify token: %v", err)
	}
	return raw
}

// A stranger who guesses the invited address must not inherit the invitation.
func TestRegisterDoesNotStealPendingInvitation(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с приглашением")

	victimEmail := fmt.Sprintf("victim-%d@test.local", userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": victimEmail, "role": "admin"}), http.StatusCreated)

	// The attacker knows only the address — no token — and registers first.
	attacker := registerWith(t, victimEmail, "")
	if listHasID(attacker.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("захват: регистрация по чужому приглашённому email дала доступ в %s", wsID)
	}
	// …and the invitation must still be pending for its real addressee.
	if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 1 {
		t.Fatalf("приглашение сгорело на чужой регистрации: pending = %d, want 1", got)
	}

	// The manager can still revoke it, the addressee having lost the address.
	owner.expect(t, owner.del("/workspaces/"+wsID+"/invitations/"+inv["id"].(string)), http.StatusNoContent)
}

// The invited user still joins — by presenting the emailed token at register.
func TestRegisterWithInviteTokenJoins(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с токеном")

	email := fmt.Sprintf("invited-%d@test.local", userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)
	token := invitationToken(t, inv)

	newcomer := registerWith(t, email, token)
	if !listHasID(newcomer.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("приглашённый с токеном не попал в пространство")
	}
	// Burned: a second registration cannot replay the same token.
	other := fmt.Sprintf("other-%d@test.local", userSeq.Add(1))
	replay := registerWith(t, other, token)
	if listHasID(replay.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("токен приглашения переиспользован чужим email")
	}
}

// A token addressed to someone else grants nothing, even at register time.
func TestRegisterWithForeignInviteTokenJoinsNothing(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство чужого токена")

	victim := fmt.Sprintf("victim2-%d@test.local", userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": victim, "role": "admin"}), http.StatusCreated)
	token := invitationToken(t, inv)

	attacker := fmt.Sprintf("attacker-%d@test.local", userSeq.Add(1))
	c := registerWith(t, attacker, token)
	if listHasID(c.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("чужой токен приглашения впустил %s в %s", attacker, wsID)
	}
}

// Verifying the address is the other proof of ownership: it applies whatever is
// pending for that email, so an invitee who registered without the link still
// lands in the workspace.
func TestVerifyEmailAppliesPendingInvitations(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с верификацией")

	email := fmt.Sprintf("late-%d@test.local", userSeq.Add(1))
	owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)

	late := registerWith(t, email, "")
	if listHasID(late.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("членство выдано до верификации адреса")
	}
	verifyEmailOf(t, late)
	if !listHasID(late.get("/workspaces").listBody(t), wsID) {
		t.Fatalf("после верификации приглашение так и не применилось")
	}
}
