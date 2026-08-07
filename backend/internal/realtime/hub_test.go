package realtime

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// newTestClient builds a hub-less client with a fixed scope set. The socket is
// nil: these tests exercise the fan-out decision, not the wire.
func newTestClient(userID uuid.UUID, scopes ...string) *Client {
	c := NewClient(nil, nil, userID, scopes)
	return c
}

func TestClientCanSee(t *testing.T) {
	wsA, wsB := uuid.New().String(), uuid.New().String()
	c := newTestClient(uuid.New(), wsA)

	cases := []struct {
		name  string
		scope string
		want  bool
	}{
		{"own workspace", wsA, true},
		{"foreign workspace", wsB, false},
		{"empty scope is nobody's", "", false},
		{"unknown id", uuid.New().String(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.canSee(tc.scope); got != tc.want {
				t.Fatalf("canSee(%q) = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}

// TestHubFanOutIsScoped is the regression guard for the pre-fix behaviour, where
// every client received every event regardless of workspace.
func TestHubFanOutIsScoped(t *testing.T) {
	h := NewHub()
	go h.Run()

	wsA, wsB := uuid.New().String(), uuid.New().String()
	alice := newTestClient(uuid.New(), wsA)
	bob := newTestClient(uuid.New(), wsB)
	h.register <- alice
	h.register <- bob

	h.Broadcast(Event{Scope: wsA, Type: "task.created"})

	select {
	case ev := <-alice.send:
		if ev.Type != "task.created" {
			t.Fatalf("alice got %q, want task.created", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("alice did not receive her own workspace's event")
	}

	select {
	case ev := <-bob.send:
		t.Fatalf("bob received a foreign workspace's event: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestHubMultiScope covers a user who belongs to several workspaces: they see
// events from each of them and nothing else.
func TestHubMultiScope(t *testing.T) {
	h := NewHub()
	go h.Run()

	wsA, wsB, wsC := uuid.New().String(), uuid.New().String(), uuid.New().String()
	c := newTestClient(uuid.New(), wsA, wsB)
	h.register <- c

	h.Broadcast(Event{Scope: wsC, Type: "nope"})
	h.Broadcast(Event{Scope: wsB, Type: "yes"})

	select {
	case ev := <-c.send:
		if ev.Type != "yes" {
			t.Fatalf("got %q, want the wsB event only", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered for a joined workspace")
	}
}

// TestHubDropUser checks that a membership change cuts the user's live sockets
// (all of them) without touching anyone else's.
func TestHubDropUser(t *testing.T) {
	h := NewHub()
	go h.Run()

	scope := uuid.New().String()
	uid := uuid.New()
	tab1 := newTestClient(uid, scope)
	tab2 := newTestClient(uid, scope)
	other := newTestClient(uuid.New(), scope)
	h.register <- tab1
	h.register <- tab2
	h.register <- other

	h.DropUser(uid)

	for i, c := range []*Client{tab1, tab2} {
		select {
		case _, ok := <-c.send:
			if ok {
				t.Fatalf("tab %d: send channel still open after DropUser", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("tab %d: send channel was not closed by DropUser", i)
		}
	}

	// The untouched client keeps working.
	h.Broadcast(Event{Scope: scope, Type: "still.here"})
	select {
	case ev, ok := <-other.send:
		if !ok || ev.Type != "still.here" {
			t.Fatalf("bystander client got %+v (open=%v), want the event", ev, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("DropUser disconnected an unrelated user")
	}
}
