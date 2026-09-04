package confroom

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// The recording indicator (#2877) is the one piece of room state whose *absence*
// is a privacy failure rather than a cosmetic one, and whose source of truth
// lives outside this package — in a database row that every arriving socket
// re-reads. These tests cover the seam that creates: the same recording is set
// repeatedly, by different callers, in an order nobody controls.

func recRoom(t *testing.T) (*Room, *Participant) {
	t.Helper()
	room := newRoom(uuid.New(), nil)
	p := NewParticipant(uuid.New(), "Аня", RoleHost)
	room.join(p, false, time.Now())
	drain(p)
	return room, p
}

func TestRecordingAppearsInSnapshot(t *testing.T) {
	room, p := recRoom(t)
	id := uuid.NewString()
	started := time.Now().Truncate(time.Second)

	room.SetRecording(&RecordingView{ID: id, StartedAt: started, StartedBy: "Аня"})

	msg := lastOfType(p, TypeState)
	if msg == nil {
		t.Fatal("starting a recording did not broadcast a state frame")
	}
	rec, ok := msg["recording"].(map[string]any)
	if !ok {
		t.Fatalf("state frame carried no recording: %v", msg)
	}
	if rec["id"] != id {
		t.Errorf("recording id = %v, want %s", rec["id"], id)
	}
	// The name is the whole point of the indicator: "somebody is recording" is
	// not an answer anyone accepts.
	if rec["started_by"] != "Аня" {
		t.Errorf("started_by = %v, want Аня", rec["started_by"])
	}
}

func TestRecordingSeedIsIdempotent(t *testing.T) {
	room, p := recRoom(t)
	view := RecordingView{ID: uuid.NewString(), StartedAt: time.Now(), StartedBy: "Аня"}
	room.SetRecording(&view)
	drain(p)

	// Every socket that joins re-seeds the same recording from the database. If
	// that repainted the room, a busy call would broadcast a full snapshot to
	// everyone each time somebody opened a second tab.
	room.SetRecording(&view)
	if frames := drain(p); len(frames) != 0 {
		t.Fatalf("re-seeding an unchanged recording broadcast %d frame(s)", len(frames))
	}
}

func TestClearRecordingTurnsTheDotOff(t *testing.T) {
	room, p := recRoom(t)
	id := uuid.NewString()
	room.SetRecording(&RecordingView{ID: id, StartedAt: time.Now(), StartedBy: "Аня"})
	drain(p)

	room.ClearRecording(id)

	msg := lastOfType(p, TypeState)
	if msg == nil {
		t.Fatal("clearing a recording did not broadcast a state frame")
	}
	if msg["recording"] != nil {
		t.Errorf("recording = %v after clear, want null", msg["recording"])
	}
}

func TestClearRecordingIgnoresAnotherRecording(t *testing.T) {
	room, p := recRoom(t)
	current := uuid.NewString()
	room.SetRecording(&RecordingView{ID: current, StartedAt: time.Now(), StartedBy: "Аня"})
	drain(p)

	// A finalisation that arrives late — the poller and the host's stop button
	// race by design — must not switch off a recording that started since.
	room.ClearRecording(uuid.NewString())

	if frames := drain(p); len(frames) != 0 {
		t.Fatalf("clearing an unrelated recording broadcast %d frame(s)", len(frames))
	}
	if got := room.State(time.Now()).Recording; got == nil || got.ID != current {
		t.Fatalf("recording = %v, want the one that is still running", got)
	}
}

func TestStaleSeedCannotResurrectTheDot(t *testing.T) {
	room, p := recRoom(t)
	id := uuid.NewString()
	view := RecordingView{ID: id, StartedAt: time.Now(), StartedBy: "Аня"}
	room.SetRecording(&view)
	room.ClearRecording(id)
	drain(p)

	// A socket whose database read happened a moment before the stop committed
	// arrives holding a row that says "active". Honouring it would put the dot
	// back on a call nobody is recording — and nothing would ever take it off,
	// because the stop has already been and gone.
	room.SetRecording(&view)

	if frames := drain(p); len(frames) != 0 {
		t.Fatalf("a stale seed broadcast %d frame(s)", len(frames))
	}
	if got := room.State(time.Now()).Recording; got != nil {
		t.Fatalf("recording = %v after a stale seed, want none", got)
	}
}

func TestSnapshotRecordingIsACopy(t *testing.T) {
	room, _ := recRoom(t)
	id := uuid.NewString()
	room.SetRecording(&RecordingView{ID: id, StartedAt: time.Now(), StartedBy: "Аня"})

	snap := room.State(time.Now())
	snap.Recording.StartedBy = "кто-то другой"

	if got := room.State(time.Now()).Recording; got.StartedBy != "Аня" {
		t.Fatalf("started_by = %q — a caller edited the room's own state through a snapshot", got.StartedBy)
	}
}

func TestSetRecordingIgnoresNilAndEmpty(t *testing.T) {
	room, p := recRoom(t)
	room.SetRecording(nil)
	// An empty id would be a recording ClearRecording could never name, so the
	// dot would be stuck on until the room emptied.
	room.SetRecording(&RecordingView{StartedAt: time.Now(), StartedBy: "Аня"})

	if frames := drain(p); len(frames) != 0 {
		t.Fatalf("an unusable recording broadcast %d frame(s)", len(frames))
	}
	if got := room.State(time.Now()).Recording; got != nil {
		t.Fatalf("recording = %v, want none", got)
	}
}
