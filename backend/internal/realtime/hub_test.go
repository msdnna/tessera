package realtime

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// waitFor polls until cond holds, so the tests don't race the hub goroutine.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (h *Hub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close is what makes a deploy look like a reconnect rather than a broken
// socket: Run must exit and every client's send channel must be closed, which
// is the signal the write pump turns into a normal close frame.
func TestHubCloseReleasesClients(t *testing.T) {
	h := NewHub()
	stopped := make(chan struct{})
	go func() {
		h.Run()
		close(stopped)
	}()

	c := &Client{hub: h, send: make(chan Event, 1)}
	h.register <- c
	waitFor(t, "client registration", func() bool { return h.clientCount() == 1 })

	h.Close()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Close")
	}
	select {
	case _, ok := <-c.send:
		if ok {
			t.Fatal("send channel delivered an event instead of being closed")
		}
	default:
		t.Fatal("client send channel was not closed")
	}
	if n := h.clientCount(); n != 0 {
		t.Fatalf("client set not drained: %d remain", n)
	}
}

// Close is called from the shutdown path and may race a second caller; a double
// close of the done channel would panic the process on its way out.
func TestHubCloseIsIdempotent(t *testing.T) {
	h := NewHub()
	stopped := make(chan struct{})
	go func() {
		h.Run()
		close(stopped)
	}()
	h.Close()
	h.Close() // a second close of done would panic

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after a repeated Close")
	}
}

// A client that unregisters after the hub is gone must not park forever on a
// channel nobody reads — that would leak a goroutine per open socket.
func TestClientUnregisterAfterCloseDoesNotBlock(t *testing.T) {
	h := NewHub()
	go h.Run()
	h.Close()
	waitFor(t, "hub shutdown", func() bool {
		select {
		case <-h.done:
			return true
		default:
			return false
		}
	})

	done := make(chan struct{})
	go func() {
		select {
		case h.unregister <- &Client{hub: h, send: make(chan Event, 1)}:
		case <-h.done:
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("unregister blocked after Close")
	}
}

// Broadcast must stay non-blocking after shutdown: handlers still call it while
// in-flight requests drain, and a blocked broadcast would stall the drain.
func TestBroadcastAfterCloseDoesNotBlock(t *testing.T) {
	h := NewHub()
	go h.Run()
	h.Close()

	done := make(chan struct{})
	go func() {
		for range cap(h.broadcast) + 5 {
			h.Broadcast(Event{Type: "task.updated"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked after Close")
	}
}

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
