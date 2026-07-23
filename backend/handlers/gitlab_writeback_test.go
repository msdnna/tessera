package handlers

import "testing"

// shouldPushWriteback is now purely the completion→state echo guard; every other
// trigger is loop-safe and always passes (the binding gate does the real filtering).
func TestShouldPushWriteback(t *testing.T) {
	cases := []struct {
		name      string
		kind      string
		payload   map[string]any
		lastState string
		want      bool
	}{
		{"completion → closed, was open", "completion", map[string]any{"completed": true}, "opened", true},
		{"completion echo suppressed", "completion", map[string]any{"completed": true}, "closed", false},
		{"completion → open, was open", "completion", map[string]any{"completed": false}, "opened", false},
		{"legacy state payload changed", "state", map[string]any{"state": "closed"}, "opened", true},
		{"legacy state echo suppressed", "state", map[string]any{"state": "closed"}, "closed", false},
		{"priority always passes gate", "priority", map[string]any{"priority": float64(3)}, "", true},
		{"comment always", "comment", map[string]any{"body": "hi"}, "", true},
		{"title_desc pushable", "title_desc", map[string]any{}, "", true},
		{"milestone pushable", "milestone", map[string]any{}, "", true},
		{"column pushable", "column", map[string]any{"column_id": "c1"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldPushWriteback(tc.kind, tc.payload, tc.lastState); got != tc.want {
				t.Errorf("shouldPushWriteback(%q) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// triggerFromKind maps enqueue kind+payload to a BindTrigger for the enqueue gate.
func TestTriggerFromKind(t *testing.T) {
	col := triggerFromKind("column", map[string]any{"column_id": "c1", "column_name": "В процессе"})
	if col.Type != "column" || col.ColumnID != "c1" || col.ColumnName != "В процессе" {
		t.Errorf("column trigger = %+v", col)
	}
	prio := triggerFromKind("priority", map[string]any{"priority": float64(2)})
	if prio.Priority == nil || *prio.Priority != 2 {
		t.Errorf("priority trigger = %+v", prio)
	}
	comp := triggerFromKind("completion", map[string]any{"completed": true})
	if comp.Completed == nil || !*comp.Completed {
		t.Errorf("completion trigger = %+v", comp)
	}
	// legacy "state" kind aliases to completion
	legacy := triggerFromKind("state", map[string]any{"state": "closed"})
	if legacy.Type != "completion" || legacy.Completed == nil || !*legacy.Completed {
		t.Errorf("legacy state alias = %+v", legacy)
	}
	due := triggerFromKind("due", map[string]any{})
	if due.DateKind != "due" {
		t.Errorf("due trigger default kind = %+v", due)
	}
}
