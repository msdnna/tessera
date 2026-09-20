// A checklist item may carry a repeat rule of its own, and the duplicate branch
// has to hand it forward.
//
// The rule is written in the same DueEditor whether the modal was opened on a
// card or on one of its subtasks — cloneRecurringTask's own comment says so, and
// that is why a recurring *item* is rebuilt under the same parent rather than as
// a card. So "еженедельный отчёт" whose step «Свериться с бухгалтерией» repeats
// monthly is an ordinary plan, not an exotic one.
//
// The two branches disagreed about it for the same reason they once disagreed
// about the due-notify overrides and about the stage/quadrant pins: the
// reschedule branch edits each item in place (ReopenSubtasks,
// LiftSubtasksFromColumn and ShiftSubtaskWindow name no such column), so the
// item's rule survives; the create_new branch rebuilds the checklist with
// CreateTask, whose parameters carry no recurrence — the parent's own rule is
// put back afterwards by setTaskRecurrence, each item's is not. The duplicate's
// step was therefore born a one-off.
//
// The loss is silent and permanent: from the second occurrence on, the item no
// longer repeats, and the only copy still showing the rule is the spent
// occurrence sitting in «Готово». Nothing on the new card says a rule was
// dropped — the popover simply reads «Не повторять».
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceCarriesChecklistItemOwnRule(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ruleOf reads a task's stored repeat rule through the API the clients use.
	ruleOf := func(id string) map[string]any {
		t.Helper()
		detail := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		r, _ := detail["recurrence"].(map[string]any)
		return r
	}

	mkSub := func(parentID, title string) string {
		t.Helper()
		sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": todo, "parent_id": parentID,
			"due_date": rfc(due),
		}), http.StatusCreated)
		return sub["id"].(string)
	}

	// ── the create_new branch: the original stays, a duplicate carries on ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	stepID := mkSub(origID, "Свериться с бухгалтерией")
	// A second level: the duplicate walks the whole subtree, so an item's rule
	// goes the same way at any depth.
	deepID := mkSub(stepID, "Запросить выписку")
	// An item nobody set a rule on — the carry copies what was asked for, it does
	// not invent a routine.
	mkSub(origID, "Отправить руководителю")

	c.expect(t, c.patch("/tasks/"+stepID, map[string]any{
		"title": "Свериться с бухгалтерией", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "monthly", "interval": 1},
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+deepID, map[string]any{
		"title": "Запросить выписку", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 2},
	}), http.StatusOK)
	if r := ruleOf(stepID); r == nil || r["freq"] != "monthly" {
		t.Fatalf("fixture: the checklist item did not keep its own rule: %v", r)
	}

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	cloneStep := subtaskByTitle(t, clone, "Свериться с бухгалтерией")
	cloneStepID := cloneStep["id"].(string)
	got := ruleOf(cloneStepID)
	if got == nil {
		t.Fatalf("the duplicate's checklist item lost its own repeat rule")
	}
	if got["freq"] != "monthly" || got["interval"] != float64(1) {
		t.Fatalf("the duplicate's item got a different rule: %v", got)
	}

	cloneStepDetail := c.expect(t, c.get("/tasks/"+cloneStepID), http.StatusOK)
	cloneDeep := subtaskByTitle(t, cloneStepDetail, "Запросить выписку")
	deepRule := ruleOf(cloneDeep["id"].(string))
	if deepRule == nil || deepRule["freq"] != "daily" || deepRule["interval"] != float64(2) {
		t.Fatalf("the duplicate lost a nested item's repeat rule: %v", deepRule)
	}

	plain := subtaskByTitle(t, clone, "Отправить руководителю")
	if r := ruleOf(plain["id"].(string)); r != nil {
		t.Fatalf("the duplicate invented a repeat rule for an item that had none: %v", r)
	}

	// The spent occurrence keeps its own item's rule — the duplicate copied it,
	// it did not take it. (The parent's rule IS stripped from the original: that
	// is what makes it historical. An item is not the thing that recurred.)
	if r := ruleOf(stepID); r == nil || r["freq"] != "monthly" {
		t.Fatalf("the first occurrence's item lost its rule to the duplicate: %v", r)
	}

	// ── the reschedule branch: the control, green before the fix and after ──
	// It never rebuilt anything, so it is what the duplicate has to match.
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	dailyStepID := mkSub(dailyID, "Собрать метрики")
	c.expect(t, c.patch("/tasks/"+dailyStepID, map[string]any{
		"title": "Собрать метрики", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "monthly", "interval": 1},
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	if r := ruleOf(dailyStepID); r == nil || r["freq"] != "monthly" {
		t.Fatalf("a reschedule dropped the checklist item's own rule: %v", r)
	}
}
