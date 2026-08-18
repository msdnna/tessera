package handlers

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/gitlab"
)

// TestResolveParentRef is the regression guard for #2592 p.4 ("later pulls must not
// break parent/child connectivity"). The bug it locks down: an issue nobody claimed
// used to be re-parented to nil unconditionally, so one failed ChildIIDs call read as
// "all of this parent's subtasks were detached" and scattered them to top-level.
//
// The distinction under test is between the two ways an issue ends up unclaimed:
// GitLab told us it has no such child (a real detach — re-parent), versus we never got
// an answer (unknown — leave the task alone).
func TestResolveParentRef(t *testing.T) {
	parent := uuid.New()
	cases := []struct {
		name           string
		claimedBy      *uuid.UUID
		hierarchyKnown bool
		wantID         *uuid.UUID
		wantKnown      bool
	}{
		{"claimed by a grouped parent", &parent, true, &parent, true},
		{"claimed even though another parent's query failed", &parent, false, &parent, true},
		{"unclaimed, hierarchy answered — real detach", nil, true, nil, true},
		{"unclaimed, hierarchy unknown — keep the subtask put", nil, false, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveParentRef(tc.claimedBy, tc.hierarchyKnown)
			if got.known != tc.wantKnown {
				t.Fatalf("known = %v, want %v", got.known, tc.wantKnown)
			}
			if !sameUUIDPtr(got.id, tc.wantID) {
				t.Fatalf("id = %v, want %v", got.id, tc.wantID)
			}
		})
	}
}

// TestResolveParentRefOnlyDetachesWhenAnswered states the safety property directly:
// the un-parenting branch is reachable only when GitLab actually answered. Written as
// a property rather than another row above because it is the invariant that matters —
// any future branch added to resolveParentRef has to keep satisfying it.
func TestResolveParentRefOnlyDetachesWhenAnswered(t *testing.T) {
	parent := uuid.New()
	for _, claimedBy := range []*uuid.UUID{nil, &parent} {
		for _, known := range []bool{true, false} {
			ref := resolveParentRef(claimedBy, known)
			detaches := ref.known && ref.id == nil
			if detaches && !known {
				t.Fatalf("resolveParentRef(%v, %v) un-parents on an unanswered hierarchy", claimedBy, known)
			}
		}
	}
}

// TestCheckGroupGate covers the config-only refusals of the grouping endpoints. The
// row that carries the most weight is the last one: the button writes a label, and a
// label this integration's rules do not read back as grouping would look like it
// worked and then import as an ordinary tag on the next pull, leaving the parent
// ungrouped and its children unclaimed.
func TestCheckGroupGate(t *testing.T) {
	on := gitlab.Writeback{PushChildren: true}
	rules := gitlab.DefaultRules()
	cases := []struct {
		name       string
		enabled    bool
		wb         gitlab.Writeback
		rules      gitlab.Rules
		linked     bool
		wantStatus int
	}{
		{"allowed", true, on, rules, true, 0},
		{"integration disabled", false, on, rules, true, http.StatusBadRequest},
		{"push_children off", true, gitlab.Writeback{}, rules, true, http.StatusBadRequest},
		{"task not linked", true, on, rules, false, http.StatusBadRequest},
		{
			"label is not a grouping label under these rules",
			true,
			gitlab.Writeback{PushChildren: true, GroupLabel: "просто тег"},
			rules, true, http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg := checkGroupGate(tc.enabled, tc.wb, tc.rules, tc.linked)
			if status != tc.wantStatus {
				t.Fatalf("status = %d (%q), want %d", status, msg, tc.wantStatus)
			}
			if status != 0 && msg == "" {
				t.Fatal("refused without a reason")
			}
		})
	}
}

// TestCheckGroupGateRejectsUnreadableLabelEvenWhenEverythingElseIsOn states that
// guard as a property: no combination of "everything enabled" may let a label through
// that ResolvesToGroup rejects. It is the one gate that protects data rather than
// permissions.
func TestCheckGroupGateRejectsUnreadableLabelEvenWhenEverythingElseIsOn(t *testing.T) {
	rules := gitlab.DefaultRules()
	for _, label := range []string{"todo", "S: В работе", "P: Высокий", "M", "M:"} {
		wb := gitlab.Writeback{PushChildren: true, GroupLabel: label}
		if rules.ResolvesToGroup(wb.EffectiveGroupLabel()) {
			continue // genuinely grouping under the default rules — not a case here
		}
		if status, _ := checkGroupGate(true, wb, rules, true); status == 0 {
			t.Fatalf("group_label %q accepted, but the pull would not read it as grouping", label)
		}
	}
}
