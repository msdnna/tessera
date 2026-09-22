package docroom

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

// A document room is reachable only through the registry: Notify, Send, Drop,
// Close and EvictUser all start from rs.rooms. A participant sitting in a room
// that is no longer registered still holds its socket and still answers lock
// requests, so nothing about it looks broken from the inside — it has simply
// stopped being addressable from the outside.
//
// These tests cover the one way that used to happen: Join published the room to
// the registry and only then put the participant in it, so a Leave arriving in
// between found an empty room and forgot it (#2932).

// registeredRoom returns the room the registry currently holds for a document,
// or nil. Reading it directly is the point — the question is not what the
// joiner thinks it joined, it is what everyone else can still find.
func registeredRoom(rs *Rooms, docID uuid.UUID) *Room {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.rooms[docID]
}

// The realistic pairing is a page reload: the new socket connects while the old
// one is still tearing down, both on the same document. Leave does re-check the
// room under the registry lock before forgetting it, which is enough only if a
// joiner is already counted by then.
//
// Repeated rather than orchestrated: the window is a mutex handoff, so there is
// no seam to pause at without instrumenting the code under test. The test is
// one-sided by construction — an atomic Join cannot fail it however the two
// goroutines interleave, so a green run is never a scheduling accident.
func TestJoinIsNotLostToAConcurrentLeave(t *testing.T) {
	const rounds = 500
	for i := range rounds {
		rooms := New()
		docID := uuid.New()

		leaving := NewParticipant(uuid.New(), "Алиса")
		rooms.Join(docID, leaving)

		arriving := NewParticipant(uuid.New(), "Борис")
		var joined *Room
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			rooms.Leave(docID, leaving)
		}()
		go func() {
			defer wg.Done()
			<-start
			joined = rooms.Join(docID, arriving)
		}()
		close(start)
		wg.Wait()

		if got := registeredRoom(rooms, docID); got != joined {
			t.Fatalf("round %d: joined a room the registry does not hold (registry has %p, joiner got %p) — "+
				"the newcomer is invisible to Notify/Drop/EvictUser", i, got, joined)
		}
		rooms.Close()
	}
}

// Why the registry entry is worth a test of its own: EvictUser is how a revoked
// membership takes a live document socket down, and it can only reach rooms the
// registry still lists. A joiner lost to the race above keeps their presence,
// keeps their block locks and keeps being told when the body is saved, for as
// long as the tab stays open.
func TestEvictUserReachesAJoinerThatRacedALeave(t *testing.T) {
	const rounds = 500
	for i := range rounds {
		rooms := New()
		docID := uuid.New()

		leaving := NewParticipant(uuid.New(), "Алиса")
		rooms.Join(docID, leaving)

		arriving := NewParticipant(uuid.New(), "Борис")
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			rooms.Leave(docID, leaving)
		}()
		go func() {
			defer wg.Done()
			<-start
			rooms.Join(docID, arriving)
		}()
		close(start)
		wg.Wait()

		rooms.EvictUser(arriving.UserID)
		select {
		case <-arriving.closed:
		default:
			t.Fatalf("round %d: revoking access left the socket up — the room is not in the registry", i)
		}
		rooms.Close()
	}
}

// The other half of the same invariant, and the one that has to keep holding
// after the fix: a room nobody is in is still forgotten. Rooms outlive nothing —
// documents are numerous, and an entry per document ever opened is a slow leak.
func TestAnEmptiedRoomIsStillForgotten(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()

	p := NewParticipant(uuid.New(), "Алиса")
	rooms.Join(docID, p)
	if rooms.Count() != 1 {
		t.Fatalf("joining left %d rooms, want 1", rooms.Count())
	}
	rooms.Leave(docID, p)
	if rooms.Count() != 0 {
		t.Fatalf("the last participant left but %d rooms remain", rooms.Count())
	}
}
