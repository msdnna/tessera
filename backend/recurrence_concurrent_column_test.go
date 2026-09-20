// The column trigger is the third way to advance a repeating task, and it is the
// one the guard added for the other two never reached.
//
// Closing a task twice at once is now refused from the inside — applyTaskPatch
// arms the optimistic lock on the checkbox, applyMove arms it on the done
// column's auto-complete — but the column trigger sits below both, in an arm of
// its own that only asks «did this move change the column, and does the rule name
// it». Two drags of the same card into the trigger column both read it in the
// source column, so both answer yes, and the advance runs twice.
//
// A drag is the easiest of the three gestures to make twice: the board is a
// shared screen, the card snaps back until the request returns, and the advance
// is slow (a duplicate rebuilds the whole checklist, copies every attachment's
// bytes and rewrites the description), so the window is hundreds of milliseconds.
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

// dragTo sends PATCH /tasks/<id>/move without touching *testing.T — it runs on a
// goroutine of its own, where t.Fatalf is not allowed.
func dragTo(token, taskID, columnID string) {
	body, err := json.Marshal(map[string]any{"column_id": columnID})
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPatch, testServer.URL+"/api/tasks/"+taskID+"/move", bytes.NewReader(body))
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

// dragTwice fires the same move from several goroutines released at once.
func dragTwice(token, taskID, columnID string, n int) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			dragTo(token, taskID, columnID)
		}()
	}
	close(start)
	wg.Wait()
}

// Two simultaneous drags into the trigger column must advance the series once.
func TestConcurrentColumnTriggerAdvancesTheSeriesOnce(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, doing, review := s.col(t, 0), s.col(t, 1), s.col(t, 2)
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

	// The two branches are separate code and fail differently, so they are
	// separate subtests: one aborting must not leave the other unreported.

	// The second advance is a second card on the board: same title, same
	// checklist, same rule — and from then on it repeats on a series of its own.
	t.Run("копия рождается одна", func(t *testing.T) {
		orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Выдача пропусков", "column_id": todo, "due_date": rfc(due),
		}), http.StatusCreated)
		origID := orig["id"].(string)
		c.expect(t, c.patch("/tasks/"+origID, map[string]any{
			"recurrence": map[string]any{
				"freq": "weekly", "interval": 1, "create_new": true,
				"trigger": "column", "trigger_column": review, "target_column": doing,
			},
		}), http.StatusOK)

		dragTwice(c.token, origID, review, 2)

		clones := 0
		for _, m := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
			if m["title"] == "Выдача пропусков" && m["id"] != origID {
				clones++
			}
		}
		if clones != 1 {
			t.Fatalf("два одновременных переноса в колонку-триггер породили копий: %d, нужна одна", clones)
		}
	})

	// Here the card's own due date hides the second advance — both requests step
	// it from the same date and land on the same answer. Each checklist item's
	// window is carried by a RELATIVE shift, so the second advance moves it a
	// second time: the card says tomorrow and its steps say the day after.
	t.Run("окно чек-листа шагает один раз", func(t *testing.T) {
		task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Обход этажей", "column_id": todo, "due_date": rfc(due),
		}), http.StatusCreated)
		taskID := task["id"].(string)
		item := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Проверить лестницы", "column_id": todo,
			"parent_id": taskID, "due_date": rfc(due),
		}), http.StatusCreated)
		c.expect(t, c.patch("/tasks/"+taskID, map[string]any{
			"recurrence": map[string]any{
				"freq": "daily", "interval": 1,
				"trigger": "column", "trigger_column": review, "target_column": doing,
			},
		}), http.StatusOK)

		dragTwice(c.token, taskID, review, 2)

		after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
		step := c.expect(t, c.get("/tasks/"+item["id"].(string)), http.StatusOK)
		if got, want := dueOf(step), dueOf(after); !got.Equal(want) {
			t.Fatalf("срок пункта чек-листа %s, а у карточки %s — окно шагнуло лишний раз", got, want)
		}
		// The move itself is what the person asked for twice, and twice is once:
		// the card belongs where the rule put the next occurrence, not in the
		// trigger column a losing drag left it.
		if after["column_id"] != doing {
			t.Fatalf("карточка осталась в колонке %v, а правило переносит в %v", after["column_id"], doing)
		}
	})
}
