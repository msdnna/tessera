// A routine's steps are as often written as checkboxes in the description as
// they are as subtasks, and the next occurrence has to hand them back unticked.
//
// The boxes in a description are not decoration: RichContent draws a real
// <input> for every GFM task item and a click rewrites the stored markdown
// through toggleTaskMarker. So «- [ ] полить цветы» in the description of a
// daily chore is the same promise the checklist makes — the cheaper way to write
// it, for a routine not worth four cards.
//
// Both branches of the recurrence carried the description verbatim: the
// reschedule one hands UpdateTask t.Description, the duplicate hands CreateTask
// src.Description. Everything else about being half-done is undone on purpose —
// completed_at is cleared, ReopenSubtasks unticks every item at every depth,
// LiftSubtasksFromColumn takes them out of «Готово» — and carryChecklistFrom's
// own comment names the bug it prevents: «the second run of the routine was born
// half-done». The boxes in the description were the one place it still was, from
// the second occurrence to the last, with no gesture available to reset them
// short of editing the markdown by hand every single time.
//
// The one thing the reset must not touch is a checklist that is being *quoted*:
// «- [x] …» inside a fence is a code sample, drawn as text with no box at all
// (toggleTaskMarker spells out the same trap from the other side — it must not
// flip a line no box was rendered for).
package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRecurrenceUnticksDescriptionCheckboxes(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// A live checklist, a box already ticked inside a blockquote (GFM draws one
	// there too), and the same list quoted as a code sample below it.
	body := strings.Join([]string{
		"Утренний обход:",
		"",
		"- [x] Полить цветы",
		"- [ ] Протереть пыль",
		"* [X] Вынести мусор",
		"",
		"> - [x] И то, что просили в цитате",
		"",
		"Записывается это так:",
		"",
		"```md",
		"- [x] шаг, который уже сделан",
		"```",
	}, "\n")

	descOf := func(id string) string {
		t.Helper()
		detail := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		d, _ := detail["description"].(string)
		return d
	}
	// wantFresh: every box the reader can click is empty again, and the sample
	// inside the fence is exactly as it was written.
	wantFresh := func(label, got string) {
		t.Helper()
		for _, line := range []string{
			"- [ ] Полить цветы",
			"- [ ] Протереть пыль",
			"* [ ] Вынести мусор",
			"> - [ ] И то, что просили в цитате",
		} {
			if !strings.Contains(got, line) {
				t.Fatalf("%s: the next occurrence was born with a ticked box — %q missing from:\n%s", label, line, got)
			}
		}
		if !strings.Contains(got, "```md\n- [x] шаг, который уже сделан\n```") {
			t.Fatalf("%s: the reset reached into a code sample:\n%s", label, got)
		}
		if !strings.Contains(got, "Утренний обход:") {
			t.Fatalf("%s: the description lost its prose:\n%s", label, got)
		}
	}

	// ── the duplicate branch ──────────────────────────────────────────────
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Обход офиса", "column_id": todo, "due_date": rfc(due),
		"description": body,
	}), http.StatusCreated)
	origID := orig["id"].(string)

	// An item of the checklist keeps its own description, boxes and all — the
	// carry has to reach it too, not just the card.
	step := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Проверить кухню", "column_id": todo, "parent_id": origID,
		"due_date": rfc(due), "description": body,
	}), http.StatusCreated)
	stepID := step["id"].(string)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Обход офиса", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Обход офиса")
	wantFresh("duplicate", clone["description"].(string))
	cloneStep := subtaskByTitle(t, clone, "Проверить кухню")
	wantFresh("duplicate's checklist item", descOf(cloneStep["id"].(string)))

	// The spent occurrence is the record of what was done — it keeps its ticks.
	if !strings.Contains(descOf(origID), "- [x] Полить цветы") {
		t.Fatalf("the finished occurrence lost the record of what was ticked:\n%s", descOf(origID))
	}
	if !strings.Contains(descOf(stepID), "- [x] Полить цветы") {
		t.Fatalf("the finished occurrence's item lost the record of what was ticked:\n%s", descOf(stepID))
	}

	// ── the reschedule branch ─────────────────────────────────────────────
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный обход", "column_id": todo, "due_date": rfc(due),
		"description": body,
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	dailyStep := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Проверить склад", "column_id": todo, "parent_id": dailyID,
		"due_date": rfc(due), "description": body,
	}), http.StatusCreated)
	dailyStepID := dailyStep["id"].(string)

	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный обход", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	wantFresh("reschedule", descOf(dailyID))
	wantFresh("reschedule's checklist item", descOf(dailyStepID))
}

// A description nobody wrote a checklist into must come back byte-for-byte: the
// reset is a checklist gesture, not a rewrite of everyone's markdown.
func TestRecurrenceLeavesPlainDescriptionAlone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	body := strings.Join([]string{
		"Созвон с подрядчиком. Повестка:",
		"",
		"1. Сроки",
		"2. Бюджет",
		"",
		"Ссылка: [регламент](/docs/1)",
	}, "\n")

	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Созвон", "column_id": todo, "due_date": rfc(due),
		"description": body,
	}), http.StatusCreated)
	id := task["id"].(string)

	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Созвон", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	detail := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
	if got, _ := detail["description"].(string); got != body {
		t.Fatalf("a description with no checkboxes came back changed:\nwant:\n%s\ngot:\n%s", body, got)
	}
}
