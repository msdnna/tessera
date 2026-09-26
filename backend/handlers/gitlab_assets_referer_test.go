package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const refererPAT = "glpat-ThisIsALiveAccessToken"

// avatarRedirectStand builds a fake GitLab whose avatar route refuses header
// auth (a sign-in page, which is why the query-param attempt exists at all) and
// answers the `?private_token=` attempt with a 302 to redirectTo — the shape of
// an instance that keeps uploads in object storage. It returns the GitLab URL
// and its host.
func avatarRedirectStand(t *testing.T, target *httptest.Server) (glURL, ghost string) {
	t.Helper()
	gl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("private_token") == refererPAT {
			http.Redirect(w, r, target.URL+"/objstore/avatar.png", http.StatusFound)
			return
		}
		// Attempts 1 and 2 (Bearer / PRIVATE-TOKEN headers): a session-gated web
		// route serves HTML, so streamGitlabImage falls through to attempt 3.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>sign in</html>"))
	}))
	t.Cleanup(gl.Close)
	return gl.URL, strings.TrimPrefix(gl.URL, "http://")
}

// imageSink serves a PNG and records the Referer it was asked with.
func imageSink(t *testing.T, seen *string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	t.Cleanup(s.Close)
	return s
}

func callStreamGitlabImage(t *testing.T, rawURL, ghost string) bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/gitlab/avatar", nil)
	return (&API{}).streamGitlabImage(c, rawURL, ghost, refererPAT)
}

// The avatar proxy's redirect hook drops the credential headers when a hop leaves
// the GitLab host — but the third attempt puts the PAT in the *query string*, and
// net/http fills Referer with the previous URL verbatim (it strips userinfo, never
// the query). So without stripping Referer as well, the hop to a presigned
// object-storage URL hands the user's PAT to a host that was never meant to see
// it, while the header-stripping above it reads as if that case is covered.
func TestStreamGitlabImage_RedirectOffHostDoesNotLeakPATInReferer(t *testing.T) {
	var seen string
	sink := imageSink(t, &seen)
	glURL, ghost := avatarRedirectStand(t, sink)

	if !callStreamGitlabImage(t, glURL+"/uploads/-/system/user/avatar/7/avatar.png", ghost) {
		t.Fatal("streamGitlabImage did not stream the image the redirect pointed at")
	}
	if strings.Contains(seen, refererPAT) {
		t.Fatalf("the user's GitLab PAT reached the foreign host in Referer: %q", seen)
	}
}

// The counterweight, and the reason the test above cannot pass vacuously: a hop
// that stays on the GitLab host keeps its Referer. It also pins the scope of the
// fix — the credential is stripped because the host changed, not because Referer
// is being blanket-removed from every redirect.
func TestStreamGitlabImage_SameHostRedirectKeepsReferer(t *testing.T) {
	var seen string
	var gl *httptest.Server
	gl = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/objstore/"):
			seen = r.Header.Get("Referer")
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
		case r.URL.Query().Get("private_token") == refererPAT:
			http.Redirect(w, r, gl.URL+"/objstore/avatar.png", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html>sign in</html>"))
		}
	}))
	t.Cleanup(gl.Close)

	ghost := strings.TrimPrefix(gl.URL, "http://")
	if !callStreamGitlabImage(t, gl.URL+"/uploads/-/system/user/avatar/7/avatar.png", ghost) {
		t.Fatal("streamGitlabImage did not stream the image the redirect pointed at")
	}
	if !strings.Contains(seen, refererPAT) {
		t.Fatalf("Referer was dropped on a same-host hop, so the test above proves nothing: %q", seen)
	}
}
