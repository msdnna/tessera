// The background half of conference recording (#2864, subtask #2877).
//
// What is being pinned here is the part nobody watches: a recording ends when
// the last person leaves the call, hours after whoever started it closed the
// tab. Everything that decides whether that recording is playable, lost, or
// silently taking up disk forever happens in a tick loop, so this file drives
// single passes of it against a stub SFU.
//
// The stub is not a mock of our own client — the requests go over real HTTP and
// come back as the protojson LiveKit actually emits (int64s as strings, the
// zero-valued status omitted), which is where the units in the row come from.
//
// One property of the worker makes these tests safe to run beside the rest of
// the suite: it only touches rows that name an egress, rows that have named
// none for two minutes, and rows already past their expiry. Every recording
// planted by the flow tests next door is none of those.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/handlers"
	"tessera/internal/db"
	"tessera/internal/livekit"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// fakeEgress is a stub livekit-server: the two Egress RPCs the worker uses,
// answering from a table the test fills in.
type fakeEgress struct {
	mu      sync.Mutex
	// items are rendered the way livekit-server renders them: PROTO field
	// names (egress_id, file_results), not the camelCase JSON names. A stub
	// that answers in camelCase is worse than no stub — it agrees with a client
	// that reads nothing, which is exactly the bug the recording stand caught.
	items   []map[string]any
	stopped []string         // egress ids StopEgress was called with
}

// add records one egress job. status "" reproduces the real omission of
// EGRESS_STARTING; fileSize/fileNanos are only read for terminal statuses,
// exactly as LiveKit fills them in.
func (f *fakeEgress) add(egressID, room, status string, fileSize int64, fileNanos int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item := map[string]any{"egress_id": egressID, "room_name": room}
	if status != "" {
		item["status"] = status
	}
	if fileSize > 0 || fileNanos > 0 {
		item["file_results"] = []map[string]any{{
			"filename": "/data/uploads/rec/x.mp4",
			// Strings, not numbers: protojson renders int64 as a JSON string,
			// and a client that assumed otherwise would fail to decode every
			// finished recording.
			"size":     strconv.FormatInt(fileSize, 10),
			"duration": strconv.FormatInt(fileNanos, 10),
		}}
	}
	f.items = append(f.items, item)
}

func (f *fakeEgress) stopCount(egressID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, id := range f.stopped {
		if id == egressID {
			n++
		}
	}
	return n
}

