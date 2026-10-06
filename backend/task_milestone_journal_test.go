package main

import (
	"net/http"
	"testing"
)

// Pinning a task to a milestone belongs in the task's journal (#2932).
//
// The milestone was the last field of a task that changed without a word in
// «История». Everything else leaves a line — a rename, a description, a
// priority, a due or start date, an estimate, a completion, a column (`moved`),
// a tag, an assignee, a relation, an attachment, an archive — while
// applyMilestone wrote milestone_id, queued the GitLab write-back, broadcast
// task.updated and stopped there.
//
// That is the one field a whole screen is computed from: «Этапы»
// (ListWorkspaceMilestones) rolls task_count / done_count / Σ estimate up per
// milestone, so dragging one task from one stage to another moves the progress
// of both — and nothing anywhere recorded who did it or when. Through the
// /milestone quick action it read even worse: the history showed «выполнил
// команды» and not what the command had done.
//
// The entry records the milestone by TITLE, like the tag on tagged/untagged:
// tasks.milestone_id is ON DELETE SET NULL, so an id would dangle the moment the
// stage is deleted — which is exactly when the old history matters.
func TestMilestoneChangeIsJournalled(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	first := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Этап 1"}), http.StatusCreated)["id"].(string)
	second := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Этап 2"}), http.StatusCreated)["id"].(string)

	id := mkTask(t, c, s.Board, s.col(t, 0), "Задача этапа")["id"].(string)

	c.expect(t, c.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": first}), http.StatusNoContent)
	if data := lastTagEvent(t, c, id, "milestone"); data["title"] != "Этап 1" {
		t.Fatalf("pin journal = %v, want the milestone's title", data)
	}

	// Re-pinning the milestone the task already carries changes nothing, so it
	// must not add a second line — the rule tagged/untagged gets from its row
	// count. Both clients send the whole picker value on every save, so this is
	// the common case, not a corner one.
	c.expect(t, c.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": first}), http.StatusNoContent)
	if n := countEvents(t, c, id, "milestone"); n != 1 {
		t.Fatalf("milestone entries after a repeated pin = %d, want 1", n)
	}

	// Moving between stages is one entry naming where the task landed: that is
	// the move the «Этапы» screen shows as two progress bars changing at once.
	c.expect(t, c.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": second}), http.StatusNoContent)
	if data := lastTagEvent(t, c, id, "milestone"); data["title"] != "Этап 2" {
		t.Fatalf("re-pin journal = %v, want the new milestone's title", data)
	}
	if n := countEvents(t, c, id, "milestone"); n != 2 {
		t.Fatalf("milestone entries after a move = %d, want 2", n)
	}

	// Unpinning names the stage the task is leaving — the reader has no other way
	// back to it once the pointer is gone.
	c.expect(t, c.del("/tasks/"+id+"/milestone"), http.StatusNoContent)
	if data := lastTagEvent(t, c, id, "milestone_removed"); data["title"] != "Этап 2" {
		t.Fatalf("unpin journal = %v, want the title of the milestone it left", data)
	}

	// Clearing a milestone the task does not carry writes nothing: the DELETE is
	// idempotent and answers 204 either way.
	c.expect(t, c.del("/tasks/"+id+"/milestone"), http.StatusNoContent)
	if n := countEvents(t, c, id, "milestone_removed"); n != 1 {
		t.Fatalf("milestone_removed entries after a repeated clear = %d, want 1", n)
	}
}
