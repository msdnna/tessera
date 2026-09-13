package main

import (
	"net/http"
	"testing"
)

// A move names the target column in two places the reader sees — the task's
// journal and the "task moved" notification. Both carry the column's name_key
// beside the stored name (#2800), because the four columns seeded with a board
// are server-born Russian strings that every client captions from the key. Drop
// the key and an English UI reads «Готово» on the very row whose board header
// says "Done".
//
// The other half of the deal is what makes the distinction mean anything: a
// column the user renamed loses its key (see UpdateColumn), so it must travel
// verbatim and be shown that way in every language.
func TestMoveJournalCarriesSeededColumnKey(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача")
	id := task["id"].(string)

	// Into a seeded column ("В процессе", index 1): the journal records the key.
	c.expect(t, c.patch("/tasks/"+id+"/move", map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)
	if data := lastMoveData(t, c, id); data["to"] != "В процессе" || data["to_key"] != "in_progress" {
		t.Fatalf("seeded move journal = %v, want the stored name and its key", data)
	}

	// Rename the third column: that drops its name_key, so the move after it has
	// nothing to caption from and the chosen name stands on its own.
	renamed := c.expect(t, c.patch("/columns/"+s.col(t, 2), map[string]any{"name": "Протестировано"}), http.StatusOK)
	if renamed["name_key"] != nil {
		t.Fatalf("rename kept name_key = %v", renamed["name_key"])
	}
	c.expect(t, c.patch("/tasks/"+id+"/move", map[string]any{"column_id": s.col(t, 2)}), http.StatusOK)
	data := lastMoveData(t, c, id)
	if data["to"] != "Протестировано" {
		t.Fatalf("renamed move journal = %v, want the chosen name", data)
	}
	if _, ok := data["to_key"]; ok {
		t.Fatalf("a user-named column must carry no key, got %v", data["to_key"])
	}
}

// lastMoveData returns the data payload of the newest "moved" journal entry.
func lastMoveData(t *testing.T, c *client, taskID string) map[string]any {
	t.Helper()
	events := c.get("/tasks/" + taskID + "/events").listBody(t)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i]["kind"] != "moved" {
			continue
		}
		data, _ := events[i]["data"].(map[string]any)
		if data == nil {
			t.Fatalf("moved event has no data: %v", events[i])
		}
		return data
	}
	t.Fatalf("no moved event on task %s: %v", taskID, events)
	return nil
}
