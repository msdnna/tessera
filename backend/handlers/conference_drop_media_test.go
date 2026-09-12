package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/livekit"
	"tessera/internal/mail"
)

// fakeRoomService is a stub livekit-server that records the DeleteRoom calls it
// receives. Only that one RPC: the teardown under test makes no other.
type fakeRoomService struct {
	mu      sync.Mutex
	deleted []string
}

func (f *fakeRoomService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/twirp/livekit.RoomService/DeleteRoom" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	room, _ := req["room"].(string)
	f.mu.Lock()
	f.deleted = append(f.deleted, room)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{})
}

// awaitDeleted polls until the stub has been told to delete room, since the
// teardown is dispatched in the background (nobody waits on the SFU).
func (f *fakeRoomService) awaitDeleted(room string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, got := range f.deleted {
			if got == room {
				f.mu.Unlock()
				return true
			}
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// apiWithFakeSFU builds an API whose only wired dependency is the stub SFU.
// dropConfRoom touches neither the database nor the hub, and the conference-room
// registry is optional wiring it already nil-guards.
func apiWithFakeSFU(t *testing.T) (*API, *fakeRoomService) {
	t.Helper()
	fake := &fakeRoomService{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	api := NewAPI(nil, nil, nil, t.TempDir(), "handlers-unit-test-encryption-key",
		mail.New(mail.Config{}), "http://test", "")
	api.WireLiveKit(livekit.New(livekit.Config{
		URL: srv.URL, PublicURL: "wss://test", APIKey: "devkey",
		APISecret: "devsecret-at-least-32-chars-long!!",
	}))
	return api, fake
}

// TestDroppingAConferenceEndsItAtTheSFU: ending or deleting a conference has to
// reach the media, not only the presence room (#2932).
//
// The two halves of a call live in different places: the roster is ours, the
// audio and video are the SFU's, and LiveKit authorises a participant once — at
// the join — and never re-reads anything after that. So a teardown that stops at
// closing the presence sockets leaves the room itself alive: a client that
// ignores the "ended" frame, or anyone re-presenting the join token it still
// holds for the rest of its fifteen minutes, keeps talking in a conference that
// Tessera considers over. After a delete there is not even a row left to hang a
// refusal on, and the room survives as long as somebody keeps it non-empty.
//
// This is the same seam a kick and an eviction already use; the only thing the
// end path needed was to use it too.
func TestDroppingAConferenceEndsItAtTheSFU(t *testing.T) {
	api, fake := apiWithFakeSFU(t)
	confID := uuid.New()

	api.dropConfRoom(confID, "ended")

	if !fake.awaitDeleted(livekit.RoomName(confID)) {
		t.Fatal("ending the conference never reached the SFU: the room (and everyone publishing to it) lives on")
	}
}

// TestDroppingAConferenceWithoutAnSFUIsNotAPanic pins the install without
// LIVEKIT_* set, which is the default: the client reports itself disabled and
// the teardown has to be a no-op rather than a nil dereference in a detached
// goroutine, where nothing would catch it.
func TestDroppingAConferenceWithoutAnSFUIsNotAPanic(t *testing.T) {
	api := NewAPI(nil, nil, nil, t.TempDir(), "handlers-unit-test-encryption-key",
		mail.New(mail.Config{}), "http://test", "")

	api.dropConfRoom(uuid.New(), "deleted")
	// The teardown is asynchronous; give the goroutine a moment to have panicked
	// if it were going to, since a panic there takes the whole test binary down.
	time.Sleep(200 * time.Millisecond)
}