func (f *fakeEgress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	str := func(k string) string {
		if v, ok := req[k].(string); ok {
			return v
		}
		return ""
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/twirp/livekit.Egress/ListEgress":
		// Requests keep the camelCase spelling our client sends: protojson's
		// parser accepts either, so only the replies above are one-sided.
		id, room := str("egressId"), str("roomName")
		out := []map[string]any{}
		for _, it := range f.items {
			if id != "" && it["egress_id"] != id {
				continue
			}
			if room != "" && it["room_name"] != room {
				continue
			}
			out = append(out, it)
		}
		// An unknown id is an empty list, not a 404 — the behaviour GetEgress
		// turns into ErrEgressGone.
		_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
	case "/twirp/livekit.Egress/StopEgress":
		id := str("egressId")
		f.stopped = append(f.stopped, id)
		for _, it := range f.items {
			if it["egress_id"] == id {
				_ = json.NewEncoder(w).Encode(it)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "not_found", "msg": "no such egress"})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// recordingWorker builds an API wired to a stub SFU. It is a second API beside
// the harness's own on purpose: the shared one has no LiveKit configured (which
// is what lets the flow tests assert the 503 path), and switching that on
// underneath tests running in parallel would change what they see.
func recordingWorker(t *testing.T) (*handlers.API, *fakeEgress) {
	t.Helper()
	fake := &fakeEgress{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	api := handlers.NewAPI(testQueries, testPool, realtime.NewHub(), testUploadDir,
		"integration-test-encryption-key", mail.New(mail.Config{}), "http://test", "")
	api.WireLiveKit(livekit.New(livekit.Config{
		URL: srv.URL, PublicURL: "wss://test", APIKey: "devkey", APISecret: "devsecret-at-least-32-chars-long!!",
	}))
	return api, fake
}

// attachEgress gives a planted row LiveKit's job id, the way the start handler
// does on its second statement.
func attachEgress(t *testing.T, recID, egressID string) {
	t.Helper()
	if _, err := testQueries.SetConferenceRecordingEgress(context.Background(), db.SetConferenceRecordingEgressParams{
		ID: uuid.MustParse(recID), EgressID: egressID,
	}); err != nil {
		t.Fatalf("attach egress: %v", err)
	}
}

// backdate moves a row's clock, so the worker's age-based guards can be reached
// without the test sleeping through them.
func backdate(t *testing.T, recID, column, interval string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"UPDATE conference_recordings SET "+column+" = now() - $2::interval WHERE id = $1",
		uuid.MustParse(recID), interval)
	if err != nil {
		t.Fatalf("backdate %s: %v", column, err)
	}
}

func getRecording(t *testing.T, recID string) db.ConferenceRecording {
	t.Helper()
	rec, err := testQueries.GetConferenceRecording(context.Background(), uuid.MustParse(recID))
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	return rec
}

func roomOf(confID string) string { return livekit.RoomName(uuid.MustParse(confID)) }

// TestRecordingWorkerClosesFinishedEgress: the ordinary ending. Everyone left,
// LiveKit ended the egress on its own, and nobody was polling — the row has to
// pick up the file's size and length, and the expiry that lets the sweeper come
// for it later.
func TestRecordingWorkerClosesFinishedEgress(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка с записью")
	recID, _ := plantRecording(t, confID, "active")
	attachEgress(t, recID, "eg-complete")

	api, fake := recordingWorker(t)
	fake.add("eg-complete", roomOf(confID), "EGRESS_COMPLETE", 4096, int64(90*time.Second))
	api.SweepConferenceRecordings(context.Background())

	rec := getRecording(t, recID)
	if rec.Status != "completed" {
		t.Fatalf("status = %q, want completed (error %q)", rec.Status, rec.Error)
	}
	if rec.SizeBytes != 4096 {
		t.Errorf("size = %d, want 4096", rec.SizeBytes)
	}
	// 90 seconds arrived as 90000000000: reading LiveKit's nanoseconds as
	// seconds would put this recording's length in the year 4870.
	if rec.DurationSec != 90 {
		t.Errorf("duration = %ds, want 90s", rec.DurationSec)
	}
	if rec.EndedAt == nil {
		t.Error("ended_at was not stamped")
	}
	// The conference default is 30 days; without an expiry the sweeper would
	// never come for the file and recording_ttl_days would stay a promise.
	if rec.ExpiresAt == nil {
		t.Fatal("expires_at is nil — the file would be kept forever")
	}
	if d := time.Until(*rec.ExpiresAt); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Errorf("expires in %v, want ~30 days", d)
	}
}

// TestRecordingWorkerLeavesRunningRecording: a recording in progress must
// survive every tick that passes under it. Closing one early would take the red
// dot off a call that is still being recorded.
func TestRecordingWorkerLeavesRunningRecording(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка в процессе")
	recID, path := plantRecording(t, confID, "active")
	attachEgress(t, recID, "eg-running")

	api, fake := recordingWorker(t)
	fake.add("eg-running", roomOf(confID), "EGRESS_ACTIVE", 0, 0)
	api.SweepConferenceRecordings(context.Background())

	if rec := getRecording(t, recID); rec.Status != "active" {
		t.Fatalf("status = %q, want active — a live recording was closed", rec.Status)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file of a running recording was removed: %v", err)
	}
}

// TestRecordingWorkerSalvagesForgottenEgress: livekit-server keeps finished
// egresses for a while and then forgets them, so a row that outlived the
// server's memory is not evidence of failure. If the mp4 is on disk with bytes
// in it, the meeting was recorded and must stay watchable.
func TestRecordingWorkerSalvagesForgottenEgress(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка после перезапуска SFU")
	recID, path := plantRecording(t, confID, "active")
	attachEgress(t, recID, "eg-forgotten")

	api, _ := recordingWorker(t) // the stub knows nothing about this egress
	api.SweepConferenceRecordings(context.Background())

	rec := getRecording(t, recID)
	if rec.Status != "completed" {
		t.Fatalf("status = %q, want completed — a recorded meeting was thrown away", rec.Status)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file was deleted: %v", err)
	}
	// plantRecording writes 14 bytes; the size has to come from the file,
	// because the server that could have reported it is the one that forgot.
	if rec.SizeBytes != 14 {
		t.Errorf("size = %d, want the file's own 14", rec.SizeBytes)
	}
}

// TestRecordingWorkerFailsForgottenEgressWithoutFile: same forgotten egress,
// but nothing was written. That is a recording somebody asked for and will
// never get, and the row is where they find out.
func TestRecordingWorkerFailsForgottenEgressWithoutFile(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка без файла")
	recID, path := plantRecording(t, confID, "active")
	attachEgress(t, recID, "eg-lost")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove planted file: %v", err)
	}

	api, _ := recordingWorker(t)
	api.SweepConferenceRecordings(context.Background())

	rec := getRecording(t, recID)
	if rec.Status != "failed" {
		t.Fatalf("status = %q, want failed", rec.Status)
	}
	if rec.Error == "" {
		t.Error("no reason was recorded — the answer would only exist in the container log")
	}
}

// TestRecordingWorkerAdoptsRunningEgress: the process died between claiming the
// row and attaching the job id. The egress is running regardless, so the row
// has to be repaired rather than failed — otherwise a headless Chrome keeps
// recording into a file nothing points at until the session limit stops it.
func TestRecordingWorkerAdoptsRunningEgress(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка с потерянным id")
	recID, _ := plantRecording(t, confID, "active")
	backdate(t, recID, "started_at", "5 minutes")

	api, fake := recordingWorker(t)
	fake.add("eg-orphan", roomOf(confID), "EGRESS_ACTIVE", 0, 0)
	api.SweepConferenceRecordings(context.Background())

	rec := getRecording(t, recID)
	if rec.Status != "active" {
		t.Fatalf("status = %q, want active — a running recording was failed", rec.Status)
	}
	if rec.EgressID != "eg-orphan" {
		t.Fatalf("egress_id = %q, want eg-orphan — the handle was not recovered", rec.EgressID)
	}
}

