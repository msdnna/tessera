package handlers

import (
	"testing"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// step builds one route entry. Position and status are the only fields the rule
// functions read, so the rest stays zero.
func step(pos int32, status string) db.DocumentApprovalStep {
	return db.DocumentApprovalStep{ID: uuid.New(), Position: pos, Status: status}
}

// TestNextApprovalStatus pins the rule that decides when a route stops being
// open. The rejection branch matters most: it has to win over pending steps that
// have not been asked yet, because the point of a rejection is that the text goes
// back for changes before anyone else spends time reading it.
func TestNextApprovalStatus(t *testing.T) {
	cases := []struct {
		name  string
		steps []db.DocumentApprovalStep
		want  string
	}{
		{"nobody signed yet", []db.DocumentApprovalStep{
			step(0, approvalPending), step(1, approvalPending),
		}, approvalPending},
		{"one of two signed", []db.DocumentApprovalStep{
			step(0, approvalApproved), step(1, approvalPending),
		}, approvalPending},
		{"everyone signed", []db.DocumentApprovalStep{
			step(0, approvalApproved), step(1, approvalApproved),
		}, approvalApproved},
		{"a rejection outranks the pending rest", []db.DocumentApprovalStep{
			step(0, approvalApproved), step(1, approvalRejected), step(2, approvalPending),
		}, approvalRejected},
		{"a rejection outranks a fully signed rest", []db.DocumentApprovalStep{
			step(0, approvalRejected), step(1, approvalApproved),
		}, approvalRejected},
		// A route with no steps cannot be created through the handler, but if one
		// ever existed it must not read as "approved by nobody" — that is the one
		// wrong answer here, and it is the answer a naive all-approved loop gives.
		{"an empty route is not approved", nil, approvalPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextApprovalStatus(tc.steps); got != tc.want {
				t.Fatalf("nextApprovalStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCanDecideNowSequential is what makes the ordering of a sequential route
// real rather than advisory: enforced on the client alone, a later approver could
// sign before the person whose objection would have stopped the route.
func TestCanDecideNowSequential(t *testing.T) {
	first, second, third := step(0, approvalApproved), step(1, approvalPending), step(2, approvalPending)
	steps := []db.DocumentApprovalStep{first, second, third}

	if !canDecideNow(approvalModeSequential, steps, second.ID) {
		t.Fatal("the earliest pending step must be signable")
	}
	if canDecideNow(approvalModeSequential, steps, third.ID) {
		t.Fatal("a later approver signed ahead of the pending one before them")
	}
	if canDecideNow(approvalModeSequential, steps, first.ID) {
		t.Fatal("an already-signed step was offered a second decision")
	}
	if canDecideNow(approvalModeSequential, steps, uuid.New()) {
		t.Fatal("a step outside the route was allowed to decide")
	}

	// The rule reads positions, not slice order: the query orders by position
	// today, and a route that arrived shuffled must not hand the turn to whoever
	// happens to be first in the slice.
	shuffled := []db.DocumentApprovalStep{third, second, first}
	if !canDecideNow(approvalModeSequential, shuffled, second.ID) {
		t.Fatal("sequential order followed slice order instead of position")
	}
	if canDecideNow(approvalModeSequential, shuffled, third.ID) {
		t.Fatal("shuffling the slice let a later approver sign early")
	}
}

// TestCanDecideNowParallel covers the other half of the mode split: everyone was
// asked at once, so position constrains nothing but the order the panel lists
// people in.
func TestCanDecideNowParallel(t *testing.T) {
	first, second := step(0, approvalPending), step(1, approvalPending)
	steps := []db.DocumentApprovalStep{first, second}

	if !canDecideNow(approvalModeParallel, steps, second.ID) {
		t.Fatal("parallel mode made a later approver wait")
	}
	if !canDecideNow(approvalModeParallel, steps, first.ID) {
		t.Fatal("parallel mode blocked the first approver")
	}

	signed := []db.DocumentApprovalStep{step(0, approvalApproved), second}
	if canDecideNow(approvalModeParallel, signed, signed[0].ID) {
		t.Fatal("a signed step was offered a second decision in parallel mode")
	}
	if canDecideNow(approvalModeParallel, steps, uuid.New()) {
		t.Fatal("a step outside the route was allowed to decide")
	}
}

// TestStepForApprover checks the lookup that answers "am I on this route", and
// in particular that a route whose approver account was deleted (approver_id set
// to NULL) does not match the caller by accident.
func TestStepForApprover(t *testing.T) {
	me, other := uuid.New(), uuid.New()
	mine := step(1, approvalPending)
	mine.ApproverID = &me
	theirs := step(0, approvalPending)
	theirs.ApproverID = &other
	orphan := step(2, approvalPending) // account deleted: approver_id is NULL

	steps := []db.DocumentApprovalStep{theirs, mine, orphan}
	got, found := stepForApprover(steps, me)
	if !found || got.ID != mine.ID {
		t.Fatalf("stepForApprover found %v/%v, want my step", got.ID, found)
	}
	if _, found := stepForApprover(steps, uuid.New()); found {
		t.Fatal("a stranger was placed on the route")
	}
	if _, found := stepForApprover([]db.DocumentApprovalStep{orphan}, uuid.Nil); found {
		t.Fatal("a step with no approver matched a nil caller")
	}
}

// TestDedupeUUIDsKeepsOrder matters because the order of the approvers list *is*
// the sequential route, and because a repeated name would otherwise hit the
// UNIQUE constraint as a 500 — or, in sequential mode, stall the route on a step
// whose single decision cannot satisfy two places.
func TestDedupeUUIDsKeepsOrder(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	got := dedupeUUIDs([]uuid.UUID{b, a, b, c, a})
	want := []uuid.UUID{b, a, c}
	if len(got) != len(want) {
		t.Fatalf("dedupeUUIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupeUUIDs = %v, want %v (order is the route)", got, want)
		}
	}
	if out := dedupeUUIDs(nil); len(out) != 0 {
		t.Fatalf("dedupeUUIDs(nil) = %v, want empty", out)
	}
}
