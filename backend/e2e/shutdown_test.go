//go:build e2e

// Graceful shutdown as main() actually assembles it. backend/shutdown_test.go
// exercises drain() against an http.Server the test builds itself — a copy of
// the wiring, free to drift from the real one. Here the signal goes to the real
// process, over a real socket, with a real request in flight.
package e2e

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGracefulShutdownDrainsInFlightRequest(t *testing.T) {
	s := startServer(t, nil)

	// A request the server cannot finish on its own: headers announce a body
	// that has only partly arrived, so the handler is parked inside
	// ShouldBindJSON until the test decides to send the rest. Any endpoint would
	// do — login is convenient because an unknown user gives a deterministic 401.
	body := fmt.Sprintf(`{"email":"nobody-%s@test.local","password":"password-123"}`, runID)
	head := "POST /api/auth/login HTTP/1.1\r\n" +
		"Host: 127.0.0.1\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(body)) +
		"Connection: close\r\n\r\n"

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+s.Port, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	split := len(body) / 2
	if _, err := io.WriteString(conn, head+body[:split]); err != nil {
		t.Fatalf("write partial request: %v", err)
	}
	// Let the server read the headers and enter the handler before the signal —
	// otherwise this would test "SIGTERM before the request", a different case.
	time.Sleep(500 * time.Millisecond)

	s.signal(syscall.SIGTERM)

	// The listener must be gone almost immediately: a rolling deploy depends on
	// the old process refusing new work while it finishes the old.
	deadline := time.Now().Add(5 * time.Second)
	refused := false
	for time.Now().Before(deadline) {
		res, err := http.Get(s.URL + "/api/health") //nolint:noctx // bounded by the loop deadline
		if err != nil {
			refused = true
			break
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}
	if !refused {
		t.Error("server still accepted new connections after SIGTERM")
	}

	// …and the in-flight one still gets its answer.
	if _, err := io.WriteString(conn, body[split:]); err != nil {
		t.Fatalf("write rest of the body: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	answer, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read the in-flight response: %v (drained too early?)", err)
	}
	if !strings.HasPrefix(string(answer), "HTTP/1.1 401") {
		t.Fatalf("in-flight request was cut off instead of answered:\n%s", answer)
	}

	code, exited := s.awaitExit(20 * time.Second)
	if !exited {
		t.Fatalf("process still alive after the drain budget\n%s", s.stderr.String())
	}
	if code != 0 {
		t.Errorf("exit code %d after SIGTERM, want 0", code)
	}
	// drain() logs its last line only after the workers and the hub are down, in
	// that order — the ordering main() depends on to avoid a worker writing into
	// a closed pool.
	if out := s.stderr.String(); !strings.Contains(out, "shutdown: complete") {
		t.Errorf("shutdown did not run to completion:\n%s", out)
	}
}
