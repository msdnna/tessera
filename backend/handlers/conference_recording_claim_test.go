package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/db"
	"tessera/internal/livekit"
	"tessera/internal/mail"
	"tessera/middleware"
)

// The rollback that follows a recording whose row could not be given its job id
// (#2932).
//
// StartConferenceRecording claims the row before starting the egress, and the
// unique partial index on (conference_id) WHERE status='active' is what makes
// that claim a mutex. The price of that order is that every failure after the
// insert has to hand the claim back — otherwise our own bookkeeping refuses the
// next press of the button for a recording that never happened. Two of those
// failures are pinned here: the egress that would not start, and the statement
// that could not attach its id.
//
// Neither is reachable through a harness with a working database, which is why
// these drive the handler over a scripted DBTX: the whole point is what happens
// when one specific statement fails.

// scriptedDB is a db.DBTX that answers by sqlc query name. Every generated
// constant keeps its `-- name: X :one` header, so matching on that is stable
// against SQL edits in a way matching on the statement text is not.
//
// Rows are handed over as the struct sqlc scans into: dest pointers arrive in
// the struct's own field order for a :one query that selects the whole row, so
// filling them by reflection needs no column list here to drift out of date.
type scriptedDB struct {
	mu      sync.Mutex
	rows    map[string]any   // query name → row to scan, or error to return
	errs    map[string]error // query name → failure
	deleted []uuid.UUID      // ids passed to DeleteConferenceRecording
}

func (s *scriptedDB) queryName(sql string) string {
	_, rest, ok := strings.Cut(sql, "-- name: ")
	if !ok {
		return ""
	}
	name, _, _ := strings.Cut(rest, " ")
	return name
}

func (s *scriptedDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	name := s.queryName(sql)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.errs[name]; ok {
		return errRow{err}
	}
	row, ok := s.rows[name]
	if !ok {
		return errRow{fmt.Errorf("scriptedDB: unscripted query %q", name)}
	}
	return structRow{row}
}

func (s *scriptedDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	name := s.queryName(sql)
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "DeleteConferenceRecording" {
		id, _ := args[0].(uuid.UUID)
		s.deleted = append(s.deleted, id)
	}
	if err, ok := s.errs[name]; ok {
		return pgconn.CommandTag{}, err
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *scriptedDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("scriptedDB: unscripted Query %q", s.queryName(sql))
}

func (s *scriptedDB) claimsReleased() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uuid.UUID(nil), s.deleted...)
}

// errRow is a pgx.Row that only ever fails — a statement that errored, or one
// this script has no answer for.
type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// structRow copies a row struct into sqlc's scan destinations positionally.
type structRow struct{ src any }

func (r structRow) Scan(dest ...any) error {
	v := reflect.ValueOf(r.src)
	if v.NumField() != len(dest) {
		return fmt.Errorf("structRow: %s has %d fields, %d scanned",
			v.Type(), v.NumField(), len(dest))
	}
	for i, d := range dest {
		reflect.ValueOf(d).Elem().Set(v.Field(i))
	}
	return nil
}

// startingEgress is a stub livekit-server that starts one job and remembers
// every stop asked of it.
type startingEgress struct {
	mu      sync.Mutex
	stopped []string
}

func (f *startingEgress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.Header().Set("Content-Type", "application/json")
	// Proto field names in the replies, as livekit-server renders them.
	item := map[string]any{"egress_id": "eg-claim-test", "room_name": "room"}
	switch r.URL.Path {
	case "/twirp/livekit.Egress/StartRoomCompositeEgress":
		item["status"] = "EGRESS_STARTING"
		_ = json.NewEncoder(w).Encode(item)
	case "/twirp/livekit.Egress/StopEgress":
		f.mu.Lock()
		id, _ := req["egressId"].(string)
		f.stopped = append(f.stopped, id)
		f.mu.Unlock()
		item["status"] = "EGRESS_ENDING"
		_ = json.NewEncoder(w).Encode(item)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *startingEgress) stops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stopped...)
}