// TestRecordingWorkerFailsUnclaimedWithoutEgress: the same row, but the SFU is
// recording nothing on that room. Leaving it active would be worse than failing
// it: the one-active index would refuse every future recording of this
// conference forever.
func TestRecordingWorkerFailsUnclaimedWithoutEgress(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка, не начавшаяся")
	recID, _ := plantRecording(t, confID, "active")
	backdate(t, recID, "started_at", "5 minutes")

	api, fake := recordingWorker(t)
	// A finished egress on the same room — a previous recording. Adopting it
	// would hand this row somebody else's file.
	fake.add("eg-old", roomOf(confID), "EGRESS_COMPLETE", 1024, int64(time.Minute))
	api.SweepConferenceRecordings(context.Background())

	rec := getRecording(t, recID)
	if rec.Status != "failed" {
		t.Fatalf("status = %q, want failed", rec.Status)
	}
	if rec.EgressID != "" {
		t.Fatalf("adopted %q — a finished egress of an earlier recording", rec.EgressID)
	}

	// And the conference can be recorded again, which is the point of closing it.
	if _, err := testQueries.CreateConferenceRecording(context.Background(), db.CreateConferenceRecordingParams{
		ConferenceID: uuid.MustParse(confID), FilePath: "/tmp/next.mp4", FileName: "next.mp4",
	}); err != nil {
		t.Fatalf("the conference is still blocked for recording: %v", err)
	}
}

// TestRecordingWorkerWaitsOutTheClaimGrace: a row with no egress id is normally
// microseconds old — the start handler is between two statements. A tick that
// lands in that gap must not conclude the recording is abandoned.
func TestRecordingWorkerWaitsOutTheClaimGrace(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка, только что начатая")
	recID, _ := plantRecording(t, confID, "active")

	api, _ := recordingWorker(t)
	api.SweepConferenceRecordings(context.Background())

	if rec := getRecording(t, recID); rec.Status != "active" {
		t.Fatalf("status = %q, want active — a recording being started was failed", rec.Status)
	}
}

// TestRecordingWorkerCutsOffStuckEgress: EgressInfo.Done answers only on the
// statuses we know, so an unrecognised one reads as "still running" — which
// without a wall-clock backstop is forever. The stop matters as much as the
// row: it is the last moment anything holds that worker's id.
func TestRecordingWorkerCutsOffStuckEgress(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка, зависшая навсегда")
	recID, _ := plantRecording(t, confID, "active")
	attachEgress(t, recID, "eg-stuck")
	backdate(t, recID, "started_at", "4 hours")

	api, fake := recordingWorker(t)
	fake.add("eg-stuck", roomOf(confID), "EGRESS_SOMETHING_NEW", 0, 0)
	api.SweepConferenceRecordings(context.Background())

	if rec := getRecording(t, recID); rec.Status != "failed" {
		t.Fatalf("status = %q, want failed after the backstop", rec.Status)
	}
	if n := fake.stopCount("eg-stuck"); n != 1 {
		t.Fatalf("StopEgress called %d times, want 1 — the worker was abandoned mid-recording", n)
	}
}

// TestRecordingSweeperDeletesExpired: recording_ttl_days has been a promise
// since 0069 with nothing behind it. The file goes first and the row second —
// the other order leaves the largest file this application writes with nothing
// left that knows its path.
func TestRecordingSweeperDeletesExpired(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка на удаление по сроку")
	recID, path := plantRecording(t, confID, "completed")
	backdate(t, recID, "expires_at", "1 hour")

	api, _ := recordingWorker(t)
	api.SweepConferenceRecordings(context.Background())

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the mp4 is still on disk: %v", err)
	}
	if _, err := testQueries.GetConferenceRecording(context.Background(), uuid.MustParse(recID)); err == nil {
		t.Fatal("the row outlived the file it describes")
	}
}

// TestRecordingSweeperSurvivesMissingFile: an operator who cleared the volume
// by hand must not wedge the sweeper on the same batch forever — the rows would
// then outlive every recording they describe.
func TestRecordingSweeperSurvivesMissingFile(t *testing.T) {
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка без файла на диске")
	recID, path := plantRecording(t, confID, "completed")
	backdate(t, recID, "expires_at", "1 hour")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove planted file: %v", err)
	}

	api, _ := recordingWorker(t)
	api.SweepConferenceRecordings(context.Background())

	if _, err := testQueries.GetConferenceRecording(context.Background(), uuid.MustParse(recID)); err == nil {
		t.Fatal("a row whose file was already gone was kept")
	}
}
