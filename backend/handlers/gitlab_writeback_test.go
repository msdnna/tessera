package handlers

import "testing"

func TestShouldPushWriteback(t *testing.T) {
	cases := []struct {
		name      string
		kind      string
		payload   map[string]any
		lastState string
		prioInv   bool
		want      bool
	}{
		{"state changed", "state", map[string]any{"state": "closed"}, "opened", false, true},
		{"state echo suppressed", "state", map[string]any{"state": "closed"}, "closed", false, false},
		{"state missing", "state", map[string]any{}, "opened", false, false},
		{"priority invertible", "priority", map[string]any{"priority": float64(3)}, "", true, true},
		{"priority not invertible", "priority", map[string]any{"priority": float64(3)}, "", false, false},
		{"comment always", "comment", map[string]any{"body": "hi"}, "", false, true},
		{"title_desc pushable", "title_desc", map[string]any{}, "", true, true},
		{"milestone pushable", "milestone", map[string]any{}, "", true, true},
		{"unknown kind", "weight", map[string]any{}, "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldPushWriteback(tc.kind, tc.payload, tc.lastState, tc.prioInv); got != tc.want {
				t.Errorf("shouldPushWriteback(%q) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}
