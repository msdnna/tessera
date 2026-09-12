package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/livekit"
	"tessera/internal/mail"
)

// The purge that follows the deletion of a conference somebody was recording
// (#2932).
//
// Deleting the conference takes its recording rows with it, so from that moment
// the mp4 the egress is still finishing is reachable by nothing: both sweeps walk
// rows. Two things therefore have to happen, and neither is observable through the
// HTTP surface of a harness with no SFU — which is why they are pinned here, over
// real HTTP against a stub livekit-server.

// windDownDelay is how long the stub takes to land its file after being told to
// stop. Short, but on its own clock: the write is what a real egress does while
// nobody is looking, and a stub that only wrote when it was polled would agree
// with an implementation that never polls.
const windDownDelay = 300 * time.Millisecond

// windingDownEgress is a stub livekit-server for one job that is still running
// when it is asked to stop. It reports itself finished only once its file is on
// disk — and it creates the directory it writes into, exactly as the real worker
// does on its way to its output path.
type windingDownEgress struct {
	mu      sync.Mutex
	file    string // the mp4 this job lands as it winds down
	stopped []string
	wrote   bool
}

func (f *windingDownEgress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.Header().Set("Content-Type", "application/json")

	// Proto field names in the replies, as livekit-server renders them.
	item := map[string]any{"egress_id": "eg-winding-down", "room_name": "room"}
	switch r.URL.Path {
	case "/twirp/livekit.Egress/StopEgress":
		f.mu.Lock()
		id, _ := req["egressId"].(string)
		f.stopped = append(f.stopped, id)
		f.mu.Unlock()
		go f.windDown()
		item["status"] = "EGRESS_ENDING"
		_ = json.NewEncoder(w).Encode(item)
	case "/twirp/livekit.Egress/ListEgress":
		item["status"] = "EGRESS_ACTIVE"
		if f.landed() {
			item["status"] = "EGRESS_COMPLETE"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{item}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// windDown writes the mp4 a moment after the stop was acknowledged — the bytes
// arriving long after the row that named them was deleted.
func (f *windingDownEgress) windDown() {
	time.Sleep(windDownDelay)
	f.mu.Lock()
	file := f.file
	f.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(file), 0o775)
	_ = os.WriteFile(file, []byte("bytes written on the way out"), 0o644)
	f.mu.Lock()
	f.wrote = true
	f.mu.Unlock()
}

func (f *windingDownEgress) landed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.wrote
}

func (f *windingDownEgress) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stopped)
}

// apiWithFakeEgress builds an API whose only wired dependencies are the stub SFU
// and an uploads directory. purgeRecordingDir touches neither the database nor
// the hub.
func apiWithFakeEgress(t *testing.T) (*API, *windingDownEgress) {
	t.Helper()
	fake := &windingDownEgress{}
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

// TestPurgeWaitsForTheEgressBeforeClearingTheDirectory: the file arrives after
// the delete, so the purge cannot be a one-shot unlink.
//
// A recording in flight is a headless Chrome writing into a directory whose rows
// have just left by cascade. Two failures are possible and this pins both: leaving
// the worker running (it keeps recording a conference that no longer exists, until
// the two-hour session limit), and clearing the directory before it is done (the
// job recreates what it needs on the way to its output path, and the mp4 lands in
// an empty directory nobody will ever look in again).
func TestPurgeWaitsForTheEgressBeforeClearingTheDirectory(t *testing.T) {
	api, fake := apiWithFakeEgress(t)
	confID := uuid.New()
	dir, err := api.recordingDir(confID)
	if err != nil {
		t.Fatalf("recording dir: %v", err)
	}
	fake.mu.Lock()
	fake.file = filepath.Join(dir, uuid.NewString()+".mp4")
	fake.mu.Unlock()

	api.purgeRecordingDir(confID, "eg-winding-down")

	// The file has to have landed before the directory is judged: a purge that
	// cleared it on the way past and left before the worker wrote would look
	// identical at any earlier moment, and it is the file that matters.
	if !awaitTrue(fake.landed) {
		t.Fatal("the stub never wrote its file: the test is not exercising a wind-down")
	}
	if !awaitMissing(dir) {
		left, _ := os.ReadDir(dir)
		t.Fatalf("recording directory of a deleted conference still holds %d file(s) nothing points at", len(left))
	}
	if fake.stopCount() == 0 {
		t.Error("the egress was never stopped: a worker is still recording a conference that no longer exists")
	}
}

// TestPurgeWithoutAnEgressIsImmediate pins the ordinary delete — no recording was
// running — and the install with no SFU configured, which is the default. Nothing
// is being written in either case, so the directory goes at once rather than on a
// goroutine nobody waits for.
func TestPurgeWithoutAnEgressIsImmediate(t *testing.T) {
	api, _ := apiWithFakeEgress(t)
	confID := uuid.New()
	dir, err := api.recordingDir(confID)
	if err != nil {
		t.Fatalf("recording dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "leftover.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatalf("leftover file: %v", err)
	}

	api.purgeRecordingDir(confID, "")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory survived a purge with nothing to wait for: %v", err)
	}
}

// awaitMissing waits for a path to disappear: the purge outlives the call that
// starts it, by design.
func awaitMissing(path string) bool {
	return awaitTrue(func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	})
}

// awaitTrue polls a condition up to a deadline comfortably past one poll of the
// purge loop.
func awaitTrue(cond func() bool) bool {
	deadline := time.Now().Add(20 * time.Second)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
