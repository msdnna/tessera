// Revoking an invitation must be scoped to the workspace in the URL.
//
// DELETE /workspaces/:id/invitations/:invId checks that the caller manages :id,
// and every sibling on this route family then ties the nested id back to that
// workspace — member removal scopes its delete by workspace_id, the GitLab
// routes resolve their binding through integrationInWorkspace. This one did not:
// the id travelled straight into the statement, so managing any workspace was
// enough to burn a pending invitation belonging to any other.
//
// The realistic door is a demoted admin: while they managed the workspace they
// could list its invitations, ids and all, and an invitation stays pending for a
// week. Losing the role took nothing away — they own a workspace of their own,
// which is all the gate asked for.
package main

import (
	"fmt"
	"net/http"
	"testing"
)

// TestInvitationDeleteIsWorkspaceScoped drives the revoke from outside the
// workspace that owns the invitation, then checks the invitation is still good
// for the person it was addressed to.
func TestInvitationDeleteIsWorkspaceScoped(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с живым приглашением")

	invited := fmt.Sprintf("invitee-scope-%d@test.local", userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": invited, "role": "member"}), http.StatusCreated)
	invID, _ := inv["id"].(string)
	token := invitationToken(t, inv)

	// The attacker manages only their own workspace — the gate's whole demand.
	attacker := signup(t)
	otherWS := mkWorkspace(t, attacker, "Пространство атакующего")

	t.Run("чужое пространство в URL", func(t *testing.T) {
		r := attacker.del("/workspaces/" + otherWS + "/invitations/" + invID)
		if r.Status == http.StatusNoContent {
			t.Fatalf("отзыв прошёл: приглашение %s сожжено из чужого пространства", invID)
		}
		if r.Status != http.StatusNotFound && r.Status != http.StatusForbidden {
			t.Fatalf("status %d, want 404 или 403\n%s", r.Status, r.Body)
		}
	})

	t.Run("приглашение всё ещё висит", func(t *testing.T) {
		if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 1 {
			t.Fatalf("pending = %d, want 1", got)
		}
	})

	t.Run("адресат по-прежнему входит по токену", func(t *testing.T) {
		newcomer := registerWith(t, invited, token)
		if !listHasID(newcomer.get("/workspaces").listBody(t), wsID) {
			t.Fatalf("адресат не попал в %s — приглашение всё-таки сгорело", wsID)
		}
	})
}

// TestDemotedAdminCannotRevokeInvitations is the same hole through the door it
// would actually be walked through: someone who legitimately saw the ids, and
// then lost the role that let them see them.
func TestDemotedAdminCannotRevokeInvitations(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с понижением")

	admin := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": admin.Email, "role": "admin"}), http.StatusCreated)
	ownWS := mkWorkspace(t, admin, "Пространство бывшего админа") // their own, still managed

	invited := fmt.Sprintf("invitee-demote-%d@test.local", userSeq.Add(1))
	owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": invited, "role": "member"}), http.StatusCreated)

	// Seen while still an admin, remembered after.
	seen := admin.get("/workspaces/" + wsID + "/invitations").listBody(t)
	if len(seen) != 1 {
		t.Fatalf("админ видит %d приглашений, want 1", len(seen))
	}
	invID, _ := seen[0]["id"].(string)

	owner.expect(t, owner.patch("/workspaces/"+wsID+"/members/"+admin.UserID,
		map[string]any{"role": "member"}), http.StatusOK)

	// Demoted, they must not reach the invitation from any angle: neither through
	// the workspace they no longer manage, nor through the one they still do.
	admin.expect(t, admin.del("/workspaces/"+wsID+"/invitations/"+invID), http.StatusForbidden)
	if r := admin.del("/workspaces/" + ownWS + "/invitations/" + invID); r.Status == http.StatusNoContent {
		t.Fatalf("бывший админ отозвал приглашение через своё пространство")
	}
	if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 1 {
		t.Fatalf("pending после понижения = %d, want 1", got)
	}
}

// The manager of the workspace that owns the invitation still revokes it — the
// scope must narrow the statement, not close it.
func TestManagerStillRevokesOwnInvitation(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство со своим отзывом")

	invited := fmt.Sprintf("invitee-own-%d@test.local", userSeq.Add(1))
	inv := owner.expect(t, owner.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": invited, "role": "member"}), http.StatusCreated)

	owner.expect(t, owner.del("/workspaces/"+wsID+"/invitations/"+inv["id"].(string)), http.StatusNoContent)
	if got := len(owner.get("/workspaces/" + wsID + "/invitations").listBody(t)); got != 0 {
		t.Fatalf("pending после отзыва = %d, want 0", got)
	}
}
