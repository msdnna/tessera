//go:build e2e

// Package e2e is the black-box end-to-end suite (#2709). It builds the real
// binaries and runs them as subprocesses against a throwaway database, over a
// real TCP socket.
//
// It deliberately does NOT re-test the API surface: backend/harness_test.go
// already boots the whole newRouter() over a real Postgres and covers the
// business flows. What it covers is everything that harness cannot reach by
// construction — main() itself, the production config guards (they call
// log.Fatal, which is untestable in-process), migration rollback, SIGTERM
// draining, the refresh cookie over a real Host, and the cmd/ binaries that
// ship inside the image.
//
// Build-tagged so `make test-backend` neither slows down nor needs a binary.
//
//	make test-e2e-backend                    # the suite
//	make test-e2e-backend-docker             # + the image tier (E2E_DOCKER=1)
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// defaultDBURL points at the same box and credentials as the in-process
	// harness. Only the database *name* is replaced: every run gets a throwaway
	// database, because the migration test rolls the schema back and would
	// otherwise pull it out from under a parallel `make test-backend`. The
	// production `tessera` database is never touched.
	defaultDBURL = "postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable"

	testJWTSecret = "e2e-jwt-secret-at-least-32-chars!!!!"
	testEncKey    = "e2e-encryption-key-at-least-16"

	// bootTimeout bounds "binary started → /api/health answers". Generous: on a
	// cold CI runner the first connection also brings up the pgx pool.
	bootTimeout = 45 * time.Second
)

var (
	backendDir string // absolute path of backend/, the build context
	binServer  string // built ./           (the API server)
	binMigrate string // built ./cmd/migrate
	binToken   string // built ./cmd/token

	adminDBURL  string // a reachable database, used to CREATE/DROP the throwaway ones
	serverDBURL string // the throwaway database the servers run against
	runID       string
	subprocDir  string // cwd of every subprocess: empty, so godotenv.Load finds no .env
	uploadDir   string

	userSeq atomic.Int64
)

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

// runSuite is TestMain's body as a function so its defers (drop the database,
// remove the build dir) still run — os.Exit in TestMain would skip them.
func runSuite(m *testing.M) int {
	abs, err := filepath.Abs("..")
	if err != nil {
		log.Printf("e2e: resolve backend dir: %v", err)
		return 1
	}
	backendDir = abs

	adminDBURL = os.Getenv("TEST_DATABASE_URL")
	if adminDBURL == "" {
		adminDBURL = defaultDBURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Mirror the in-process harness: an unreachable database is a skip, not a
	// red run — the suite has to be a no-op on a box with no Postgres.
	if err := pingDB(ctx, adminDBURL); err != nil {
		log.Printf("e2e: no database reachable at %s (%v) — skipping the e2e suite", redactDBURL(adminDBURL), err)
		return 0
	}

	runID = fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano()%1_000_000)

	work, err := os.MkdirTemp("", "tessera-e2e-*")
	if err != nil {
		log.Printf("e2e: temp dir: %v", err)
		return 1
	}
	defer os.RemoveAll(work)

	// Subprocesses run here rather than in backend/: main() and both CLIs call
	// godotenv.Load(), and a developer's backend/.env would silently override
	// the environment a test is trying to assert on.
	subprocDir = filepath.Join(work, "cwd")
	uploadDir = filepath.Join(work, "uploads")
	for _, d := range []string{subprocDir, uploadDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Printf("e2e: mkdir %s: %v", d, err)
			return 1
		}
	}

	binServer = filepath.Join(work, "tessera")
	binMigrate = filepath.Join(work, "migrate")
	binToken = filepath.Join(work, "token")
	for _, b := range []struct{ out, pkg string }{
		{binServer, "."},
		{binMigrate, "./cmd/migrate"},
		{binToken, "./cmd/token"},
	} {
		if err := buildBinary(b.out, b.pkg); err != nil {
			log.Printf("e2e: build %s: %v", b.pkg, err)
			return 1
		}
	}

	dbName := "tessera_e2e_" + runID
	if err := createDatabase(ctx, dbName); err != nil {
		log.Printf("e2e: create database %s: %v", dbName, err)
		return 1
	}
	defer dropDatabase(dbName)
	serverDBURL = withDBName(adminDBURL, dbName)

	if out, err := runMigrate(serverDBURL); err != nil {
		log.Printf("e2e: migrate the throwaway database: %v\n%s", err, out)
		return 1
	}

	return m.Run()
}

// ── toolchain, build, database plumbing ─────────────────────────────────────

// goTool picks the Go that must do the building. `go test` doesn't hand the test
// binary its own toolchain path, and on this box the pinned toolchain is the
// versioned wrapper (go1.25.9) while plain `go` may be older — building with
// that one fails on the toolchain directive. So: an explicit E2E_GO wins (the
// Makefile passes it), otherwise try the wrapper named after the version this
// binary was compiled with, otherwise plain `go` (the CI layout, where setup-go
// installs the pinned version as `go`).
func goTool() string {
	if v := os.Getenv("E2E_GO"); v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.GoVersion != "" {
		if p, err := exec.LookPath(bi.GoVersion); err == nil {
			return p
		}
	}
	return "go"
}

