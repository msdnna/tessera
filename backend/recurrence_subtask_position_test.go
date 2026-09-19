// A recurrence written on a checklist item must not renumber the checklist.
//
// Card order and checklist order are two separate float8 ladders: a card is
// numbered among the top-level cards of its COLUMN, an item among its PARENT's
// children. Both recurrence branches relocate the task into the rule's target
// column (the board's first by default) and both used to take the number for it
// off the column's ladder — a number that means nothing in the list the item is
// actually ordered in.
package main

import (
	"net/http"
	"testing"
	"time"
)

// titles lists a task detail's checklist in the order the API hands it back.
func subtaskTitles(t *testing.T, detail map[string]any) []string {
	t.Helper()
	subs, _ := detail["subtasks"].([]any)
	out := make([]string, 0, len(subs))
	for _, s := range subs {
		m, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("malformed subtask entry: %v", s)
		}
		out = append(out, m["title"].(string))
	}
	return out
}

func sameOrder(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The reschedule branch leaves the parent's checklist in the order the user put
// it in.
//
// Ticking a recurring item advances it and moves it to the rule's target column,
// which is the point of the gesture. Its position, though, is the item's rank
// among its siblings — unanchoredPosition is the rule the rest of the server
// already follows for a subtask that merely changed column (quick actions'
// /close, the task modal's own move): keep the number, the parent's order is not
// what the gesture was about.
//
// Off the column's ladder it is the column tail instead, so the item leaves the
// middle of the checklist and reappears at the bottom — on every single
// occurrence, the order degrading a little more each week.
func TestRecurringSubtaskKeepsItsPlaceInTheChecklist(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, doing := s.col(t, 0), s.col(t, 1)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)

	// Cards standing in the target column: they are what pushes that column's tail
	// above the checklist's own numbers, and every real board has some.
	for _, title := range []string{"Чужая карточка A", "Чужая карточка B", "Чужая карточка C"} {
		c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": todo,
		}), http.StatusCreated)
	}

	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": doing,
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	order := []string{"Собрать метрики", "Свести цифры", "Разослать отчёт"}
	ids := map[string]string{}
	for _, title := range order {
		sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": doing, "parent_id": parentID,
		}), http.StatusCreated)
		ids[title] = sub["id"].(string)
	}

	const recurring = "Свести цифры"
	advanced := c.expect(t, c.patch("/tasks/"+ids[recurring], map[string]any{
		"title": recurring, "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	// The move itself is the feature — assert it happened, so "keep the order"
	// can never be satisfied by quietly not moving the item at all.
	if advanced["column_id"] != todo {
		t.Fatalf("fixture: the recurring item did not reach the target column: %v", advanced["column_id"])
	}
	if got, want := parseTS(t, advanced["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("fixture: due = %v, want %v", got, want)
	}

	detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	if got := subtaskTitles(t, detail); !sameOrder(got, order) {
		t.Fatalf("recurrence reordered the checklist: %v, want %v", got, order)
	}
}

// The create_new branch appends the duplicate to the checklist instead of
// landing it on top of a live item.
//
// The duplicate is a new subtask, so its number belongs at the end of the
// parent's list — CreateTask's rule, the one the GitLab importer already
// follows for an imported child. Taken off the column's ladder it is
// "the column's last card + one gap", and on the ordinary layout below (the
// parent is the column's only card) that arithmetic hits an existing item's
// position exactly: two children with one number and no order of their own,
// the dead slot positionExhausted and rejectSameNeighbour exist to prevent.
func TestRecurringSubtaskCloneIsAppendedToTheChecklist(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)

	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo,
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	order := []string{"Собрать метрики", "Свести цифры", "Разослать отчёт"}
	ids := map[string]string{}
	for _, title := range order {
		sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": todo, "parent_id": parentID,
		}), http.StatusCreated)
		ids[title] = sub["id"].(string)
	}

	const recurring = "Свести цифры"
	c.expect(t, c.patch("/tasks/"+ids[recurring], map[string]any{
		"title": recurring, "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	got := subtaskTitles(t, detail)
	want := append(append([]string{}, order...), recurring)
	if !sameOrder(got, want) {
		t.Fatalf("the next occurrence landed inside the checklist: %v, want %v", got, want)
	}
}

// A recurring *card* still appends to the end of its column: the parentless case
// reads the same ladder it always did, and this is what says so.
func TestRecurringCardCloneStillAppendsToTheColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)

	const title = "Ежедневный статус"
	root := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": title, "column_id": todo, "due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	rootID := root["id"].(string)

	tail := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Чужая карточка", "column_id": todo,
	}), http.StatusCreated)

	c.expect(t, c.patch("/tasks/"+rootID, map[string]any{
		"title": title, "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, rootID, title)
	if clone["parent_id"] != nil {
		t.Fatalf("the duplicate of a root task grew a parent: %v", clone["parent_id"])
	}
	if clone["position"].(float64) <= tail["position"].(float64) {
		t.Fatalf("the duplicate did not land at the end of the column: %v, last card %v",
			clone["position"], tail["position"])
	}
}
