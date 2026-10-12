// A recurrence steps its parent's due date on the calendar of the rule's zone,
// so the rest of the occurrence — the start date, every checklist item — has to
// step on that same calendar. Measured as elapsed time instead, the day the
// clocks change is 23 or 25 hours long and the whole occurrence comes apart: the
// parent keeps its hour and everything under it moves an hour off it.
package main

import (
	"net/http"
	"testing"
	"time"
)

// Around a clock change the checklist and the start date must keep their clock,
// the same way the parent's due date does.
//
// Europe/Berlin loses an hour on 2026-03-29 (02:00 → 03:00), so a daily task due
// 09:00 on the 28th is due 09:00 on the 29th — one calendar day later, but only
// 23 hours of elapsed time. That 23 hours is then handed to the start date and to
// every checklist item as a plain interval, and an interval is not a day: it is
// right only for a row whose own window happens to straddle the transition too.
// An item that sits earlier in the day does not, so it is moved an hour BACK —
// and an item scheduled just after midnight lands on the previous date, which is
// where the board draws it.
//
// The zone is the one the editor writes into the rule (DueEditor sends the
// browser's), so this is every European and North American user, twice a year.
func TestRecurrenceKeepsClockAcrossDST(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load Europe/Berlin: %v", err)
	}
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }
	// The occurrence right before the clocks go forward.
	start := time.Date(2026, 3, 28, 8, 0, 0, 0, berlin)
	due := time.Date(2026, 3, 28, 9, 0, 0, 0, berlin)
	// What the next occurrence must read on the wall: the same clock, next day.
	wantStart := time.Date(2026, 3, 29, 8, 0, 0, 0, berlin)
	wantDue := time.Date(2026, 3, 29, 9, 0, 0, 0, berlin)
	// An item scheduled just after midnight is how this shows on the board: an
	// hour off here is a different calendar day.
	itemDue := time.Date(2026, 3, 28, 0, 30, 0, 0, berlin)
	wantItemDue := time.Date(2026, 3, 29, 0, 30, 0, 0, berlin)

	daily := map[string]any{"freq": "daily", "interval": 1, "tz": "Europe/Berlin"}

	// ── the reschedule branch: the task itself moves to the next occurrence ──
	t.Run("rescheduled occurrence keeps the clock", func(t *testing.T) {
		parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Ежедневный статус (DST)", "column_id": s.col(t, 0),
			"start_date": rfc(start), "due_date": rfc(due),
		}), http.StatusCreated)
		parentID := parent["id"].(string)

		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Собрать метрики (DST)", "column_id": s.col(t, 0), "parent_id": parentID,
			"start_date": rfc(start), "due_date": rfc(due),
		}), http.StatusCreated)
		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Ночная выгрузка (DST)", "column_id": s.col(t, 0), "parent_id": parentID,
			"due_date": rfc(itemDue),
		}), http.StatusCreated)

		advanced := c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
			"title": "Ежедневный статус (DST)", "start_date": rfc(start), "due_date": rfc(due),
			"recurrence": daily, "completed": true,
		}), http.StatusOK)

		// The parent's due date is the control: it already steps on the calendar.
		if got := parseTS(t, advanced["due_date"]); !got.Equal(wantDue) {
			t.Fatalf("parent due = %v, want %v", got.In(berlin), wantDue)
		}
		if advanced["start_date"] == nil {
			t.Fatalf("recurrence dropped the parent's start_date: %v", advanced)
		}
		if got := parseTS(t, advanced["start_date"]); !got.Equal(wantStart) {
			t.Fatalf("parent start = %v, want %v — the clocks went forward, the working window did not move",
				got.In(berlin), wantStart)
		}

		detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
		item := subtaskByTitle(t, detail, "Собрать метрики (DST)")
		if got := parseTS(t, item["due_date"]); !got.Equal(wantDue) {
			t.Fatalf("checklist item due = %v, want %v — it sat on the parent's hour and must stay there",
				got.In(berlin), wantDue)
		}
		if got := parseTS(t, item["start_date"]); !got.Equal(wantStart) {
			t.Fatalf("checklist item start = %v, want %v", got.In(berlin), wantStart)
		}
		night := subtaskByTitle(t, detail, "Ночная выгрузка (DST)")
		got := parseTS(t, night["due_date"]).In(berlin)
		if !got.Equal(wantItemDue) {
			t.Fatalf("after-midnight item due = %v, want %v — an hour back is the previous day on the board",
				got, wantItemDue)
		}
		if got.Day() != wantItemDue.Day() {
			t.Fatalf("after-midnight item landed on %v, the board draws it a day early", got)
		}
	})

	// ── the create_new branch: the duplicate is the next occurrence ──
	t.Run("duplicated occurrence keeps the clock", func(t *testing.T) {
		orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Ежедневная сводка (DST)", "column_id": s.col(t, 0),
			"start_date": rfc(start), "due_date": rfc(due),
		}), http.StatusCreated)
		origID := orig["id"].(string)

		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Свести цифры (DST)", "column_id": s.col(t, 0), "parent_id": origID,
			"start_date": rfc(start), "due_date": rfc(due),
		}), http.StatusCreated)
		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Ночной прогон (DST)", "column_id": s.col(t, 0), "parent_id": origID,
			"due_date": rfc(itemDue),
		}), http.StatusCreated)

		clone := map[string]any{"freq": "daily", "interval": 1, "tz": "Europe/Berlin", "create_new": true}
		c.expect(t, c.patch("/tasks/"+origID, map[string]any{
			"title": "Ежедневная сводка (DST)", "start_date": rfc(start), "due_date": rfc(due),
			"recurrence": clone, "completed": true,
		}), http.StatusOK)

		dup := findClone(t, c, s.Board, origID, "Ежедневная сводка (DST)")
		if got := parseTS(t, dup["due_date"]); !got.Equal(wantDue) {
			t.Fatalf("duplicate due = %v, want %v", got.In(berlin), wantDue)
		}
		if got := parseTS(t, dup["start_date"]); !got.Equal(wantStart) {
			t.Fatalf("duplicate start = %v, want %v", got.In(berlin), wantStart)
		}
		item := subtaskByTitle(t, dup, "Свести цифры (DST)")
		if got := parseTS(t, item["due_date"]); !got.Equal(wantDue) {
			t.Fatalf("duplicated checklist item due = %v, want %v", got.In(berlin), wantDue)
		}
		if got := parseTS(t, item["start_date"]); !got.Equal(wantStart) {
			t.Fatalf("duplicated checklist item start = %v, want %v", got.In(berlin), wantStart)
		}
		night := subtaskByTitle(t, dup, "Ночной прогон (DST)")
		got := parseTS(t, night["due_date"]).In(berlin)
		if !got.Equal(wantItemDue) {
			t.Fatalf("duplicated after-midnight item due = %v, want %v — the duplicate is the next occurrence, on the next date",
				got, wantItemDue)
		}
	})

	// ── the same seam in the other direction: the 25-hour autumn day ──
	t.Run("the autumn long day keeps the clock", func(t *testing.T) {
		// Berlin gains an hour on 2026-10-25 (03:00 → 02:00): the elapsed step is
		// 25 hours, so an early-morning item is pushed an hour FORWARD instead.
		aStart := time.Date(2026, 10, 24, 8, 0, 0, 0, berlin)
		aDue := time.Date(2026, 10, 24, 9, 0, 0, 0, berlin)
		aItem := time.Date(2026, 10, 24, 1, 15, 0, 0, berlin)
		wantAItem := time.Date(2026, 10, 25, 1, 15, 0, 0, berlin)

		parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Осенний статус (DST)", "column_id": s.col(t, 0),
			"start_date": rfc(aStart), "due_date": rfc(aDue),
		}), http.StatusCreated)
		parentID := parent["id"].(string)
		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Ранний пункт (DST)", "column_id": s.col(t, 0), "parent_id": parentID,
			"due_date": rfc(aItem),
		}), http.StatusCreated)

		c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
			"title": "Осенний статус (DST)", "start_date": rfc(aStart), "due_date": rfc(aDue),
			"recurrence": daily, "completed": true,
		}), http.StatusOK)

		detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
		item := subtaskByTitle(t, detail, "Ранний пункт (DST)")
		if got := parseTS(t, item["due_date"]).In(berlin); !got.Equal(wantAItem) {
			t.Fatalf("early item due = %v, want %v — the long day pushed it an hour forward", got, wantAItem)
		}
	})

	// The control that must stay green either way: a zone with no clock change at
	// all steps by a plain 24 hours, and nothing above may disturb that.
	t.Run("a zone without DST is untouched", func(t *testing.T) {
		msk, lerr := time.LoadLocation("Europe/Moscow")
		if lerr != nil {
			t.Fatalf("load Europe/Moscow: %v", lerr)
		}
		mStart := time.Date(2026, 3, 28, 8, 0, 0, 0, msk)
		mDue := time.Date(2026, 3, 28, 9, 0, 0, 0, msk)
		parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Московская летучка (DST)", "column_id": s.col(t, 0),
			"start_date": rfc(mStart), "due_date": rfc(mDue),
		}), http.StatusCreated)
		parentID := parent["id"].(string)
		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Пункт без перевода стрелок", "column_id": s.col(t, 0), "parent_id": parentID,
			"start_date": rfc(mStart), "due_date": rfc(mDue),
		}), http.StatusCreated)

		advanced := c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
			"title": "Московская летучка (DST)", "start_date": rfc(mStart), "due_date": rfc(mDue),
			"recurrence": map[string]any{"freq": "daily", "interval": 1, "tz": "Europe/Moscow"},
			"completed":  true,
		}), http.StatusOK)
		if got, want := parseTS(t, advanced["due_date"]), mDue.Add(24*time.Hour); !got.Equal(want) {
			t.Fatalf("Moscow parent due = %v, want %v", got.In(msk), want)
		}
		detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
		item := subtaskByTitle(t, detail, "Пункт без перевода стрелок")
		if got, want := parseTS(t, item["due_date"]), mDue.Add(24*time.Hour); !got.Equal(want) {
			t.Fatalf("Moscow item due = %v, want %v", got.In(msk), want)
		}
		if got, want := parseTS(t, item["start_date"]), mStart.Add(24*time.Hour); !got.Equal(want) {
			t.Fatalf("Moscow item start = %v, want %v", got.In(msk), want)
		}
	})
}
