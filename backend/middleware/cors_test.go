package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func originFor(t *testing.T, h gin.HandlerFunc, requestOrigin string) string {
	t.Helper()
	r := gin.New()
	r.Use(h)
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if requestOrigin != "" {
		req.Header.Set("Origin", requestOrigin)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Header().Get("Access-Control-Allow-Origin")
}

func TestCORSWildcard(t *testing.T) {
	// Empty primary origin behaves like "*": any origin is allowed.
	if got := originFor(t, CORS(""), "https://evil.example"); got != "*" {
		t.Fatalf("wildcard: want *, got %q", got)
	}
	if got := originFor(t, CORS("*"), "https://evil.example"); got != "*" {
		t.Fatalf("explicit *: want *, got %q", got)
	}
}

func TestCORSReflectsAllowedOrigin(t *testing.T) {
	h := CORS("https://tessera.msdnna.website", "tauri://localhost", "http://tauri.localhost")

	// The primary web origin is reflected.
	if got := originFor(t, h, "https://tessera.msdnna.website"); got != "https://tessera.msdnna.website" {
		t.Fatalf("web origin: want reflected, got %q", got)
	}
	// A desktop (Tauri) origin from the extra list is reflected.
	if got := originFor(t, h, "tauri://localhost"); got != "tauri://localhost" {
		t.Fatalf("desktop origin: want reflected, got %q", got)
	}
	// An unlisted origin gets no Allow-Origin header (browser blocks the read).
	if got := originFor(t, h, "https://evil.example"); got != "" {
		t.Fatalf("unlisted origin: want empty, got %q", got)
	}
}

func TestCORSPreflightAborts(t *testing.T) {
	r := gin.New()
	r.Use(CORS("https://tessera.msdnna.website"))
	called := false
	r.OPTIONS("/x", func(c *gin.Context) { called = true; c.Status(200) })
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://tessera.msdnna.website")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight: want 204, got %d", w.Code)
	}
	if called {
		t.Fatal("preflight should be aborted before the handler")
	}
}
