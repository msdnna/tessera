// A reminder may name a task, and the task_id arrives in the *body* — the place
// this project already pins with TestTagMustBeInSameWorkspace: "loadTask vouches
// for the task in the route, not for the id in the body". POST /reminders never
// looked at it at all.
//
// Nothing of the stranger's task is read back, so this is not a leak; it is the
// reference itself that must not be creatable. The symptom sits one step later:
// the scanner scopes a reminder's notification to the linked task's workspace,
// and deliverNotification drops a notification into a workspace the recipient is
// not a member of. So a reminder pointing at a task the user cannot see is
// swallowed in silence — and the same silence falls on a *legitimate* reminder
// the day its task moves out of reach (the project is transferred, the member is
// removed), even though the text being suppressed is the user's own.
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

// TestReminderTaskMustBeVisible refuses a reminder pointed at a stranger's task.
func TestReminderTaskMustBeVisible(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	vs := mkStack(t, victim)
	secret := mkTask(t, victim, vs.Board, vs.col(t, 0), "перевести деньги подрядчику")
	secretID := secret["id"].(string)

	attacker := signup(t) // member of nothing the victim owns

	r := attacker.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		"message":   "не моя задача",
		"task_id":   secretID,
	})
	if r.Status != http.StatusForbidden {
		t.Fatalf("reminder on a foreign task: got %d %s, want 403", r.Status, r.Body)
	}

	// A missing task is a 404, not a silent pass: the id is checked, not ignored.
	gone := attacker.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		"message":   "нет такой задачи",
		"task_id":   "00000000-0000-0000-0000-000000000000",
	})
	if gone.Status != http.StatusNotFound {
		t.Fatalf("reminder on a missing task: got %d %s, want 404", gone.Status, gone.Body)
	}

	// The control: the same call against the caller's own task still works.
	as := mkStack(t, attacker)
	mine := mkTask(t, attacker, as.Board, as.col(t, 0), "своя задача")
	attacker.expect(t, attacker.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		"message":   "своё напоминание",
		"task_id":   mine["id"].(string),
	}), http.StatusCreated)
}

// TestReminderFiresAfterLosingTheTask keeps a personal reminder working when its
// task walks away — here by the owner removing the member from the workspace.
// The reminder's text belongs to the user; losing sight of the task it names is
// not a reason to stop telling them about it.
func TestReminderFiresAfterLosingTheTask(t *testing.T) {
	ctx := context.Background()
	owner := signup(t)
	s := mkStack(t, owner)
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "сдать отчёт")
	taskID := task["id"].(string)

	// Hold the scanner lock so the harness's own worker cannot deliver the
	// reminder before the test drives its tick.
	release := holdScannerLock(t)
	member.expect(t, member.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "позвонить бухгалтеру про отчёт",
		"task_id":   taskID,
	}), http.StatusCreated)

	// Access goes away after the reminder was set — the ordinary offboarding move.
	owner.expect(t, owner.del("/workspaces/"+s.WS+"/members/"+member.UserID), http.StatusNoContent)

	release()
	api := handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
	api.ScanNotificationsOnce(ctx)
	if n := reminderNotifications(t, member.UserID); n != 1 {
		t.Fatalf("after losing access to the linked task: %d reminder notification(s), want 1", n)
	}
}
