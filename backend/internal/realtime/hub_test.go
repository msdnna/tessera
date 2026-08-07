package realtime

import (
	"testing"
	"time"
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
