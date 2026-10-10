package handlers

import (
	"testing"
	"time"

	"tessera/internal/db"
)

// The three-way rule of the stage write-back (#3010), field by field.
func TestMergeMilestoneField(t *testing.T) {
	cases := []struct {
		name               string
		ours, base, theirs string
		want               fieldMerge
	}{
		{"in sync", "a", "a", "a", fieldMerge{local: "a", base: "a"}},
		{"both moved the same way", "b", "a", "b", fieldMerge{local: "b", base: "b"}},
		{"only Tessera moved", "b", "a", "a", fieldMerge{local: "b", base: "a", push: true}},
		{"only GitLab moved", "a", "a", "c", fieldMerge{local: "c", base: "c"}},
		{"GitLab cleared it", "a", "a", "", fieldMerge{local: "", base: ""}},
		{"both moved apart", "b", "a", "c", fieldMerge{local: "b", base: "a", conflict: true}},
	}
	for _, tc := range cases {
		if got := mergeMilestoneField(tc.ours, tc.base, tc.theirs); got != tc.want {
			t.Errorf("%s: merge(%q,%q,%q) = %+v, want %+v", tc.name, tc.ours, tc.base, tc.theirs, got, tc.want)
		}
	}
}

// A baseline is told apart from "none yet", and the stage's dates read as the
// UTC calendar day they are stored under.
func TestMilestoneSnapRoundTrip(t *testing.T) {
	if _, ok := parseMilestoneSnap([]byte("{}")); ok {
		t.Error("empty object read as a baseline")
	}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	s := snapOfMilestone(db.Milestone{Title: "S", Description: "d", StartDate: &start, State: "closed"})
	if s.Start != "2026-08-01" || s.Due != "" || s.State != "closed" {
		t.Fatalf("snap = %+v", s)
	}
	back, ok := parseMilestoneSnap(s.marshal())
	if !ok || back != s {
		t.Fatalf("round trip = %+v (%v), want %+v", back, ok, s)
	}
	if d := parseSnapDate("2026-08-01"); d == nil || !d.Equal(start) {
		t.Fatalf("parseSnapDate = %v", d)
	}
	if parseSnapDate("") != nil {
		t.Error("empty date parsed as a date")
	}
}
