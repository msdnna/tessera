package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// An inbound X-Request-Id is honoured (so a fronting proxy's id survives), and
// the same value is echoed back and readable from the context.
func TestRequestIDHonoursInbound(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Request.Header.Set(HeaderRequestID, "caller-supplied-id")

	var seen string
	RequestID()(c)
	seen = GetRequestID(c)

	if seen != "caller-supplied-id" {
		t.Fatalf("GetRequestID = %q, want the inbound id", seen)
	}
	if got := w.Header().Get(HeaderRequestID); got != "caller-supplied-id" {
		t.Fatalf("echoed header = %q, want the inbound id", got)
	}
}

// With no inbound header, a fresh id is minted and still readable/echoed.
func TestRequestIDGenerates(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)

	RequestID()(c)

	id := GetRequestID(c)
	if id == "" {
		t.Fatal("no request id generated")
	}
	if w.Header().Get(HeaderRequestID) != id {
		t.Fatalf("echoed header %q != context id %q", w.Header().Get(HeaderRequestID), id)
	}
}

// GetRequestID must be safe on a plain worker context that never carried an id.
func TestGetRequestIDFromPlainContext(t *testing.T) {
	if got := GetRequestID(context.Background()); got != "" {
		t.Fatalf("GetRequestID(background) = %q, want empty", got)
	}
}
