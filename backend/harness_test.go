// Integration-test harness: boots the full HTTP surface (newRouter) over a
// real Postgres (tessera_test by default) and exposes small request helpers.
// Isolation is by data scoping — each test registers its own user and creates
// its own workspace — so tests can run in parallel without truncation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"tessera/config"
	"tessera/handlers"
	"tessera/internal/database"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/realtime"
	"tessera/migrations"
)

var (
	testServer  *httptest.Server
	testPool    *pgxpool.Pool
	testQueries *db.Queries
	testAPI     *handlers.API // spawn Run*Worker(ctx) again to drain an outbox now (workers drain once at startup)
	userSeq     atomic.Int64
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable"
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("harness: load migrations: %v", err)
	}
	mig, err := migrate.NewWithSourceInstance("iofs", src, toPgx5URL(dbURL))
	if err != nil {
		log.Printf("harness: no test database reachable (%v) — skipping integration tests", err)
		os.Exit(0)
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("harness: migrate up: %v", err)
	}
	_, _ = mig.Close()

	ctx := context.Background()
	testPool = database.Connect(ctx, dbURL)
	testQueries = db.New(testPool)

	// One truncate per run for repeatable local numbers; individual tests rely
	// on data scoping, not cleanup.
	if err := truncateAll(ctx, testPool); err != nil {
		log.Fatalf("harness: truncate: %v", err)
	}

	hub := realtime.NewHub()
	go hub.Run()

	uploadDir, err := os.MkdirTemp("", "tessera-uploads-*")
	if err != nil {
		log.Fatalf("harness: %v", err)
	}
	defer os.RemoveAll(uploadDir)

	cfg := &config.Config{
		JWTSecret:     "integration-test-secret-min32chars!!",
		UploadDir:     uploadDir,
		EncryptionKey: "integration-test-encryption-key",
		PublicURL:     "http://tessera.test",
		CORSOrigin:    "*",
	}
	router, rh := newRouter(cfg, testQueries, testPool, hub, mail.New(mail.Config{}))
	testAPI = rh
	testServer = httptest.NewServer(router)
	defer testServer.Close()

	// Run the background workers like production does — they drain outboxes
	// (GitLab write-back, notifications) created by the flow tests.
	rh.RegisterBackgroundWorkers()
	workerCtx, stopWorkers := context.WithCancel(ctx)
	go rh.RunSyncWorker(workerCtx)
	go rh.RunGitlabWriteBackWorker(workerCtx)
	go rh.RunNotificationWorker(workerCtx)
	go rh.RunNotificationScanner(workerCtx)
	go rh.RunRecurrenceWorker(workerCtx)

	code := m.Run()
	stopWorkers()
	testPool.Close()
	os.Exit(code)
}

// toPgx5URL mirrors cmd/migrate's scheme rewrite for golang-migrate's pgx/v5
// driver.
func toPgx5URL(u string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(u, p) {
			return "pgx5://" + strings.TrimPrefix(u, p)
		}
	}
	return u
}

func truncateAll(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `DO $$
DECLARE t text;
BEGIN
  SELECT string_agg(quote_ident(tablename), ', ') INTO t
  FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations';
  IF t IS NOT NULL THEN
    EXECUTE 'TRUNCATE ' || t || ' RESTART IDENTITY CASCADE';
  END IF;
END $$`)
	return err
}

// ── request helpers ──────────────────────────────────────────────────────────

// client is an authenticated API caller bound to one registered user.
type client struct {
	t     *testing.T
	token string
	// filled by signup
	UserID  string
	Email   string
	Refresh string
}

// resp is a decoded API response: status plus the raw body for flexible
// decoding into maps, slices or structs.
type resp struct {
	Status int
	Body   []byte
}

func (r resp) mapBody(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("decode object body (%d): %v\n%s", r.Status, err, r.Body)
	}
	return m
}

func (r resp) listBody(t *testing.T) []map[string]any {
	t.Helper()
	var l []map[string]any
	if err := json.Unmarshal(r.Body, &l); err != nil {
		t.Fatalf("decode array body (%d): %v\n%s", r.Status, err, r.Body)
	}
	return l
}

