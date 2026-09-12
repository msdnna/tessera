// The image credential has to leave with the account (#2932).
//
// Deactivation already reaches every credential that touches the database — the
// refresh chain, the PAT, the open socket (see deactivation_test.go). The media
// cookie was the one that didn't: a 30-day JWT, verified by signature alone, so
// nothing about it was ever looked up. On an install with MEDIA_REQUIRE_AUTH on
// that left the departing browser reading inline images for the rest of the
// month — the same month a revoked refresh token would have bought.
package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestMediaCookieRejectedForDeactivatedOwner pins the cookie arm of MediaAuth on
// the flag, with it flipped straight in the database: no revocation has run, so
// this is the credential answering on the merits of the account behind it rather
// than on a cleanup that happened to fire.
func TestMediaCookieRejectedForDeactivatedOwner(t *testing.T) {
	t.Parallel()
	srv := mediaAuthServer(t)

	// A file to ask for, uploaded by somebody else: what's on trial is the
	// caller's credential, not their claim to this particular image.
	owner := signup(t)
	m := owner.expect(t, uploadFile(t, owner, "/uploads", "file", "ушедший.png", []byte(pngBody)), http.StatusCreated)
	url, _ := m["url"].(string)
	if url == "" {
		t.Fatalf("upload returned no url: %v", m)
	}

	reg, _ := registerCookieMode(t)
	cookie := mediaCookie(reg)
	if cookie == nil {
		t.Fatal("no media cookie to present")
	}
	var body struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(reg.Body, &body); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	if body.User.ID == "" {
		t.Fatalf("register response carries no user id: %s", reg.Body)
	}

	if got := mediaGet(t, srv, url, nil, cookie); got != http.StatusOK {
		t.Fatalf("media cookie of a live account: status %d, want 200", got)
	}

	setActiveInDB(t, body.User.ID, false)
	if got := mediaGet(t, srv, url, nil, cookie); got != http.StatusUnauthorized {
		t.Fatalf("media cookie of a deactivated owner: status %d, want 401 — "+
			"the cookie is signed for 30 days, so without a lookup the account never actually leaves", got)
	}

	// Deactivation is a pause, not a shredder: the cookie the user still holds
	// works again when the account comes back, exactly like their PAT.
	setActiveInDB(t, body.User.ID, true)
	if got := mediaGet(t, srv, url, nil, cookie); got != http.StatusOK {
		t.Fatalf("media cookie after reactivation: status %d, want 200", got)
	}
}
