package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestAssigneeMustBeWorkspaceMember pins the authorization of the user_id a
// caller sends in the *body* of POST /tasks/:id/assignees. loadTask vouches for
// the task in the route, not for the user: an unchecked user_id assigns a total
// stranger to the caller's own task, and two things follow.
//
//  1. ListTaskAssignees selects u.email and u.name, so the task detail reads
//     that stranger's address back to the caller — an address-book oracle over
//     the whole installation, keyed by user UUID.
//  2. applyAssignee notifies the assignee ("Вам назначена задача «…»") with a
//     title the caller controls, from a workspace the victim is not in and
//     cannot open.
//
// The sibling notification paths already guard this: notifyMentions and
// notifyThread both skip ids that fail GetMembership. The assignee path is the
// one that forgot. The quick-action arm (`/assign <name>`) resolves names among
// the workspace's own members and was never affected.
func TestAssigneeMustBeWorkspaceMember(t *testing.T) {
	t.Parallel()
	victim := signup(t)

	attacker := signup(t) // member of nothing the victim owns
	as := mkStack(t, attacker)
	mine := mkTask(t, attacker, as.Board, as.col(t, 0), "Счёт на оплату, срочно")
	mineID := mine["id"].(string)

	t.Run("assigning a non-member is refused", func(t *testing.T) {
		r := attacker.post("/tasks/"+mineID+"/assignees", map[string]any{"user_id": victim.UserID})
		if r.Status != http.StatusForbidden {
			t.Fatalf("assign non-member: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("the stranger's email does not leak into the task", func(t *testing.T) {
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		gained, _ := got["assignees"].([]any)
		if len(gained) != 0 {
			t.Fatalf("task gained %d assignees: %v", len(gained), gained)
		}
		if strings.Contains(string(attacker.get("/tasks/"+mineID).Body), victim.Email) {
			t.Fatalf("victim address %s leaked into the task detail", victim.Email)
		}
	})

	t.Run("the stranger gets no notification", func(t *testing.T) {
		list := victim.get("/notifications").listBody(t)
		for _, n := range list {
			if kind, _ := n["kind"].(string); kind == "assigned" {
				t.Fatalf("victim received an assignment notification from a foreign workspace: %v", n)
			}
		}
	})

	t.Run("an unknown user_id is refused too", func(t *testing.T) {
		r := attacker.post("/tasks/"+mineID+"/assignees",
			map[string]any{"user_id": "00000000-0000-0000-0000-000000000001"})
		if r.Status != http.StatusForbidden {
			t.Fatalf("assign unknown user: got %d %s, want 403", r.Status, r.Body)
		}
	})

	t.Run("a member of the caller's own workspace still assigns", func(t *testing.T) {
		attacker.expect(t, attacker.post("/tasks/"+mineID+"/assignees",
			map[string]any{"user_id": attacker.UserID}), http.StatusNoContent)
		got := attacker.expect(t, attacker.get("/tasks/"+mineID), http.StatusOK)
		list, _ := got["assignees"].([]any)
		if len(list) != 1 {
			t.Fatalf("own assignment did not stick: %v", got["assignees"])
		}
		attacker.expect(t, attacker.del("/tasks/"+mineID+"/assignees/"+attacker.UserID),
			http.StatusNoContent)
	})
}

// TestRevokedMemberCanStillBeUnassigned pins the deliberate asymmetry in the
// membership check above: it guards the *add* arm only.
//
// RemoveMember drops the membership row and leaves the member's existing task
// assignments standing (only a project transfer sweeps them, via
// StripNonMemberAssignees). Guarding the remove arm too would therefore strand
// those rows for good — the ex-member stays assigned, the UI's × returns 403 and
// nobody can clean it up. Removing is also harmless: it deletes a row on a task
// the caller already holds and answers 204 whether or not the row was there, so
// it is not an existence oracle either.
func TestRevokedMemberCanStillBeUnassigned(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "задача уходящего")
	taskID := task["id"].(string)
	owner.expect(t, owner.post("/tasks/"+taskID+"/assignees",
		map[string]any{"user_id": member.UserID}), http.StatusNoContent)

	owner.expect(t, owner.del("/workspaces/"+s.WS+"/members/"+member.UserID), http.StatusNoContent)

	owner.expect(t, owner.del("/tasks/"+taskID+"/assignees/"+member.UserID), http.StatusNoContent)
	got := owner.expect(t, owner.get("/tasks/"+taskID), http.StatusOK)
	if list, _ := got["assignees"].([]any); len(list) != 0 {
		t.Fatalf("stale assignee survived the unassign: %v", list)
	}
}