func buildBinary(out, pkg string) error {
	cmd := exec.Command(goTool(), "build", "-o", out, pkg)
	cmd.Dir = backendDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, b)
	}
	return nil
}

func pingDB(ctx context.Context, dsn string) error {
	var err error
	// One retry: a just-started Postgres container can refuse the first dial
	// even after its healthcheck flips.
	for range 2 {
		var conn *pgx.Conn
		conn, err = pgx.Connect(ctx, dsn)
		if err == nil {
			err = conn.Ping(ctx)
			_ = conn.Close(ctx)
			if err == nil {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return err
}

// createDatabase makes a throwaway database. CREATE DATABASE cannot run inside a
// transaction, so it goes over a plain connection rather than a pool.
func createDatabase(ctx context.Context, name string) error {
	conn, err := pgx.Connect(ctx, adminDBURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	return err
}

// dropDatabase removes a throwaway database. WITH (FORCE) so a leaked
// connection from a subprocess that outlived its test can't wedge the drop.
func dropDatabase(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, adminDBURL)
	if err != nil {
		log.Printf("e2e: drop %s: connect: %v", name, err)
		return
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		log.Printf("e2e: drop %s: %v", name, err)
	}
}

// withDBName swaps the database name in a postgres URL, keeping credentials,
// host and query parameters.
func withDBName(raw, name string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Path = "/" + name
	return u.String()
}

// redactDBURL hides the password before a DSN reaches a log line.
func redactDBURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable dsn)"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.String()
}

// runMigrate runs the built cmd/migrate against dbURL and returns its output.
func runMigrate(dbURL string, args ...string) (string, error) {
	cmd := exec.Command(binMigrate, args...)
	cmd.Dir = subprocDir
	cmd.Env = append(baseEnv(), "DATABASE_URL="+dbURL)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// ── the server under test ───────────────────────────────────────────────────

// baseEnv is a deliberately bare environment: the point of the suite is that the
// binary boots from what it is *given*, so the ambient DATABASE_URL/JWT_SECRET of
// the developer's shell must not leak in.
func baseEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	if h := os.Getenv("HOME"); h != "" {
		env = append(env, "HOME="+h)
	}
	return env
}

// serverEnv is the environment of a healthy e2e server, before per-test overrides.
func serverEnv(port string) map[string]string {
	return map[string]string{
		"DATABASE_URL":   serverDBURL,
		"PORT":           port,
		"JWT_SECRET":     testJWTSecret,
		"ENCRYPTION_KEY": testEncKey,
		"UPLOAD_DIR":     uploadDir,
		// Every test signs up from the same loopback address; a shared throttle
		// would make unrelated tests fail each other.
		"RATE_LIMIT_ENABLED": "false",
		// Well under `go test`'s own timeout, so a stuck drain surfaces as a
		// test failure with a message rather than a panic dump.
		"GRACEFUL_TIMEOUT": "10s",
		// Release mode: gin's debug banner is ~200 lines of route dump per
		// server, and these tests start a server each. LOG_LEVEL is left at its
		// default on purpose — slog.SetDefault routes the stdlib log through the
		// handler at INFO, so raising the threshold would hide main()'s own
		// lifecycle lines, which several tests assert on.
		"GIN_MODE": "release",
	}
}

type server struct {
	t      *testing.T
	cmd    *exec.Cmd
	Port   string
	URL    string // http://127.0.0.1:<port>
	stderr *syncBuffer

	done     chan struct{} // closed when the process has been reaped
	waitErr  error         // valid after done
	stopOnce sync.Once
}

// syncBuffer collects a subprocess's stderr while the test reads it from
// another goroutine.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// launch starts the server binary and returns immediately, without waiting for
// it to serve — the config-guard tests need a handle on a process that is
// *supposed* to die on boot.
func launch(t *testing.T, env map[string]string) *server {
	t.Helper()
	port := env["PORT"]
	cmd := exec.Command(binServer)
	cmd.Dir = subprocDir
	cmd.Env = baseEnv()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Own process group: SIGTERM goes to this server only, never to the test
	// runner that spawned it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	s := &server{t: t, cmd: cmd, Port: port, URL: "http://127.0.0.1:" + port, stderr: out, done: make(chan struct{})}
	go func() {
		s.waitErr = cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(s.stop)
	return s
}

// startServer boots a healthy server on a free port and waits until it answers.
func startServer(t *testing.T, overrides map[string]string) *server {
	t.Helper()
	env := serverEnv(freePort(t))
	for k, v := range overrides {
		env[k] = v
	}
	s := launch(t, env)
	s.awaitHealthy(bootTimeout)
	return s
}

// awaitHealthy polls /api/health until the server answers, failing early (and
// loudly, with the captured output) if the process died instead.
func (s *server) awaitHealthy(d time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			s.t.Fatalf("server exited during boot: %v\n%s", s.waitErr, s.stderr.String())
		default:
		}
		res, err := http.Get(s.URL + "/api/health") //nolint:noctx // bounded by the loop deadline
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("server did not become healthy within %s\n%s", d, s.stderr.String())
}

// signal sends sig to the server process.
func (s *server) signal(sig os.Signal) {
	s.t.Helper()
	if err := s.cmd.Process.Signal(sig); err != nil {
		s.t.Fatalf("signal %v: %v", sig, err)
	}
}

// awaitExit waits for the process to be reaped and reports its exit code.
// ok=false means it was still running when the wait expired.
func (s *server) awaitExit(d time.Duration) (code int, ok bool) {
	s.t.Helper()
	select {
	case <-s.done:
		return exitCode(s.waitErr), true
	case <-time.After(d):
		return 0, false
	}
}

// stop terminates the server the way an orchestrator would, escalating to
// SIGKILL if the drain overruns. Registered as a t.Cleanup by launch, so no test
// can leak a process onto the port.
func (s *server) stop() {
	s.stopOnce.Do(func() {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-s.done:
		case <-time.After(20 * time.Second):
			_ = s.cmd.Process.Kill()
			<-s.done
		}
	})
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// freePort asks the kernel for an unused port. The suite never hardcodes 8090:
// a zombie from an earlier dev run may still own it (CLAUDE.md).
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	return port
}

