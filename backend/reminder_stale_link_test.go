// A reminder that names a task keeps firing after the task walks out of reach —
// that is what TestReminderFiresAfterLosingTheTask pins: reminderScope falls
// back to the user's own workspace so the membership gate cannot swallow a
// message whose text is the user's own.
//
// The workspace is re-scoped; the task_id is not. The notification row is then
// written with a personal workspace_id and a foreign task_id, and every reader
// downstream follows that id without asking about membership: ListNotifications
// LEFT JOINs tasks and hands back task_number/task_board_id, deliverNotification
// enriches the live socket payload the same way, and templateData renders
// TaskTitle into whatever leaves the app over e-mail or telegram. The title it
// renders is the one the task carries *now* — edited after the reader was
// removed — so the fallback that exists to keep a private reminder alive also
// carries a stranger's current data out with it.
//
// The link is stale by the same test that re-scoped the workspace. It should be
// dropped there, not followed.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tessera/handlers"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// TestReminderDropsTheLinkItMayNoLongerFollow fires two reminders on the same
// task in one scanner tick: one for a member who was removed in the meantime,
// one for a member who stayed. The removed member still hears their reminder,
// but without the task link; the member who stayed keeps it.
func TestReminderDropsTheLinkItMayNoLongerFollow(t *testing.T) {
	ctx := context.Background()
	owner := signup(t)
	s := mkStack(t, owner)
	removed := signup(t)
	stays := signup(t)
	for _, m := range []*client{removed, stays} {
		owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
			map[string]any{"email": m.Email, "role": "member"}), http.StatusCreated)
	}

	task := mkTask(t, owner, s.Board, s.col(t, 0), "сдать отчёт")
	taskID := task["id"].(string)

	// Hold the scanner lock so the harness's own worker cannot deliver either
	// reminder before the test drives its tick.
	release := holdScannerLock(t)
	for _, m := range []*client{removed, stays} {
		m.expect(t, m.post("/reminders", map[string]any{
			"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
			"message":   "позвонить бухгалтеру про отчёт",
			"task_id":   taskID,
		}), http.StatusCreated)
	}

	// The ordinary offboarding move, and then the task moves on without them.
	owner.expect(t, owner.del("/workspaces/"+s.WS+"/members/"+removed.UserID), http.StatusNoContent)
	owner.expect(t, owner.patch("/tasks/"+taskID, map[string]any{
		"title": "перевести аванс подрядчику на счёт 40817",
	}), http.StatusOK)

	release()
	api := handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
	api.ScanNotificationsOnce(ctx)

	gone := reminderFeedEntry(t, removed)
	if gone == nil {
		t.Fatalf("the removed member got no reminder at all — the fallback is gone")
	}
	if id := gone["task_id"]; id != nil {
		t.Fatalf("the reminder still names a task the reader cannot open: task_id=%v", id)
	}
	// The join hangs off task_id, so these fall away with it; asserted separately
	// because they are what actually reaches the reader's screen and inbox.
	if n := gone["task_number"]; n != nil {
		t.Fatalf("the feed carries the foreign task's number: %v", n)
	}
	if b := gone["task_board_id"]; b != nil {
		t.Fatalf("the feed carries the foreign task's board: %v", b)
	}

	// Control: the member who stayed is owed the link — dropping it for everyone
	// would pass every assertion above and break the feature.
	kept := reminderFeedEntry(t, stays)
	if kept == nil {
		t.Fatalf("the member who stayed got no reminder")
	}
	if kept["task_id"] != taskID {
		t.Fatalf("the member who stayed lost the task link: %v", kept["task_id"])
	}
	if kept["task_number"] == nil || kept["task_board_id"] == nil {
		t.Fatalf("the member who stayed lost the task details: %v", kept)
	}
}

// TestReminderWithoutTaskStillFires is the other control: a reminder that never
// named a task is delivered on the personal-workspace fallback as before.
func TestReminderWithoutTaskStillFires(t *testing.T) {
	ctx := context.Background()
	c := signup(t)

	release := holdScannerLock(t)
	c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "забрать посылку",
	}), http.StatusCreated)

	release()
	api := handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
	api.ScanNotificationsOnce(ctx)

	e := reminderFeedEntry(t, c)
	if e == nil {
		t.Fatalf("a task-less reminder did not fire")
	}
	if e["task_id"] != nil {
		t.Fatalf("a task-less reminder grew a task link: %v", e["task_id"])
	}
}

// reminderFeedEntry returns the user's newest "reminder" notification as the feed
// renders it (nil if there is none).
func reminderFeedEntry(t *testing.T, c *client) map[string]any {
	t.Helper()
	for _, n := range c.get("/notifications").listBody(t) {
		if n["kind"] == "reminder" {
			return n
		}
	}
	return nil
}
