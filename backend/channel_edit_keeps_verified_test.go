// A channel's `verified` badge is a claim about a destination that was actually
// reached: /test sends a probe there and only success flips it. This pins that
// editing the channel away from that destination — a different URL, a replaced
// or erased secret — takes the claim back with it.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestChannelEditResetsVerified: the badge must not survive an edit that moves
// the channel somewhere the probe never went.
//
// Both clients show it (web NotificationSettings.vue, android
// NotificationSettingsScreen.kt), and the in-app help sends a reader straight to
// it when deliveries stop arriving ("4. The channel is verified"). A wrong token
// pasted over a working one is a *permanent* delivery error — no retries, no
// second chance — so the one screen that could name the problem is the one the
// user is told to trust.
func TestChannelEditResetsVerified(t *testing.T) {
	t.Parallel()
	c := signup(t)

	ok := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}
	first, second := ok(), ok()
	defer first.Close()
	defer second.Close()

	verified := func(t *testing.T, id string) bool {
		t.Helper()
		v := byID(t, c.get("/notification-channels").listBody(t), id)
		b, _ := v["verified"].(bool)
		return b
	}

	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": "Хук", "config": map[string]any{"url": first.URL},
	}), http.StatusCreated)
	id := ch["id"].(string)
	if res := c.expect(t, c.post("/notification-channels/"+id+"/test", nil), http.StatusOK); res["ok"] != true {
		t.Fatalf("probe against the first endpoint: %v", res)
	}
	if !verified(t, id) {
		t.Fatalf("a successful probe did not mark the channel verified")
	}

	// Control: an edit that leaves the destination alone (rename, disable) keeps
	// the badge — it is still the address the probe reached.
	c.expect(t, c.patch("/notification-channels/"+id, map[string]any{
		"label": "Хук v2", "config": map[string]any{"url": first.URL},
	}), http.StatusOK)
	if !verified(t, id) {
		t.Fatalf("a rename dropped the badge; only the destination should")
	}

	// The finding: the URL now points somewhere nothing was ever sent.
	up := c.expect(t, c.patch("/notification-channels/"+id, map[string]any{
		"label": "Хук v2", "config": map[string]any{"url": second.URL},
	}), http.StatusOK)
	if up["verified"] == true || verified(t, id) {
		t.Fatalf("the channel still claims verified after its url moved to an untested endpoint")
	}

	// And the badge is not stuck off: probing the new endpoint earns it back.
	if res := c.expect(t, c.post("/notification-channels/"+id+"/test", nil), http.StatusOK); res["ok"] != true {
		t.Fatalf("probe against the second endpoint: %v", res)
	}
	if !verified(t, id) {
		t.Fatalf("a successful probe against the new url did not re-verify the channel")
	}

	// Same for the credential: a replaced secret is a different request on the
	// wire, and an erased one is an unauthenticated request.
	up = c.expect(t, c.patch("/notification-channels/"+id, map[string]any{
		"config": map[string]any{"url": second.URL}, "secret": map[string]any{"auth_header": "Bearer sekret"},
	}), http.StatusOK)
	if up["verified"] == true || verified(t, id) {
		t.Fatalf("the channel still claims verified after a new auth header was stored")
	}
	c.expect(t, c.post("/notification-channels/"+id+"/test", nil), http.StatusOK)
	if !verified(t, id) {
		t.Fatalf("re-probe with the auth header did not re-verify")
	}
	up = c.expect(t, c.patch("/notification-channels/"+id, map[string]any{
		"config": map[string]any{"url": second.URL}, "clear_secret": true,
	}), http.StatusOK)
	if up["verified"] == true || verified(t, id) {
		t.Fatalf("the channel still claims verified after its secret was erased")
	}
}
