package handlers

import "testing"

// TestEvalConflict covers the three-way decision: in-sync no-op, clean overwrite
// (baseline matches or is absent), and a real both-sides-changed conflict.
func TestEvalConflict(t *testing.T) {
	f := func(base, ours, theirs string, present bool) conflictField {
		return conflictField{Field: "due", Base: base, Ours: ours, Theirs: theirs, basePresent: present}
	}
	cases := []struct {
		name    string
		triples []conflictField
		want    conflictDecision
	}{
		{"already in sync", []conflictField{f("2026-01-01", "2026-02-02", "2026-02-02", true)}, conflictNoop},
		{"clean push (baseline matches)", []conflictField{f("2026-01-01", "2026-02-02", "2026-01-01", true)}, conflictProceed},
		{"clean push (no baseline)", []conflictField{f("", "2026-02-02", "2026-03-03", false)}, conflictProceed},
		{"conflict (both moved)", []conflictField{f("2026-01-01", "2026-02-02", "2026-03-03", true)}, conflictParked},
		{"empty triples", nil, conflictNoop},
		{"clear ours, GitLab unchanged", []conflictField{f("2026-01-01", "", "2026-01-01", true)}, conflictProceed},
		{"both cleared", []conflictField{f("2026-01-01", "", "", true)}, conflictNoop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := evalConflict(tc.triples)
			if got != tc.want {
				t.Errorf("evalConflict = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestConflictCheckedKind documents which kinds go through detection this pass.
func TestConflictCheckedKind(t *testing.T) {
	for _, k := range []string{"due", "estimate", "title_desc", "state", "priority"} {
		if !conflictCheckedKind(k) {
			t.Errorf("expected %q to be conflict-checked", k)
		}
	}
	for _, k := range []string{"labels", "assignees", "comment"} {
		if conflictCheckedKind(k) {
			t.Errorf("did not expect %q to be conflict-checked", k)
		}
	}
}
