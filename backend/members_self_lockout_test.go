// A workspace manager must not be able to strip their own management rights
// (#2932).
//
// The rule already exists twice in this codebase — handlers/admin.go refuses
// "cannot change your own admin role" and handlers/users.go refuses "cannot
// change your own active state" — for the same reason both times: the one
// person who cannot undo the change is the one making it. The workspace member
// routes guarded only the *owner*, whose role is immutable anyway, and left
// every admin free to demote or delete themselves with a control that looks
// exactly like the one on everybody else's row.
//
// The neighbouring authz_owner_role_test.go covers the other direction (an admin
// taking the workspace from its owner). This one covers the direction where the
// only person harmed is the one who acted — which is why nothing catches it in
// review: there is no victim to complain.
//
// That self-removal is a missing catch rather than a way out is settled by the
// route table: RemoveMember sits behind requireManager, so a plain member cannot
// remove themselves either, and there is no leave-workspace route at all.
package main

import (
	"net/http"
	"testing"
)

func TestAdminCannotDemoteThemselves(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство без предохранителя")
	admin := mkAdmin(t, owner, wsID)

	body := admin.expect(t, admin.patch("/workspaces/"+wsID+"/members/"+admin.UserID,
		map[string]any{"role": "member"}), http.StatusForbidden)
	if body["error"] != "cannot change your own role" {
		t.Fatalf("error = %v, want %q", body["error"], "cannot change your own role")
	}

	// The refusal has to be the whole story: a 403 that still wrote the row would
	// lock the admin out just as thoroughly, only more confusingly.
	if role := memberRoleOf(t, owner, wsID, admin.UserID); role != "admin" {
		t.Fatalf("роль после отклонённого самопонижения = %q, ожидалась admin", role)
	}
}

func TestAdminCannotRemoveThemselves(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство без выхода")
	admin := mkAdmin(t, owner, wsID)

	body := admin.expect(t, admin.del("/workspaces/"+wsID+"/members/"+admin.UserID),
		http.StatusForbidden)
	if body["error"] != "cannot remove yourself from the workspace" {
		t.Fatalf("error = %v, want %q", body["error"], "cannot remove yourself from the workspace")
	}

	// Still a member, and still able to work — RemoveMember cuts live sockets
	// with revokeUserSockets, so a half-applied removal is felt immediately.
	admin.expect(t, admin.get("/workspaces/"+wsID+"/members"), http.StatusOK)
	if role := memberRoleOf(t, owner, wsID, admin.UserID); role != "admin" {
		t.Fatalf("роль после отклонённого самоудаления = %q, ожидалась admin", role)
	}
}

// The add-member route is an upsert (see the comment on CreateMembership), so it
// carries the same role change as PATCH — by a different door. An admin typing
// their own address into the one input the dialog offers, with the role select
// on its "member" default, demoted themselves and got a green "added" toast.
func TestAdminCannotDemoteThemselvesByReAdding(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с чёрным ходом")
	admin := mkAdmin(t, owner, wsID)

	body := admin.expect(t, admin.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": admin.Email, "role": "member"}), http.StatusForbidden)
	if body["error"] != "cannot change your own role" {
		t.Fatalf("error = %v, want %q", body["error"], "cannot change your own role")
	}
	if role := memberRoleOf(t, owner, wsID, admin.UserID); role != "admin" {
		t.Fatalf("роль после отклонённого повторного добавления = %q, ожидалась admin", role)
	}
}

// Controls — the guard is about the *caller*, not about admins in general. A
// manager must keep every power over everyone else, or the fix trades one
// unmanageable workspace for another.
func TestManagerStillManagesOtherMembers(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с рабочим управлением")
	admin := mkAdmin(t, owner, wsID)

	other := signup(t)
	admin.expect(t, admin.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": other.Email, "role": "member"}), http.StatusCreated)
	admin.expect(t, admin.patch("/workspaces/"+wsID+"/members/"+other.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)
	// Re-adding someone else still upserts their role, which is the documented
	// way a manager changes it.
	admin.expect(t, admin.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": other.Email, "role": "member"}), http.StatusCreated)
	if role := memberRoleOf(t, owner, wsID, other.UserID); role != "member" {
		t.Fatalf("роль другого участника после повторного добавления = %q, ожидалась member", role)
	}
	if r := admin.del("/workspaces/" + wsID + "/members/" + other.UserID); r.Status != http.StatusNoContent {
		t.Fatalf("удаление другого участника = %d, ожидалось 204\n%s", r.Status, r.Body)
	}

	// And the owner keeps their own separate protection, phrased as before: the
	// new sentinel must not swallow the older one.
	got := admin.expect(t, admin.patch("/workspaces/"+wsID+"/members/"+owner.UserID,
		map[string]any{"role": "member"}), http.StatusForbidden)
	if got["error"] != "cannot change the workspace owner's role" {
		t.Fatalf("отказ на владельце = %v, ожидался прежний сентинел", got["error"])
	}
}

// The owner is the one member both guards claim, so their order has to be a
// decision rather than an accident: the owner rule answers first, because "the
// workspace owner cannot be removed" is true for everyone who tries it and does
// not change when the person trying happens to be the owner. Pinned here so the
// self guard cannot be slipped in above it and quietly re-word a refusal three
// other tests already assert.
func TestOwnerRemovingThemselvesStillHearsTheOwnerRule(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство своего владельца")

	body := owner.expect(t, owner.del("/workspaces/"+wsID+"/members/"+owner.UserID),
		http.StatusForbidden)
	if body["error"] != "cannot remove the workspace owner" {
		t.Fatalf("error = %v, want %q", body["error"], "cannot remove the workspace owner")
	}
	if role := memberRoleOf(t, owner, wsID, owner.UserID); role != "owner" {
		t.Fatalf("роль владельца после отказа = %q, ожидалась owner", role)
	}
}
