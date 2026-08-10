package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// routerWithBodyLimit echoes how many bytes the handler managed to read, so the
// tests can tell "refused up front" from "cut off mid-read".
func routerWithBodyLimit(def int64, byRoute map[string]int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(def, byRoute))
	echo := func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			c.String(http.StatusBadRequest, "read failed after %d bytes", n)
			return
		}
		c.String(http.StatusOK, "%d", n)
	}
	r.POST("/api/small", echo)
	r.POST("/api/big", echo)
	r.POST("/api/exempt", echo)
	return r
}

func post(r *gin.Engine, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBodyLimitRefusesOversizedWith413(t *testing.T) {
	r := routerWithBodyLimit(1024, nil)

	if got := post(r, "/api/small", bytes.Repeat([]byte("x"), 1024)).Code; got != http.StatusOK {
		t.Fatalf("body exactly at the limit: got %d, want 200", got)
	}
	w := post(r, "/api/small", bytes.Repeat([]byte("x"), 1025))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body one byte over: got %d, want 413", w.Code)
	}
}

func TestBodyLimitPerRouteBudget(t *testing.T) {
	r := routerWithBodyLimit(1024, map[string]int64{
		"/api/big":    8192,
		"/api/exempt": NoBodyLimit,
	})

	payload := bytes.Repeat([]byte("x"), 4096)
	if got := post(r, "/api/small", payload).Code; got != http.StatusRequestEntityTooLarge {
		t.Fatalf("default route: got %d, want 413", got)
	}
	if got := post(r, "/api/big", payload).Code; got != http.StatusOK {
		t.Fatalf("route with its own budget: got %d, want 200", got)
	}
	if got := post(r, "/api/exempt", bytes.Repeat([]byte("x"), 1<<20)).Code; got != http.StatusOK {
		t.Fatalf("exempt route: got %d, want 200", got)
	}
}

// A client that understates Content-Length gets past the cheap check; the
// MaxBytesReader behind it is what actually stops the read. The handler sees a
// read error rather than a silently truncated body — which is the property that
// matters, since a truncated body would be parsed as if it were complete.
func TestBodyLimitEnforcesDespiteLyingContentLength(t *testing.T) {
	r := routerWithBodyLimit(1024, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/small", strings.NewReader(strings.Repeat("x", 4096)))
	req.ContentLength = 10
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d (%s), want 400 from the failed read", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "read failed") {
		t.Fatalf("handler completed the read: %s", w.Body.String())
	}
}

// Chunked requests carry no Content-Length (-1), so the up-front check must not
// mistake that for "unlimited" or for "oversized".
func TestBodyLimitHandlesUnknownContentLength(t *testing.T) {
	r := routerWithBodyLimit(1024, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/small", strings.NewReader("hello"))
	req.ContentLength = -1
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "5" {
		t.Fatalf("got %d/%q, want 200/\"5\"", w.Code, w.Body.String())
	}
}
