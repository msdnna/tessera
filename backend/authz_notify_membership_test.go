package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestRemovedMemberStopsHearingAboutTasks pins the membership gate on the
// notification seam.
//
// RemoveMember deletes the membership row and nothing else — task_assignees
// references users, not memberships, so an ex-member's assignment rows stay
// standing (TestRevokedMemberCanStillBeUnassigned documents that on purpose).
// notifyTaskParticipants then walks exactly those rows, so every comment on a
// task the departed member was assigned to kept reaching them, carrying the
// comment *body* in the notification text. The sibling paths in the same file
// already guard this — notifyMentions and notifyThread both skip ids that fail
// GetMembership — and the due-date scanner walks the same assignee rows.
//
// Cutting their sockets is not the fix: the notification row survives and
// GET /notifications filters by user_id alone, and routing rules forward the
// same text to e-mail/Telegram, which no socket revocation touches.
func TestRemovedMemberStopsHearingAboutTasks(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	leaving := signup(t)
	staying := signup(t)
	for _, u := range []*client{leaving, staying} {
		owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
			map[string]any{"email": u.Email, "role": "member"}), http.StatusCreated)
	}

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Смета на подрядчика")
	taskID := task["id"].(string)
	for _, u := range []*client{leaving, staying} {
		owner.expect(t, owner.post("/tasks/"+taskID+"/assignees",
			map[string]any{"user_id": u.UserID}), http.StatusNoContent)
	}

	owner.expect(t, owner.del("/workspaces/"+s.WS+"/members/"+leaving.UserID), http.StatusNoContent)

	// Short on purpose: shortCtx inlines an excerpt of at most shortContextLimit
	// runes, and it is exactly that inlined excerpt which leaves the workspace.
	const secret = "сейф: 4417 2200"
	owner.expect(t, owner.post("/tasks/"+taskID+"/comments",
		map[string]any{"body": secret}), http.StatusCreated)

	t.Run("the ex-member hears nothing", func(t *testing.T) {
		for _, n := range leaving.get("/notifications").listBody(t) {
			if kind, _ := n["kind"].(string); kind != "comment" {
				continue
			}
			t.Fatalf("ex-member was notified about a workspace they were removed from: %v", n)
		}
	})

	t.Run("the comment body does not travel with it", func(t *testing.T) {
		if body := string(leaving.get("/notifications").Body); strings.Contains(body, secret) {
			t.Fatalf("comment body leaked to the ex-member: %s", body)
		}
	})

	t.Run("the member who stayed is still notified", func(t *testing.T) {
		for _, n := range staying.get("/notifications").listBody(t) {
			if kind, _ := n["kind"].(string); kind == "comment" {
				return
			}
		}
		t.Fatal("the remaining member lost their comment notification")
	})
}
