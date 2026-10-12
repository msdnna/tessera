package handlers

import (
	"testing"

	"github.com/google/uuid"
)

// A worker acts with no user behind it — the recurrence engine passes uuid.Nil for
// the schedule-driven path, and the wire must carry that as an ABSENT actor, not as
// the zero uuid. A set actor is a claim that a person did this: clients attribute
// board-activity toasts by it, and "00000000-…" would be shown as somebody (#2932).
func TestActorTag(t *testing.T) {
	if got := actorTag(uuid.Nil); got != "" {
		t.Errorf("actorTag(uuid.Nil) = %q, want empty", got)
	}
	id := uuid.New()
	if got := actorTag(id); got != id.String() {
		t.Errorf("actorTag(%v) = %q, want %q", id, got, id.String())
	}
}
