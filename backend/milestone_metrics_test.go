package main

import (
	"net/http"
	"testing"
	"time"
)

// The stage's journal carries ids since #2992: the stage page counts its scope
// change from these entries, and a title is not a key (two stages can share one,
// and a rename would orphan every older line).
func TestMilestoneJournalCarriesIDs(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	first := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Этап 1"}), http.StatusCreated)["id"].(string)
	second := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Этап 2"}), http.StatusCreated)["id"].(string)
	id := mkTask(t, c, s.Board, s.col(t, 0), "Задача")["id"].(string)

	c.expect(t, c.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": first}), http.StatusNoContent)
	if d := lastTagEvent(t, c, id, "milestone"); d["milestone_id"] != first || d["from_id"] != nil {
		t.Fatalf("pin = %v, want milestone_id %s and no from_id", d, first)
	}
	c.expect(t, c.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": second}), http.StatusNoContent)
	d := lastTagEvent(t, c, id, "milestone")
	if d["milestone_id"] != second || d["from_id"] != first || d["from_title"] != "Этап 1" {
		t.Fatalf("move = %v, want %s from %s", d, second, first)
	}
	c.expect(t, c.del("/tasks/"+id+"/milestone"), http.StatusNoContent)
	if d := lastTagEvent(t, c, id, "milestone_removed"); d["milestone_id"] != second || d["title"] != "Этап 2" {
		t.Fatalf("unpin = %v, want the stage it left", d)
	}
}

// closed_at follows the state (#3027): closing stamps it, a later edit of the
// closed stage keeps the day, reopening clears it.
func TestMilestoneClosedAt(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	m := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Спринт"}), http.StatusCreated)
	if m["closed_at"] != nil {
		t.Fatalf("new stage closed_at = %v", m["closed_at"])
	}
	id := m["id"].(string)
	closed := c.expect(t, c.patch("/milestones/"+id, map[string]any{"title": "Спринт", "state": "closed"}), http.StatusOK)
	stamp, _ := closed["closed_at"].(string)
	if stamp == "" {
		t.Fatalf("closing did not stamp closed_at: %v", closed)
	}
	time.Sleep(10 * time.Millisecond)
	again := c.expect(t, c.patch("/milestones/"+id, map[string]any{"title": "Спринт 1", "state": "closed"}), http.StatusOK)
	if again["closed_at"] != stamp {
		t.Fatalf("editing a closed stage moved closed_at %v → %v", stamp, again["closed_at"])
	}
	open := c.expect(t, c.patch("/milestones/"+id, map[string]any{"title": "Спринт 1", "state": "active"}), http.StatusOK)
	if open["closed_at"] != nil {
		t.Fatalf("reopening kept closed_at %v", open["closed_at"])
	}
	born := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Старый", "state": "closed"}), http.StatusCreated)
	if born["closed_at"] == nil {
		t.Fatalf("a stage created closed has no closed_at")
	}
}

// The project's stage list carries the rollups the «Проект» window and the
// sidebar show (#2992), and the metrics endpoint answers the stage page.
func TestMilestoneRollupsAndMetrics(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	prev := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Спринт 41", "state": "closed"}), http.StatusCreated)["id"].(string)
	start := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02") + "T00:00:00Z"
	due := time.Now().UTC().AddDate(0, 0, 4).Format("2006-01-02") + "T00:00:00Z"
	ms := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Спринт 42", "start_date": start, "due_date": due}), http.StatusCreated)["id"].(string)

	old := mkTask(t, c, s.Board, s.col(t, 0), "Прошлый")["id"].(string)
	c.expect(t, c.post("/tasks/"+old+"/milestone", map[string]any{"milestone_id": prev}), http.StatusNoContent)

	done := s.Columns[len(s.Columns)-1]["id"].(string)
	for i, title := range []string{"Первая", "Вторая", "Третья"} {
		id := mkTask(t, c, s.Board, s.col(t, 0), title)["id"].(string)
		c.expect(t, c.post("/tasks/"+id+"/milestone", map[string]any{"milestone_id": ms}), http.StatusNoContent)
		if i == 0 {
			c.expect(t, c.patch("/tasks/"+id+"/move", map[string]any{"column_id": done}), http.StatusOK)
		}
	}
	c.expect(t, c.patch("/tasks/"+old+"/move", map[string]any{"column_id": done}), http.StatusOK)

	var row map[string]any
	for _, m := range c.get("/projects/" + s.Project + "/milestones").listBody(t) {
		if m["id"] == ms {
			row = m
		}
	}
	if row == nil || row["task_count"] != float64(3) || row["done_count"] != float64(1) {
		t.Fatalf("rollup = %v, want 3 tasks, 1 done", row)
	}

	mt := c.expect(t, c.get("/milestones/"+ms+"/metrics"), http.StatusOK)
	totals := mt["totals"].(map[string]any)
	if totals["tasks"] != float64(3) || totals["done"] != float64(1) {
		t.Fatalf("totals = %v", totals)
	}
	signals := mt["signals"].(map[string]any)
	if signals["no_assignee"] != float64(2) || signals["no_estimate"] != float64(2) {
		t.Fatalf("signals = %v", signals)
	}
	// All three were pinned today — after the start day, so they count as added.
	scope := mt["scope"].(map[string]any)
	if scope["added"] != float64(3) || scope["now"] != float64(3) {
		t.Fatalf("scope = %v", scope)
	}
	series := mt["series"].([]any)
	if len(series) != 4 {
		t.Fatalf("series has %d days, want 4 (start … today)", len(series))
	}
	lastPt := series[len(series)-1].(map[string]any)
	if lastPt["scope"] != float64(3) || lastPt["done"] != float64(1) {
		t.Fatalf("today's point = %v", lastPt)
	}
	pace := mt["pace"].([]any)
	if len(pace) != 2 || pace[0].(map[string]any)["id"] != prev || pace[0].(map[string]any)["done"] != float64(1) {
		t.Fatalf("pace = %v, want the closed stage first", pace)
	}
	if len(mt["tasks"].([]any)) != 3 || mt["milestone"].(map[string]any)["title"] != "Спринт 42" {
		t.Fatalf("tasks/milestone = %v / %v", mt["tasks"], mt["milestone"])
	}

	// A stranger cannot read it.
	other := signup(t)
	other.expect(t, other.get("/milestones/"+ms+"/metrics"), http.StatusForbidden)
}
