// The workspace owner's role must survive every path that writes a membership.
//
// UpdateMemberRole and RemoveMember both stop at the owner with a 403 — the role
// is meant to be immutable outside the (not yet built) ownership transfer. But
// both guards sit in the handler, while the write underneath them is an upsert:
// CreateMembership does ON CONFLICT DO UPDATE SET role = EXCLUDED.role, and the
// three callers that reach it — AddMember, and the two invitation-accept paths —
// never ask who the target is. So the guarded operation is available unguarded
// one route over, and an admin can take the workspace from its owner: after the
// demotion requireManager reads "member", and the owner loses members, roles,
// invitations and settings in the workspace they created.
package main

import (
	"fmt"
	"net/http"
	"testing"
)

// memberRoleOf reads a member's role out of the workspace member list.
func memberRoleOf(t *testing.T, c *client, wsID, userID string) string {
	t.Helper()
	for _, m := range c.get("/workspaces/" + wsID + "/members").listBody(t) {
		if id, _ := m["user_id"].(string); id == userID {
			role, _ := m["role"].(string)
			return role
		}
	}
	t.Fatalf("пользователь %s не найден среди членов %s", userID, wsID)
	return ""
}

// mkAdmin signs up a user and makes them an admin of the workspace.
func mkAdmin(t *testing.T, owner *client, wsID string) *client {
	t.Helper()
	admin := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": admin.Email, "role": "admin"}), http.StatusCreated)
	return admin
}

// TestAdminCannotDemoteOwnerViaAddMember walks the takeover the short way: the
// add-member route carries the same role field as the patch that refuses it.
func TestAdminCannotDemoteOwnerViaAddMember(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство под захватом")
	admin := mkAdmin(t, owner, wsID)

	// The guarded route says no — that is the intent this test holds the other
	// routes to.
	admin.expect(t, admin.patch("/workspaces/"+wsID+"/members/"+owner.UserID,
		map[string]any{"role": "member"}), http.StatusForbidden)

	if r := admin.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": owner.Email, "role": "member"}); r.Status < 400 {
		t.Errorf("добавление участника прошло на владельце: status %d\n%s", r.Status, r.Body)
	}

	if role := memberRoleOf(t, owner, wsID, owner.UserID); role != "owner" {
		t.Fatalf("роль владельца после атаки = %q, want owner", role)
	}
	// The role is what authorises: if it moved, the owner is locked out of their
	// own workspace's management.
	owner.expect(t, owner.get("/workspaces/"+wsID+"/invitations"), http.StatusOK)
	newcomer := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": newcomer.Email, "role": "member"}), http.StatusCreated)
}

// TestInvitationCannotDemoteOwner walks it the long way: an invitation addressed
// to the owner's own mailbox, applied — without any click on the owner's part
// beyond verifying their address — the next time they prove they own it.
func TestInvitationCannotDemoteOwner(t *testing.T) {
	t.Parallel()
	email := fmt.Sprintf("owner-invited-%d@test.local", userSeq.Add(1))
	owner := registerWith(t, email, "")
	wsID := mkWorkspace(t, owner, "Пространство с приглашением владельцу")
	admin := mkAdmin(t, owner, wsID)

	admin.post("/workspaces/"+wsID+"/invitations",
		map[string]any{"email": owner.Email, "role": "member"})

	// A routine, unrelated action by the victim.
	verifyEmailOf(t, owner)

	if role := memberRoleOf(t, owner, wsID, owner.UserID); role != "owner" {
		t.Fatalf("роль владельца после верификации адреса = %q, want owner", role)
	}
	owner.expect(t, owner.get("/workspaces/"+wsID+"/invitations"), http.StatusOK)
}

// The upsert still has its job: re-adding an ordinary member with a new role is
// how a manager promotes someone, and that must keep working.
func TestAddMemberStillChangesOrdinaryRole(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с повышением")
	plain := signup(t)

	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": plain.Email, "role": "member"}), http.StatusCreated)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": plain.Email, "role": "admin"}), http.StatusCreated)

	if role := memberRoleOf(t, owner, wsID, plain.UserID); role != "admin" {
		t.Fatalf("роль обычного участника после повторного добавления = %q, want admin", role)
	}
}
