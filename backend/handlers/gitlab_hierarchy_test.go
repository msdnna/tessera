package handlers

import (
	"testing"

	"github.com/google/uuid"
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
