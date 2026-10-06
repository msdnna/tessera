// Deleting a notification channel must also scrub it out of the user's routing
// rules: channel_ids is a plain uuid[] with no FK, so a dangling id survives the
// delete and then trips the ownership check on the rule's next save.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDeleteChannelScrubsRoutes walks the path a user actually takes: two
// channels on one rule, delete one of them, then touch the rule again the way
// the settings screen does (it re-sends the rule's stored channel_ids verbatim
// when the enable switch is flipped). Without the scrub the rule is wedged —
// every later save answers 400 "одного из каналов не существует".
func TestDeleteChannelScrubsRoutes(t *testing.T) {
	t.Parallel()
	c := signup(t)

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	mkChannel := func(label string) string {
		t.Helper()
		ch := c.expect(t, c.post("/notification-channels", map[string]any{
			"type": "webhook", "label": label, "config": map[string]any{"url": hook.URL},
		}), http.StatusCreated)
		return ch["id"].(string)
	}
	doomed, kept := mkChannel("Уходящий"), mkChannel("Остающийся")

	rt := c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{}, "channel_ids": []string{doomed, kept},
		"options": map[string]any{}, "enabled": true,
	}), http.StatusCreated)
	rtID := rt["id"].(string)

	if r := c.del("/notification-channels/" + doomed); r.Status != http.StatusNoContent {
		t.Fatalf("delete channel: %d\n%s", r.Status, r.Body)
	}

	// The rule must no longer point at the dead channel.
	routes := c.get("/notification-routes").listBody(t)
	stored := byID(t, routes, rtID)
	ids, _ := stored["channel_ids"].([]any)
	if len(ids) != 1 || ids[0] != kept {
		t.Fatalf("route still carries the deleted channel: %v", stored["channel_ids"])
	}

	// Replay the settings screen's enable toggle: it echoes back whatever the
	// list handed it, so a dangling id would come straight back as a 400.
	upd := c.patch("/notification-routes/"+rtID, map[string]any{
		"position": stored["position"], "matcher": stored["matcher"],
		"channel_ids": stored["channel_ids"], "options": stored["options"],
		"enabled": false,
	})
	if upd.Status != http.StatusOK {
		t.Fatalf("toggling the rule after a channel delete: %d\n%s", upd.Status, upd.Body)
	}

	// Deleting the last referenced channel empties the list rather than leaving
	// a rule that matches everything and delivers nowhere-but-not-visibly.
	if r := c.del("/notification-channels/" + kept); r.Status != http.StatusNoContent {
		t.Fatalf("delete second channel: %d\n%s", r.Status, r.Body)
	}
	stored = byID(t, c.get("/notification-routes").listBody(t), rtID)
	if ids, _ := stored["channel_ids"].([]any); len(ids) != 0 {
		t.Fatalf("route channel_ids after both channels went: %v", stored["channel_ids"])
	}
}
