package handlers

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// streamGitlabImage tries the PAT three ways, and the last of them puts it in
// the query string (`?private_token=…`) because older GitLab web routes accept
// nothing else. A transport error on that attempt yields a *url.Error whose
// Error() carries the whole URL — credential included — and the diagnostic
// log line prints it. The failing attempt is exactly the likely one: a private
// instance whose host does not resolve, times out, or is refused by the
// netguard dialer. Nobody sees the leak in a working install, which is why it
// needs a test rather than a reading.
func TestStreamGitlabImage_TransportErrorDoesNotLogPAT(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/gitlab/avatar", nil)

	// Port 1 is refused immediately, so all three attempts fail at the
	// transport and the third one — the one carrying the credential in the
	// URL — is the one that lands in lastBody.
	const pat = "glpat-ThisIsALiveAccessToken"
	if (&API{}).streamGitlabImage(c, "http://127.0.0.1:1/uploads/-/system/user/avatar/7/avatar.png", "127.0.0.1:1", pat) {
		t.Fatal("streamGitlabImage reported success against a refused port")
	}

	if strings.Contains(buf.String(), pat) {
		t.Fatalf("the user's GitLab PAT reached the log in the clear:\n%s", buf.String())
	}
	// The redaction must not cost the diagnostic its point: the parameter is
	// still named, so a reader can tell which attempt failed.
	if line := buf.String(); !strings.Contains(line, "private_token=***") {
		t.Errorf("redacted line lost the parameter name, log reads:\n%s", line)
	}
}
