package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"tessera-mcp/internal/model"
)

// The estimate a task carries is the backend's canonical number, and the MCP
// must name it that way on both sides of the wire. These tests deliberately go
// through JSON rather than the Go field names: the defect being pinned is what
// the tool's *schema* promises an agent, and the Go identifier is only the way
// that promise is spelled.

// TestEstimateReportedAsCanonicalValue pins the read side: a summary and a task
// detail carry `estimate`, never an `estimate_hours` that would be sixty times
// the truth on a time-unit board.
func TestEstimateReportedAsCanonicalValue(t *testing.T) {
	est := 180.0 // three hours on a time board, stored as minutes
	task := model.Task{ID: "t1", Title: "T", Estimate: &est}

	raw, err := json.Marshal(summarize(task, "К работе", nil, "", time.Now()))
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if _, ok := got["estimate_hours"]; ok {
		t.Errorf("summary reports estimate_hours=%v; the stored 180 is minutes on a time board, not hours", got["estimate_hours"])
	}
	if got["estimate"] != 180.0 {
		t.Errorf("summary estimate = %v, want the canonical 180", got["estimate"])
	}
}

// TestUpdateTaskEstimateIsCanonical pins the write side of tessera_update_task:
// the number an agent passes reaches the API untouched under `estimate`, and the
// old hours-flavoured spelling is no longer a field at all (silently accepting it
// would keep writing three minutes where three hours were meant).
func TestUpdateTaskEstimateIsCanonical(t *testing.T) {
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": detail("t1", "b1")})
	ctx := context.Background()

	var in updateTaskInput
	if err := json.Unmarshal([]byte(`{"task_id":"t1","estimate":180}`), &in); err != nil {
		t.Fatalf("decode input: %v", err)
	}
	if _, _, err := updateTask(c)(ctx, nil, in); err != nil {
		t.Fatalf("updateTask: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &body); err != nil {
		t.Fatalf("no PATCH recorded: %v", err)
	}
	if body["estimate"] != 180.0 {
		t.Errorf("PATCH estimate = %v, want 180", body["estimate"])
	}

	var legacy updateTaskInput
	if err := json.Unmarshal([]byte(`{"task_id":"t1","estimate_hours":3}`), &legacy); err != nil {
		t.Fatalf("decode legacy input: %v", err)
	}
	if _, _, err := updateTask(c)(ctx, nil, legacy); err == nil {
		t.Error("estimate_hours still accepted; it would store three minutes for three hours")
	}
}

// TestCreateTaskEstimateIsCanonical is the same contract on the create path,
// which create_subtasks shares through buildTaskSpec.
func TestCreateTaskEstimateIsCanonical(t *testing.T) {
	c, mux := newMux(t, map[string]any{
		"/api/boards/b1":         model.Board{ID: "b1", ProjectID: "p1"},
		"/api/boards/b1/columns": []model.Column{{ID: "c1", Name: "К работе", Position: 1}},
		"/api/projects/p1":       model.Project{ID: "p1", WorkspaceID: "w1", Slug: "tessera"},
	})

	var in createTaskInput
	if err := json.Unmarshal([]byte(`{"board_id":"b1","title":"T","estimate":180}`), &in); err != nil {
		t.Fatalf("decode input: %v", err)
	}
	if _, _, err := createTask(c)(context.Background(), nil, in); err != nil {
		t.Fatalf("createTask: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(mux.writes["POST /api/boards/b1/tasks"], &body); err != nil {
		t.Fatalf("no create recorded: %v", err)
	}
	if body["estimate"] != 180.0 {
		t.Errorf("create estimate = %v, want 180", body["estimate"])
	}
}

// TestClearEstimateSpelling pins what `clear` accepts: the canonical field name,
// not the hours-flavoured one the tool used to advertise.
func TestClearEstimateSpelling(t *testing.T) {
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": detail("t1", "b1")})
	ctx := context.Background()

	if _, _, err := updateTask(c)(ctx, nil, updateTaskInput{TaskID: "t1", Clear: []string{"estimate"}}); err != nil {
		t.Fatalf("clear estimate: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &body); err != nil {
		t.Fatalf("no PATCH recorded: %v", err)
	}
	if v, ok := body["estimate"]; !ok || v != nil {
		t.Errorf("PATCH estimate = %v (present=%v), want an explicit null", v, ok)
	}
	if _, _, err := updateTask(c)(ctx, nil, updateTaskInput{TaskID: "t1", Clear: []string{"estimate_hours"}}); err == nil {
		t.Error("clear=[estimate_hours] still accepted; the field no longer exists")
	}
}