// ── HTTP helpers ────────────────────────────────────────────────────────────

type resp struct {
	Status int
	Body   []byte
	Header http.Header
}

func (r resp) mapBody(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("decode object body (%d): %v\n%s", r.Status, err, r.Body)
	}
	return m
}

// request is the knobs a single call may need; everything but Method and Path is
// optional. Client is set when a test needs a cookie jar of its own.
type request struct {
	Method string
	Path   string // API path without the /api prefix
	Token  string
	Body   any
	Header http.Header
	Client *http.Client
}

func (s *server) call(t *testing.T, r request) resp {
	t.Helper()
	var rd io.Reader
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(r.Method, s.URL+"/api"+r.Path, rd) //nolint:noctx // bounded by the client timeout
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	cl := r.Client
	if cl == nil {
		cl = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := cl.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", r.Method, r.Path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

func (s *server) get(t *testing.T, path, token string) resp {
	t.Helper()
	return s.call(t, request{Method: http.MethodGet, Path: path, Token: token})
}

func (s *server) post(t *testing.T, path, token string, body any) resp {
	t.Helper()
	return s.call(t, request{Method: http.MethodPost, Path: path, Token: token, Body: body})
}

func expect(t *testing.T, r resp, want int) map[string]any {
	t.Helper()
	if r.Status != want {
		t.Fatalf("status %d, want %d\n%s", r.Status, want, r.Body)
	}
	if len(r.Body) == 0 || r.Body[0] != '{' {
		return nil
	}
	return r.mapBody(t)
}

// account is a registered user with its credentials.
type account struct {
	Email   string
	Access  string
	Refresh string
	UserID  string
}

// register signs a fresh user up over the real socket. The email is unique per
// call, so tests share the throwaway database without cleaning up after each
// other — the same data-scoping trick the in-process harness uses.
func (s *server) register(t *testing.T, label string) *account {
	t.Helper()
	email := fmt.Sprintf("e2e-%s-%d-%s@test.local", label, userSeq.Add(1), runID)
	r := s.post(t, "/auth/register", "", map[string]any{
		"email": email, "name": "E2E " + label, "password": "password-123",
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
	return &account{Email: email, Access: out.AccessToken, Refresh: out.RefreshToken, UserID: out.User.ID}
}

// mkStack builds workspace → group → project → board and returns the ids plus
// the board's seeded columns — the minimum fixture for anything task-shaped.
type stack struct {
	WS, Group, Project, Board string
	Columns                   []map[string]any
}

func (s *server) mkStack(t *testing.T, a *account) stack {
	t.Helper()
	ws := expect(t, s.post(t, "/workspaces", a.Access, map[string]any{"name": "WS " + t.Name()}), http.StatusCreated)
	wsID := ws["id"].(string)
	g := expect(t, s.post(t, "/workspaces/"+wsID+"/groups", a.Access, map[string]any{"name": "Группа"}), http.StatusCreated)
	gID := g["id"].(string)
	p := expect(t, s.post(t, "/workspaces/"+wsID+"/projects", a.Access,
		map[string]any{"name": "Проект " + t.Name(), "group_id": gID}), http.StatusCreated)
	pID := p["id"].(string)
	b := expect(t, s.post(t, "/projects/"+pID+"/boards", a.Access, map[string]any{"name": "Доска"}), http.StatusCreated)
	bID := b["id"].(string)

	var cols []map[string]any
	r := s.get(t, "/boards/"+bID+"/columns", a.Access)
	if err := json.Unmarshal(r.Body, &cols); err != nil {
		t.Fatalf("decode columns: %v\n%s", err, r.Body)
	}
	if len(cols) == 0 {
		t.Fatalf("board came back with no columns\n%s", r.Body)
	}
	return stack{WS: wsID, Group: gID, Project: pID, Board: bID, Columns: cols}
}
