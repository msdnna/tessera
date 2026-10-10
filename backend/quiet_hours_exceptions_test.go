// «Только срочное и упоминания» (#3050): inside the quiet window the mentions
// of the reader and anything about an urgent task still go out to the external
// channels; the rest waits for the window to end, as it always did.
package main

import (
	"net/http"
	"testing"
	"time"
)

// putAllowUrgent switches the exception, leaving the rest of the prefs as the
// quiet-hours helpers set them.
func putAllowUrgent(t *testing.T, c *client, on bool) {
	t.Helper()
	now := time.Now().UTC()
	start := now.Add(-5 * time.Minute)
	end := now.Add(8 * time.Hour)
	got := c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
		"reminder_enabled": true, "quiet_enabled": true,
		"quiet_start_minutes": start.Hour()*60 + start.Minute(),
		"quiet_end_minutes":   end.Hour()*60 + end.Minute(),
		"quiet_tz":            "UTC", "digest_minutes": 0, "quiet_allow_urgent": on,
	}), http.StatusOK)
	if got["quiet_allow_urgent"] != on {
		t.Fatalf("quiet_allow_urgent = %v after saving %v", got["quiet_allow_urgent"], on)
	}
}

func TestQuietHoursLetMentionsAndUrgentTasksThrough(t *testing.T) {
	actor, member, s := aboutMeFixture(t)
	chID := mkWebhookChannelTmpl(t, member, "Хук на ночь", brokenHook(t), "")
	mkCatchAllRoute(t, member, chID)
	putAllowUrgent(t, member, true)

	// Ten minutes tell "sent now (and retried a minute later on the broken
	// hook)" from "held till the window ends", eight hours away.
	soon := func(kind string) bool {
		_, next := deliveryPlan(t, member.UserID, kind)
		return time.Until(next) < 10*time.Minute
	}

	plain := mkTask(t, actor, s.Board, s.col(t, 0), "Обычная задача")["id"].(string)
	assign(t, actor, plain, member.UserID)
	if soon("assigned") {
		t.Fatal("an ordinary assignment went out inside the quiet window")
	}

	actor.expect(t, actor.post("/tasks/"+plain+"/comments", map[string]any{
		"body": "@" + member.Email + " глянь", "mentions": []string{member.UserID},
	}), http.StatusCreated)
	if !soon("mention") {
		t.Fatal("a mention was held by the quiet window although the exception is on")
	}

	urgent := mkTask(t, actor, s.Board, s.col(t, 0), "Пожар")["id"].(string)
	actor.expect(t, actor.patch("/tasks/"+urgent, map[string]any{"priority": 4}), http.StatusOK)
	assign(t, actor, urgent, member.UserID)
	if !soon("assigned") {
		t.Fatal("an urgent task's assignment was held by the quiet window")
	}

	t.Run("a client that does not know the switch keeps it", func(t *testing.T) {
		member.expect(t, member.put("/notification-prefs", map[string]any{
			"due_enabled": true, "due_lead_minutes": 60, "reminder_enabled": true,
			"quiet_enabled": true, "quiet_start_minutes": 0, "quiet_end_minutes": 0, "quiet_tz": "UTC",
		}), http.StatusOK)
		if got := member.expect(t, member.get("/notification-prefs"), http.StatusOK); got["quiet_allow_urgent"] != true {
			t.Fatalf("quiet_allow_urgent = %v after a PUT without it", got["quiet_allow_urgent"])
		}
	})
}

func TestQuietHoursHoldMentionsWithoutTheException(t *testing.T) {
	actor, member, s := aboutMeFixture(t)
	chID := mkWebhookChannelTmpl(t, member, "Хук на ночь", brokenHook(t), "")
	mkCatchAllRoute(t, member, chID)
	putAllowUrgent(t, member, false)

	task := mkTask(t, actor, s.Board, s.col(t, 0), "Задача")["id"].(string)
	actor.expect(t, actor.post("/tasks/"+task+"/comments", map[string]any{
		"body": "глянь", "mentions": []string{member.UserID},
	}), http.StatusCreated)
	if _, next := deliveryPlan(t, member.UserID, "mention"); time.Until(next) < time.Hour {
		t.Fatalf("a mention went out at %s inside the quiet window with the exception off", next.UTC().Format("15:04"))
	}
}
