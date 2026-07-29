package rank

import (
	"testing"
	"time"

	"tessera-mcp/internal/model"
)

func ptime(t time.Time) *time.Time { return &t }

func TestTasksOrdering(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	archived := model.Task{ID: "archived", Priority: 4, ArchivedAt: ptime(yesterday)}
	done := model.Task{ID: "done", Priority: 4, CompletedAt: ptime(yesterday)}
	overdueLow := model.Task{ID: "overdue-low", Priority: 1, DueDate: ptime(yesterday)}
	urgentNoDue := model.Task{ID: "urgent", Priority: 4}
	normalSoon := model.Task{ID: "normal-soon", Priority: 2, DueDate: ptime(tomorrow)}
	noneNoDue := model.Task{ID: "none", Priority: 0}

	got := Tasks([]model.Task{noneNoDue, done, urgentNoDue, archived, normalSoon, overdueLow}, Options{Now: now})

	var ids []string
	for _, x := range got {
		ids = append(ids, x.ID)
	}

	// archived + completed dropped; overdue first, then priority desc, then due asc.
	want := []string{"overdue-low", "urgent", "normal-soon", "none"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("position %d: got %q, want %q (full: %v)", i, ids[i], want[i], ids)
		}
	}
}

func TestIncludeCompleted(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	done := model.Task{ID: "done", Priority: 4, CompletedAt: ptime(now.Add(-time.Hour))}
	open := model.Task{ID: "open", Priority: 1}

	got := Tasks([]model.Task{done, open}, Options{Now: now, IncludeCompleted: true})
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
	if got[0].ID != "open" || got[1].ID != "done" {
		t.Fatalf("completed task should rank last, got %q then %q", got[0].ID, got[1].ID)
	}
}
