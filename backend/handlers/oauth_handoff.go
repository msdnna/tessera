package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// The desktop shell gets its session through a one-time code instead of the tokens
// themselves (#2932).
//
// Why: the last hop of the OAuth flow is `tessera://oauth/callback#…`, and on Linux and
// Windows the OS delivers that URL by *launching the app with it as a command-line
// argument*. Arguments are not private — on Linux /proc/<pid>/cmdline is world-readable,
// so `ps aux` from any account on the machine shows whatever the fragment contained. A
// refresh token there is a 30-day session handed to every local process; it also lands in
// the desktop portal / xdg-open logs on the way. A code that lives 60 seconds, works once
// and is redeemed over HTTPS costs an attacker a race they have already lost by the time
// the window paints.
//
// Android keeps the token handoff: there the deep link arrives as an Intent, which is not
// argv and not readable by other apps, and changing the format would lock every installed
// APK out of GitLab login.

// oauthHandoffTTL bounds how long a code is worth stealing. The app redeems it the moment
// the deep link arrives, so this only has to cover process startup on a cold launch.
const oauthHandoffTTL = 60 * time.Second

// oauthHandoffStore holds the codes minted by the callback until the desktop app redeems
// them. In memory on purpose: these are single-use and expire in a minute, so persisting
// them would only widen the window and add a table to keep clean. A backend restart
// mid-login costs the user one retry.
type oauthHandoffStore struct {
	mu      sync.Mutex
	pending map[string]oauthHandoffEntry
}

type oauthHandoffEntry struct {
	userID  uuid.UUID
	expires time.Time
}

func newOAuthHandoffStore() *oauthHandoffStore {
	return &oauthHandoffStore{pending: map[string]oauthHandoffEntry{}}
}

// issue mints a code for the user. Nothing sensitive is stored: tokens are minted at
// redemption, so a flow nobody picks up never creates a refresh token at all.
func (s *oauthHandoffStore) issue(userID uuid.UUID, now time.Time) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := hex.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Sweep on write: there is no background goroutine, and without this an abandoned
	// login would sit in the map until the process restarts.
	for k, e := range s.pending {
		if now.After(e.expires) {
			delete(s.pending, k)
		}
	}
	s.pending[code] = oauthHandoffEntry{userID: userID, expires: now.Add(oauthHandoffTTL)}
	return code, nil
}

// redeem burns the code and returns whom it belongs to. The delete happens for expired
// codes too — a late redemption must not leave the entry around for a second attempt.
func (s *oauthHandoffStore) redeem(code string, now time.Time) (uuid.UUID, bool) {
	if code == "" {
		return uuid.Nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.pending[code]
	if !ok {
		return uuid.Nil, false
	}
	delete(s.pending, code)
	if now.After(e.expires) {
		return uuid.Nil, false
	}
	return e.userID, true
}

// OAuthHandoff exchanges a one-time code from the desktop deep link for the session.
// Public route: the code *is* the credential, exactly like /auth/refresh.
func (h *AuthHandler) OAuthHandoff(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	userID, ok := h.handoff.redeem(body.Code, time.Now())
	if !ok {
		// One message for unknown / expired / already-redeemed: telling them apart only
		// helps someone guessing.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired code"})
		return
	}
	user, err := h.q.GetUserByID(c, userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired code"})
		return
	}
	// Never cookie mode: the desktop shell is cross-origin against the API and stores the
	// refresh token itself, which is why this handoff exists in the first place.
	h.issueMode(c, user, false)
}