// recordingStartFixture wires a live conference, a manager calling the handler,
// and the scripted database the two failure paths need.
type recordingStartFixture struct {
	api    *API
	script *scriptedDB
	sfu    *startingEgress
	claim  uuid.UUID // the row CreateConferenceRecording hands back
	call   func() *httptest.ResponseRecorder
}

func newRecordingStartFixture(t *testing.T) *recordingStartFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	sfu := &startingEgress{}
	srv := httptest.NewServer(sfu)
	t.Cleanup(srv.Close)

	uid, wsID, confID, claimID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	script := &scriptedDB{
		rows: map[string]any{
			"GetConference": db.Conference{
				ID: confID, WorkspaceID: wsID, CreatedBy: &uid,
				Title: "standup", Status: "live", RecordingTtlDays: 30,
			},
			"CreateConferenceRecording": db.ConferenceRecording{
				ID: claimID, ConferenceID: confID, Status: "active",
				FileName: "rec.mp4", StartedAt: time.Now(), StartedBy: &uid,
			},
		},
		errs: map[string]error{},
	}

	api := NewAPI(db.New(script), nil, nil, t.TempDir(), "handlers-unit-test-encryption-key",
		mail.New(mail.Config{}), "http://test", "")
	api.WireLiveKit(livekit.New(livekit.Config{
		URL: srv.URL, PublicURL: "wss://test", APIKey: "devkey",
		APISecret: "devsecret-at-least-32-chars-long!!",
	}))

	return &recordingStartFixture{
		api: api, script: script, sfu: sfu, claim: claimID,
		call: func() *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/conferences/x/recording/start", nil)
			c.Params = gin.Params{{Key: "id", Value: confID.String()}}
			c.Set(middleware.ContextUserID, uid)
			// The caller is the conference's creator, so authorization never
			// reaches the membership query; seeding the memo keeps it that way
			// even if that shortcut is reordered.
			c.Set("membership:"+wsID.String(), membershipResult{role: "owner", ok: true})
			api.StartConferenceRecording(c)
			return w
		},
	}
}

// TestStartReleasesTheClaimWhenTheJobIDCannotBeStored: the row was inserted, the
// egress is running, and the statement that would have written its id failed.
//
// The claim has to go back, and it has to go back by the id of the row that was
// actually claimed — the sqlc call that failed returns a zero-valued row, so a
// rollback reading its id deletes uuid.Nil and leaves the real row active. That
// row is what the unique partial index consults, so the conference cannot be
// recorded again until the background worker times the claim out minutes later.
func TestStartReleasesTheClaimWhenTheJobIDCannotBeStored(t *testing.T) {
	f := newRecordingStartFixture(t)
	f.script.errs["SetConferenceRecordingEgress"] = errors.New("connection reset")

	if code := f.call().Code; code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a failed write", code)
	}
	if got := f.script.claimsReleased(); len(got) != 1 || got[0] != f.claim {
		t.Errorf("released claims = %v, want [%v]: the active row still blocks the next recording",
			got, f.claim)
	}
	// The worker is already running and the row that named it is gone, so it has
	// to be stopped here or it writes into the volume until the session limit.
	if got := f.sfu.stops(); len(got) != 1 || got[0] != "eg-claim-test" {
		t.Errorf("stopped egresses = %v, want [eg-claim-test]", got)
	}
}

// TestStartReleasesTheClaimWhenTheEgressWillNotStart pins the sibling path,
// which reads rec.ID before anything has overwritten it — the control that says
// the assertion above is about the clobbered row and not about rollback in
// general.
func TestStartReleasesTheClaimWhenTheEgressWillNotStart(t *testing.T) {
	f := newRecordingStartFixture(t)
	f.api.WireLiveKit(livekit.New(livekit.Config{
		URL: "http://127.0.0.1:1", PublicURL: "wss://test", APIKey: "devkey",
		APISecret: "devsecret-at-least-32-chars-long!!",
	}))

	if code := f.call().Code; code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 for an SFU that refused", code)
	}
	if got := f.script.claimsReleased(); len(got) != 1 || got[0] != f.claim {
		t.Errorf("released claims = %v, want [%v]", got, f.claim)
	}
}