// doReq performs a JSON request with an optional bearer token.
func doReq(t *testing.T, token, method, path string, body any) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, testServer.URL+"/api"+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp{Status: res.StatusCode, Body: data}
}

func (c *client) do(method, path string, body any) resp {
	return doReq(c.t, c.token, method, path, body)
}

// get/post/patch/put/del are one-line sugar over do.
func (c *client) get(path string) resp               { return c.do(http.MethodGet, path, nil) }
func (c *client) post(path string, body any) resp    { return c.do(http.MethodPost, path, body) }
func (c *client) patch(path string, body any) resp   { return c.do(http.MethodPatch, path, body) }
func (c *client) put(path string, body any) resp     { return c.do(http.MethodPut, path, body) }
func (c *client) del(path string) resp               { return c.do(http.MethodDelete, path, nil) }
func (c *client) expect(t *testing.T, r resp, want int) map[string]any {
	t.Helper()
	if r.Status != want {
		t.Fatalf("status %d, want %d\n%s", r.Status, want, r.Body)
	}
	if len(r.Body) == 0 || r.Body[0] != '{' {
		return nil
	}
	return r.mapBody(t)
}

// signup registers a fresh user (unique email) and returns an authenticated
// client for it.
func signup(t *testing.T) *client {
	t.Helper()
	n := userSeq.Add(1)
	email := fmt.Sprintf("it-user-%d-%s@test.local", n, strings.ToLower(t.Name()[:min(20, len(t.Name()))]))
	email = strings.Map(func(r rune) rune {
		if r == '/' || r == '#' || r == ' ' {
			return '-'
		}
		return r
	}, email)
	r := doReq(t, "", http.MethodPost, "/auth/register", map[string]any{
		"email": email, "name": fmt.Sprintf("IT User %d", n), "password": "password-123",
	})
	if r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("register: status %d\n%s", r.Status, r.Body)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("register decode: %v\n%s", err, r.Body)
	}
	return &client{t: t, token: out.AccessToken, Refresh: out.RefreshToken, UserID: out.User.ID, Email: email}
}

// ── fixture builders (workspace → … → task chain) ───────────────────────────

func mkWorkspace(t *testing.T, c *client, name string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces", map[string]any{"name": name}), http.StatusCreated)
	return m["id"].(string)
}

func mkGroup(t *testing.T, c *client, wsID, name string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces/"+wsID+"/groups", map[string]any{"name": name}), http.StatusCreated)
	return m["id"].(string)
}

func mkProject(t *testing.T, c *client, wsID, groupID, name string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces/"+wsID+"/projects",
		map[string]any{"name": name, "group_id": groupID}), http.StatusCreated)
	return m["id"].(string)
}

func mkBoard(t *testing.T, c *client, projectID, name string) string {
	t.Helper()
	m := c.expect(t, c.post("/projects/"+projectID+"/boards", map[string]any{"name": name}), http.StatusCreated)
	return m["id"].(string)
}

// mkStack builds workspace→group→project→board in one call and returns the ids.
type stack struct {
	WS, Group, Project, Board string
	Columns                   []map[string]any
}

func mkStack(t *testing.T, c *client) stack {
	t.Helper()
	ws := mkWorkspace(t, c, "WS "+t.Name())
	g := mkGroup(t, c, ws, "Группа")
	p := mkProject(t, c, ws, g, "Проект "+t.Name())
	b := mkBoard(t, c, p, "Доска")
	cols := c.get("/boards/" + b + "/columns").listBody(t)
	return stack{WS: ws, Group: g, Project: p, Board: b, Columns: cols}
}

func mkTask(t *testing.T, c *client, boardID, columnID, title string) map[string]any {
	t.Helper()
	return c.expect(t, c.post("/boards/"+boardID+"/tasks",
		map[string]any{"title": title, "column_id": columnID}), http.StatusCreated)
}

// col returns the id of the n-th column of a fixture stack.
func (s stack) col(t *testing.T, n int) string {
	t.Helper()
	if n >= len(s.Columns) {
		t.Fatalf("stack has %d columns, want index %d", len(s.Columns), n)
	}
	return s.Columns[n]["id"].(string)
}

// uploadFile posts a multipart file to path under field name.
func uploadFile(t *testing.T, c *client, path, field, filename string, content []byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api"+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data}
}
