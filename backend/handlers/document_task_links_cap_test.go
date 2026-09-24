package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/middleware"
)

// The bounds on document↔task links (#2932).
//
// The pair is an upsert, which reads as though it were self-limiting — the same
// task cannot be linked to the same block twice. It is not: block_id is the third
// column of that key, it arrives from the client, and nothing trimmed it or
// counted the rows. So a fresh anchor always inserted, and the table had no
// ceiling at all.
//
// Driven over a scripted DBTX rather than a database: what is being pinned is
// which statements the handler runs and which it refuses to, and a real table
// would need five hundred rows to say the same thing.

// linkScript answers sqlc queries by name for the link handler. Separate from
// scriptedDB (conference_recording_claim_test.go) because it has to record the
// inserts rather than the deletes, and because these queries return scalars —
// but it reuses that file's structRow/errRow, so the scanning is not duplicated.
type linkScript struct {
	mu       sync.Mutex
	rows     map[string]any // query name → row to scan
	inserted []db.CreateDocumentTaskLinkParams
}

func (s *linkScript) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	_, rest, ok := strings.Cut(sql, "-- name: ")
	if !ok {
		return errRow{fmt.Errorf("linkScript: statement without a name header")}
	}
	name, _, _ := strings.Cut(rest, " ")
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "CreateDocumentTaskLink" {
		arg := db.CreateDocumentTaskLinkParams{Quote: fmt.Sprint(args[3])}
		arg.DocumentID, _ = args[0].(uuid.UUID)
		arg.TaskID, _ = args[1].(uuid.UUID)
		arg.BlockID, _ = args[2].(string)
		s.inserted = append(s.inserted, arg)
	}
	row, ok := s.rows[name]
	if !ok {
		return errRow{fmt.Errorf("linkScript: unscripted query %q", name)}
	}
	if v, ok := row.(structish); ok {
		return structRow{v.value}
	}
	return valueRow{row}
}

func (s *linkScript) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *linkScript) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("linkScript: unscripted Query")
}

func (s *linkScript) inserts() []db.CreateDocumentTaskLinkParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]db.CreateDocumentTaskLinkParams(nil), s.inserted...)
}

// structish marks a scripted answer as a whole row to be spread across the scan
// destinations, telling it apart from the single values count(*) and EXISTS
// return.
type structish struct{ value any }

// valueRow is a pgx.Row holding one column — what a count or an EXISTS scans.
type valueRow struct{ v any }

func (r valueRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("valueRow: %d destinations for a single column", len(dest))
	}
	switch d := dest[0].(type) {
	case *int64:
		*d, _ = r.v.(int64)
	case *bool:
		*d, _ = r.v.(bool)
	case *uuid.UUID:
		*d, _ = r.v.(uuid.UUID)
	default:
		return fmt.Errorf("valueRow: cannot scan into %T", dest[0])
	}
	return nil
}

// docLinkFixture wires a member linking a task to a document they can both see.
type docLinkFixture struct {
	script *linkScript
	taskID uuid.UUID
	call   func(blockID string) *httptest.ResponseRecorder
}

// newDocLinkFixture builds the fixture with the table already holding `count`
// links and the asked-for pair present or not, which is the whole of the state
// the cap consults.
func newDocLinkFixture(t *testing.T, count int64, exists bool) *docLinkFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	uid, wsID, docID, taskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	script := &linkScript{rows: map[string]any{
		"GetDocument": structish{db.Document{
			ID: docID, WorkspaceID: wsID, AuthorID: &uid, Title: "спец",
			Content: []byte(`{"type":"doc","content":[]}`), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
		"WorkspaceIDForTask":     wsID,
		"CountDocumentTaskLinks": count,
		"DocumentTaskLinkExists": exists,
		"CreateDocumentTaskLink": structish{db.DocumentTaskLink{
			ID: uuid.New(), DocumentID: docID, TaskID: taskID, CreatedBy: &uid, CreatedAt: time.Now(),
		}},
	}}

	api := NewAPI(db.New(script), nil, nil, t.TempDir(), "handlers-unit-test-encryption-key",
		mail.New(mail.Config{}), "http://test", "")

	return &docLinkFixture{
		script: script,
		taskID: taskID,
		call: func(blockID string) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body := fmt.Sprintf(`{"task_id":%q,"block_id":%q,"quote":"пункт 4.2"}`, taskID, blockID)
			c.Request = httptest.NewRequest(http.MethodPost, "/documents/x/links", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: docID.String()}}
			c.Set(middleware.ContextUserID, uid)
			c.Set("membership:"+wsID.String(), membershipResult{role: "member", ok: true})
			api.CreateDocumentTaskLink(c)
			return w
		},
	}
}

