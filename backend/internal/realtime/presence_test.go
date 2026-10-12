package realtime

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// nextPresence reads the next queued board.presence event (Run is not running,
// so Broadcast leaves events in the channel).
func nextPresence(t *testing.T, h *Hub) (string, string, []string) {
	t.Helper()
	select {
	case ev := <-h.broadcast:
		if ev.Type != EventBoardPresence {
			t.Fatalf("event type %q, want %q", ev.Type, EventBoardPresence)
		}
		var d struct {
			BoardID string   `json:"board_id"`
			UserIDs []string `json:"user_ids"`
		}
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			t.Fatal(err)
		}
		return ev.Scope, d.BoardID, d.UserIDs
	default:
		t.Fatal("no presence event queued")
	}
	return "", "", nil
}

func TestBoardPresence(t *testing.T) {
	h := NewHub()
	ws, other := uuid.NewString(), uuid.NewString()
	board, foreign := uuid.New(), uuid.New()
	h.SetBoardResolver(func(id uuid.UUID) (string, bool) {
		switch id {
		case board:
			return ws, true
		case foreign:
			return other, true
		}
		return "", false
	})
	ann, bob := uuid.New(), uuid.New()
	a1 := newTestClient(ann, ws)
	a2 := newTestClient(ann, ws) // a second tab
	b := newTestClient(bob, ws)

	h.onFrame(a1, []byte(`{"type":"presence","board_id":"`+board.String()+`"}`))
	scope, id, users := nextPresence(t, h)
	if scope != ws || id != board.String() || !reflect.DeepEqual(users, []string{ann.String()}) {
		t.Fatalf("join: scope=%s board=%s users=%v", scope, id, users)
	}

	// The second tab of the same person changes nobody's list but is announced.
	h.onFrame(a2, []byte(`{"type":"presence","board_id":"`+board.String()+`"}`))
	_, _, users = nextPresence(t, h)
	if len(users) != 1 {
		t.Fatalf("two tabs of one user count twice: %v", users)
	}
	h.onFrame(b, []byte(`{"type":"presence","board_id":"`+board.String()+`"}`))
	_, _, users = nextPresence(t, h)
	if len(users) != 2 {
		t.Fatalf("second person: %v", users)
	}

	// The same board again is not news.
	h.onFrame(b, []byte(`{"type":"presence","board_id":"`+board.String()+`"}`))
	if len(h.broadcast) != 0 {
		t.Fatal("re-announcing the same board broadcast again")
	}

	// A board outside the socket's workspaces, an unknown one and junk are ignored.
	h.onFrame(b, []byte(`{"type":"presence","board_id":"`+foreign.String()+`"}`))
	h.onFrame(b, []byte(`{"type":"presence","board_id":"`+uuid.NewString()+`"}`))
	h.onFrame(b, []byte(`{"type":"presence","board_id":"nope"}`))
	h.onFrame(b, []byte(`not json`))
	if len(h.broadcast) != 0 {
		t.Fatalf("refused frames broadcast %d events", len(h.broadcast))
	}
	if got := h.BoardPresence(board); len(got) != 2 {
		t.Fatalf("refused frames moved someone: %v", got)
	}

	// Closing a socket (what readPump does on the way out) leaves at once; Ann
	// stays while her other tab is there.
	h.setPresence(a1, uuid.Nil)
	_, _, users = nextPresence(t, h)
	if len(users) != 2 {
		t.Fatalf("one of two tabs closed: %v", users)
	}
	h.onFrame(b, []byte(`{"type":"presence","board_id":""}`))
	_, _, users = nextPresence(t, h)
	if !reflect.DeepEqual(users, []string{ann.String()}) {
		t.Fatalf("leave: %v", users)
	}
}
