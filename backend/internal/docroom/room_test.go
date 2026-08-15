package docroom

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// drain reads every frame queued for a participant without blocking.
func drain(p *Participant) []map[string]any {
	var out []map[string]any
	for {
		select {
		case raw, ok := <-p.Out():
			if !ok {
				return out
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			out = append(out, m)
		default:
			return out
		}
	}
}

// lastState returns the most recent state snapshot in a participant's queue.
func lastState(t *testing.T, p *Participant) map[string]any {
	t.Helper()
	var last map[string]any
	for _, m := range drain(p) {
		if m["type"] == TypeState {
			last = m
		}
	}
	if last == nil {
		t.Fatal("no state snapshot delivered")
	}
	return last
}

// lockedBy names the holder of a block, or "" when it is free.
func lockedBy(state StateMsg, blockID string) string {
	for _, l := range state.Locks {
		if l.BlockID == blockID {
			return l.Name
		}
	}
	return ""
}

func TestJoinAnnouncesEveryone(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()

	alice := NewParticipant(uuid.New(), "Алиса")
	rooms.Join(docID, alice)
	// The first frame is the welcome: a client with two tabs open can only tell
	// its own lock from its other tab's by connection id.
	first := drain(alice)
	if len(first) == 0 || first[0]["type"] != TypeWelcome {
		t.Fatalf("expected a welcome frame first, got %v", first)
	}
	if first[0]["conn_id"] != alice.ID.String() {
		t.Errorf("welcome carried conn_id %v, want %s", first[0]["conn_id"], alice.ID)
	}

	bob := NewParticipant(uuid.New(), "Боб")
	rooms.Join(docID, bob)

	// Alice must learn about Bob without asking — that is the whole point of
	// presence.
	state := lastState(t, alice)
	viewers, _ := state["viewers"].([]any)
	if len(viewers) != 2 {
		t.Fatalf("alice sees %d viewers after bob joined, want 2", len(viewers))
	}
}

func TestSecondEditorIsRefusedAndTold(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()

	alice := NewParticipant(uuid.New(), "Алиса")
	bob := NewParticipant(uuid.New(), "Боб")
	room := rooms.Join(docID, alice)
	rooms.Join(docID, bob)

	if !room.Lock(alice, "block-1", now) {
		t.Fatal("alice could not take a free block")
	}
	if room.Lock(bob, "block-1", now) {
		t.Fatal("bob took a block alice holds")
	}

	// Refusal has to name the holder: "занято" without a name is not the feature
	// the task asked for.
	var denial map[string]any
	for _, m := range drain(bob) {
		if m["type"] == TypeDenied {
			denial = m
		}
	}
	if denial == nil {
		t.Fatal("bob was refused silently")
	}
	if denial["name"] != "Алиса" || denial["block_id"] != "block-1" {
		t.Errorf("denial = %v, want alice/block-1", denial)
	}

	// A different block is free game — locking is per block, not per document.
	if !room.Lock(bob, "block-2", now) {
		t.Error("bob could not take a different block")
	}
}

func TestLockMovesWithTheCaret(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()

	alice := NewParticipant(uuid.New(), "Алиса")
	bob := NewParticipant(uuid.New(), "Боб")
	room := rooms.Join(docID, alice)
	rooms.Join(docID, bob)

	room.Lock(alice, "block-1", now)
	room.Lock(alice, "block-2", now)

	// Walking down a page must not leave a trail of blocks locked behind you.
	if got := lockedBy(room.State(now), "block-1"); got != "" {
		t.Errorf("block-1 still held by %q after alice moved on", got)
	}
	if !room.Lock(bob, "block-1", now) {
		t.Error("bob could not take the block alice left")
	}
}

func TestLockExpiresWhenClientVanishes(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()

	alice := NewParticipant(uuid.New(), "Алиса")
	bob := NewParticipant(uuid.New(), "Боб")
	room := rooms.Join(docID, alice)
	rooms.Join(docID, bob)
	room.Lock(alice, "block-1", now)

	// A client that dies without closing its socket (lid shut, tab killed) must
	// not hold the block forever.
	later := now.Add(LockTTL + time.Second)
	rooms.SweepAll(later)
	if got := lockedBy(room.State(later), "block-1"); got != "" {
		t.Fatalf("expired lock still held by %q", got)
	}

	// Refreshing before the TTL keeps it, which is what the client's heartbeat
	// relies on.
	room.Lock(alice, "block-9", now)
	room.Lock(alice, "block-9", now.Add(LockTTL/2))
	rooms.SweepAll(now.Add(LockTTL + time.Second))
	if got := lockedBy(room.State(now.Add(LockTTL+time.Second)), "block-9"); got != "Алиса" {
		t.Errorf("refreshed lock was dropped, holder = %q", got)
	}
}

func TestLeaveReleasesLocksImmediately(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()

	alice := NewParticipant(uuid.New(), "Алиса")
	bob := NewParticipant(uuid.New(), "Боб")
	room := rooms.Join(docID, alice)
	rooms.Join(docID, bob)
	room.Lock(alice, "block-1", now)

	rooms.Leave(docID, alice)

	// Closing a tab should free the block now, not in 30 seconds.
	if got := lockedBy(room.State(now), "block-1"); got != "" {
		t.Errorf("block still held by %q after its holder left", got)
	}
	if room.Size() != 1 {
		t.Errorf("room size = %d after one of two left, want 1", room.Size())
	}
}

func TestUnlockOnlyReleasesYourOwn(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()

	alice := NewParticipant(uuid.New(), "Алиса")
	bob := NewParticipant(uuid.New(), "Боб")
	room := rooms.Join(docID, alice)
	rooms.Join(docID, bob)
	room.Lock(alice, "block-1", now)

	// A stale client must not be able to unlock the block someone else is typing
	// in — that would hand the block to whoever misbehaves.
	room.Unlock(bob, "block-1", now)
	if got := lockedBy(room.State(now), "block-1"); got != "Алиса" {
		t.Errorf("holder after a foreign unlock = %q, want Алиса", got)
	}
	room.Unlock(alice, "block-1", now)
	if got := lockedBy(room.State(now), "block-1"); got != "" {
		t.Errorf("holder after its own unlock = %q, want none", got)
	}
}

func TestTwoTabsOfOneUserAreSeparateHolders(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()
	uid := uuid.New()

	tab1 := NewParticipant(uid, "Алиса")
	tab2 := NewParticipant(uid, "Алиса")
	room := rooms.Join(docID, tab1)
	rooms.Join(docID, tab2)

	room.Lock(tab1, "block-1", now)
	// Identity is the connection, not the user: otherwise the second tab would
	// inherit the first tab's lock and both would write the same block.
	if room.Lock(tab2, "block-1", now) {
		t.Error("the user's other tab took a block their first tab holds")
	}
	// The two tabs are still one person in the viewer list.
	state := room.State(now)
	if len(state.Viewers) != 1 || state.Viewers[0].Conns != 2 {
		t.Errorf("viewers = %+v, want one person with two connections", state.Viewers)
	}
}

func TestRoomIsForgottenWhenEmpty(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()

	alice := NewParticipant(uuid.New(), "Алиса")
	rooms.Join(docID, alice)
	if rooms.Count() != 1 {
		t.Fatalf("rooms = %d after a join, want 1", rooms.Count())
	}
	rooms.Leave(docID, alice)
	// Documents are numerous and long-lived; a room per document ever opened is
	// a slow leak.
	if rooms.Count() != 0 {
		t.Errorf("rooms = %d after the last participant left, want 0", rooms.Count())
	}
}

func TestDropDisconnectsEveryone(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()

	alice := NewParticipant(uuid.New(), "Алиса")
	rooms.Join(docID, alice)
	rooms.Drop(docID)

	// The deleted document's readers get a closed channel, which is the write
	// pump's cue to send a close frame.
	for range alice.Out() { //nolint:revive // draining until close is the assertion
	}
	if rooms.Count() != 0 {
		t.Errorf("rooms = %d after drop, want 0", rooms.Count())
	}
}

func TestSlowParticipantIsEvictedNotSilentlySkipped(t *testing.T) {
	rooms := New()
	defer rooms.Close()
	docID := uuid.New()
	now := time.Now()

	slow := NewParticipant(uuid.New(), "Медленный")
	other := NewParticipant(uuid.New(), "Быстрый")
	room := rooms.Join(docID, slow)
	rooms.Join(docID, other)

	// Nobody reads `slow`, so its buffer fills. The shared workspace hub would
	// drop frames here and flag a resync; this room evicts instead, because a
	// dropped snapshot would freeze someone's view of who holds what.
	for i := 0; i < sendBuffer*2; i++ {
		room.Lock(other, "block-"+string(rune('a'+i%20)), now.Add(time.Duration(i)*time.Second))
	}
	if _, open := <-slow.Out(); !open {
		t.Fatal("evicted participant's channel should still yield buffered frames")
	}
	// Draining to completion must terminate: eviction closed the channel.
	for range slow.Out() { //nolint:revive // draining until close is the assertion
	}
	if room.Size() != 2 {
		// Eviction closes the socket; the read pump then calls Leave. The room
		// itself does not remove the member, so the count is unchanged here.
		t.Logf("room size after eviction = %d", room.Size())
	}
}
