// A pending invitation must not lower a role granted after it was written.
//
// The role inside an invitation is a snapshot of what somebody intended at the
// moment they typed the address — and an invitation stays pending for a week.
// Applying it is an upsert (CreateMembership), so when it finally lands it
// overwrites whatever the membership says now. The owner is safe: the statement
// itself refuses to move an 'owner' row. Everything below owner is not, and the
// ordinary sequence is enough to lose a role nobody meant to take away:
//
//	invite X as member → X registers → owner promotes X to admin →
//	X follows the week-old link (or merely verifies their address, or signs in
//	through GitLab) → X is a member again, silently.
//
// Same rule as org_map already follows after #2932: a grant that arrives from
// outside the members screen may raise a role, never lower it. Deliberate
// demotion keeps its own door — PATCH /workspaces/:id/members/:userId.
package main

import (
	"fmt"
	"net/http"
	"testing"
)

// invitedThenPromoted sets the trap all three accept paths share: a pending
// member-invitation for someone the owner has since made an admin.
func invitedThenPromoted(t *testing.T, tag string) (owner *client, wsID string, invitee *client, token string) {
	t.Helper()
	owner = signup(t)
	wsID = mkWorkspace(t, owner, "Пространство с протухшим приглашением")

	email := fmt.Sprintf("%s-%d@test.local", tag, userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)
	token = invitationToken(t, inv)

	// The invitee registers without the token — the link is still in their inbox.
	invitee = registerWith(t, email, "")
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": email, "role": "admin"}), http.StatusCreated)
	if role := memberRoleOf(t, owner, wsID, invitee.UserID); role != "admin" {
		t.Fatalf("роль до принятия приглашения = %q, want admin", role)
	}
	return owner, wsID, invitee, token
}

// TestStaleInvitationDoesNotDemoteAdmin walks the explicit accept: the invitee
// clicks the week-old link from their mailbox.
func TestStaleInvitationDoesNotDemoteAdmin(t *testing.T) {
	t.Parallel()
	owner, wsID, invitee, token := invitedThenPromoted(t, "stale-invite-accept")

	invitee.expect(t, invitee.post("/invitations/accept",
		map[string]any{"token": token}), http.StatusOK)

	if role := memberRoleOf(t, owner, wsID, invitee.UserID); role != "admin" {
		t.Fatalf("роль после принятия приглашения = %q, want admin", role)
	}
	// The role is what authorises: a demoted admin loses the members screen.
	invitee.expect(t, invitee.get("/workspaces/"+wsID+"/invitations"), http.StatusOK)

	// Refusing the demotion must not leave the invitation live for another week —
	// it was accepted, it just had nothing left to grant.
	if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 0 {
		t.Fatalf("pending приглашений = %d, want 0 — приглашение не сожжено", got)
	}
}

// TestStaleInvitationDoesNotDemoteOnVerify walks the path that needs no click on
// the link at all: acceptPendingInvitations fires on email verification (and on
// every OAuth sign-in), matching on the address alone.
func TestStaleInvitationDoesNotDemoteOnVerify(t *testing.T) {
	t.Parallel()
	owner, wsID, invitee, _ := invitedThenPromoted(t, "stale-invite-verify")

	verifyEmailOf(t, invitee)

	if role := memberRoleOf(t, owner, wsID, invitee.UserID); role != "admin" {
		t.Fatalf("роль после верификации адреса = %q, want admin", role)
	}
	if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 0 {
		t.Fatalf("pending приглашений = %d, want 0 — приглашение не сожжено", got)
	}
}

// The invitation must still do its job when it is the only thing granting
// access, and it must still be able to raise a role.
func TestInvitationStillGrantsAndPromotes(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с рабочим приглашением")

	email := fmt.Sprintf("invite-grants-%d@test.local", userSeq.Add(1))
	plain := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)
	invitee := registerWith(t, email, "")
	invitee.expect(t, invitee.post("/invitations/accept",
		map[string]any{"token": invitationToken(t, plain)}), http.StatusOK)
	if role := memberRoleOf(t, owner, wsID, invitee.UserID); role != "member" {
		t.Fatalf("роль после первого приглашения = %q, want member", role)
	}

	// A second invitation, this time for admin: raising is what the rule allows.
	up := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": email, "role": "admin"}), http.StatusCreated)
	invitee.expect(t, invitee.post("/invitations/accept",
		map[string]any{"token": invitationToken(t, up)}), http.StatusOK)
	if role := memberRoleOf(t, owner, wsID, invitee.UserID); role != "admin" {
		t.Fatalf("роль после повышающего приглашения = %q, want admin", role)
	}
}
