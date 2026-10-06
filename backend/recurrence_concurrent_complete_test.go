// Closing a repeating task is one gesture, and the board offers several ways to
// make it twice at once: the card's checkbox and the modal's, two people on the
// same board, a double tap on a phone whose first request is still in flight.
// The advance itself is slow — a duplicate rebuilds the whole checklist, copies
// every attachment's bytes and rewrites the description — so "still in flight"
// is measured in hundreds of milliseconds, not the few the user waits before
// tapping again.
//
// Nothing between the read and the advance is atomic: applyTaskPatch decides to
// advance by comparing the task it read at the start of the request against the
// row it just wrote, and applyMove does the same for the done column. Two
// requests that both read the open task therefore both advance it.
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

// tickBox sends PATCH /tasks/<id> {"completed":true} without touching
// *testing.T — it runs on a goroutine of its own, where t.Fatalf is not allowed.
func tickBox(token, taskID string) {
	body, err := json.Marshal(map[string]any{"completed": true})
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPatch, testServer.URL+"/api/tasks/"+taskID, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_ = res.Body.Close()
}

// tickTwice fires the same completion from several goroutines released at once.
func tickTwice(token, taskID string, n int) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tickBox(token, taskID)
		}()
	}
	close(start)
	wg.Wait()
}

// Two simultaneous ticks of the same box must leave one next occurrence, not
// two: the series moved on by one step because it was closed once.
func TestConcurrentCompletionAdvancesTheSeriesOnce(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }
	dueOf := func(m map[string]any) time.Time {
		t.Helper()
		got, err := time.Parse(time.RFC3339, m["due_date"].(string))
		if err != nil {
			t.Fatalf("due date %v: %v", m["due_date"], err)
		}
		return got.UTC()
	}

	// ── the reschedule branch ───────────────────────────────────────
	//
	// The due date is the series' own position, so a second advance does not
	// duplicate anything — it silently skips an occurrence. A weekly report
	// closed twice at once comes back due a fortnight out, and the week in
	// between is simply not planned.
	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Разбор почты", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	taskID := task["id"].(string)
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{
		"recurrence": map[string]any{
			"freq": "weekly", "interval": 1, "trigger": "complete", "target_column": todo,
		},
	}), http.StatusOK)

	// A checklist item, because the parent's own due date hides the second
	// advance here — both requests step it from the same date and land on the
	// same answer. Each item's window is carried by a RELATIVE shift, so the
	// second advance moves it a second time: the card says next week and its
	// steps say the week after.
	item := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Разобрать спам", "column_id": todo,
		"parent_id": taskID, "due_date": rfc(due),
	}), http.StatusCreated)

	tickTwice(c.token, taskID, 2)

	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if after["completed_at"] != nil {
		t.Fatalf("следующее повторение родилось закрытым: %v", after["completed_at"])
	}
	// The card and its step must have taken the same number of periods. Not the
	// absolute date: a second request that reads the task only after the first
	// advance has landed is an ordinary second close, and how the two overlap is
	// the network's business. What no interleaving may produce is a card and a
	// checklist on different weeks — that is the double advance, which steps the
	// parent from a date it has already left while shifting every item again.
	step := c.expect(t, c.get("/tasks/"+item["id"].(string)), http.StatusOK)
	if got, want := dueOf(step), dueOf(after); !got.Equal(want) {
		t.Fatalf("срок пункта чек-листа %s, а у карточки %s — окно шагнуло лишний раз", got, want)
	}

	// ── the create_new branch ───────────────────────────────────────
	//
	// Here the second advance is visible: an extra card on the board, with the
	// same title, the same checklist and the same rule, which then repeats on a
	// series of its own for good.
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Дежурство по кухне", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)
	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"recurrence": map[string]any{
			"freq": "daily", "interval": 1, "create_new": true,
			"trigger": "complete", "target_column": todo,
		},
	}), http.StatusOK)

	tickTwice(c.token, origID, 2)

	clones := 0
	for _, m := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
		if m["title"] == "Дежурство по кухне" && m["id"] != origID {
			clones++
		}
	}
	if clones != 1 {
		t.Fatalf("две одновременные галочки породили копий: %d, нужна одна", clones)
	}
}
