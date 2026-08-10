package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// fail returns an opaque 500 to the client while logging the real cause: the
// error text must never reach the response body, and it must reach the log.
func TestFailDoesNotLeakError(t *testing.T) {
	buf := captureSlog(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PATCH", "/api/tasks/42", nil)

	fail(c, errors.New("pq: relation secretsauce does not exist"))

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "secretsauce") {
		t.Fatalf("error cause leaked into response body: %s", w.Body.String())
	}
	if !strings.Contains(buf.String(), "secretsauce") {
		t.Fatalf("error cause was not logged server-side: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "/api/tasks/42") {
		t.Fatalf("log line missing request path: %s", buf.String())
	}
}

// soft stays silent on the common nil-error path and logs the op name when
// something actually failed — that's the whole point of #2632.
func TestSoftLogsOnlyOnError(t *testing.T) {
	buf := captureSlog(t)

	soft(context.Background(), "MarkGitlabDueOverridden", nil)
	if buf.Len() != 0 {
		t.Fatalf("soft logged on a nil error: %s", buf.String())
	}

	soft(context.Background(), "MarkGitlabDueOverridden", errors.New("boom"))
	if !strings.Contains(buf.String(), "MarkGitlabDueOverridden") {
		t.Fatalf("soft did not log the failing op: %s", buf.String())
	}
}