// TestDocumentLinkCapRefusesAFreshAnchor: at the ceiling, an anchor the document
// does not already carry is the call that would add a row, and it is the only one
// the cap is about.
func TestDocumentLinkCapRefusesAFreshAnchor(t *testing.T) {
	f := newDocLinkFixture(t, maxDocTaskLinks, false)

	if code := f.call("a1b2c3d4e5f60718").Code; code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 at the link ceiling", code)
	}
	if got := f.script.inserts(); len(got) != 0 {
		t.Errorf("inserts = %v, want none: the row was written past the cap", got)
	}
}

// TestDocumentLinkCapStillRefreshesAnExistingLink is the control that says the
// cap bounds the table rather than freezing the panel. Re-linking is how a quote
// is brought back in step with a paragraph that was edited, and it adds no row.
func TestDocumentLinkCapStillRefreshesAnExistingLink(t *testing.T) {
	f := newDocLinkFixture(t, maxDocTaskLinks, true)

	if code := f.call("a1b2c3d4e5f60718").Code; code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for a link that is already there", code)
	}
	if got := f.script.inserts(); len(got) != 1 {
		t.Errorf("inserts = %v, want one: the quote was not refreshed", got)
	}
}

// TestDocumentLinkBelowTheCapIsTaken is the other control: one short of the
// ceiling a fresh anchor is an ordinary link.
func TestDocumentLinkBelowTheCapIsTaken(t *testing.T) {
	f := newDocLinkFixture(t, maxDocTaskLinks-1, false)

	if code := f.call("a1b2c3d4e5f60718").Code; code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 below the ceiling", code)
	}
	if got := f.script.inserts(); len(got) != 1 {
		t.Errorf("inserts = %v, want one", got)
	}
}

// TestDocumentLinkRefusesAnOversizedAnchor pins the other half: without a bound
// on the anchor itself the cap above only rations rows, while each one could
// still carry a page of text in the column that decides the key.
//
// Measured in runes, so the Cyrillic control below is refused for its length and
// not for its encoding — a byte count would refuse an anchor half the size.
func TestDocumentLinkRefusesAnOversizedAnchor(t *testing.T) {
	f := newDocLinkFixture(t, 0, false)

	if code := f.call(strings.Repeat("я", maxDocLinkBlockID+1)).Code; code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an anchor past the bound", code)
	}
	if got := f.script.inserts(); len(got) != 0 {
		t.Errorf("inserts = %v, want none", got)
	}
}

// TestDocumentLinkAcceptsAnAnchorAtTheBound is the boundary control: exactly
// maxDocLinkBlockID runes of Cyrillic is 128 bytes and still a legal anchor.
func TestDocumentLinkAcceptsAnAnchorAtTheBound(t *testing.T) {
	f := newDocLinkFixture(t, 0, false)

	anchor := strings.Repeat("я", maxDocLinkBlockID)
	if code := f.call(anchor).Code; code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for an anchor exactly at the bound", code)
	}
	got := f.script.inserts()
	if len(got) != 1 || got[0].BlockID != anchor {
		t.Errorf("inserts = %v, want the anchor stored whole", got)
	}
}
