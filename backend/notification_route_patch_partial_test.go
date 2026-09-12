// PATCH /notification-routes/:id has to be a partial edit. The routing list is
// first-match-wins and an empty matcher is a catch-all, so defaulting the fields
// a request left out is not a cosmetic sloppiness: it silently promotes a narrow
// rule to the top of the list and widens it to every notification the user gets.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPatchRouteKeepsUntouchedFields flips one rule's enable switch the way a
// script (or any client that trusts the verb) would — sending only the field it
// means to change — and checks the rest of the rule survived.
//
// Before the fix all three assertions failed at once: position fell to 0, the
// matcher was replaced by {} (which notify.Matcher documents as "matches
// everything"), and the channel list emptied. The user is left with a catch-all
// ahead of their mute rule, i.e. every notification they had silenced coming
// back through a rule that no longer names a channel.
func TestPatchRouteKeepsUntouchedFields(t *testing.T) {
	t.Parallel()
	c := signup(t)

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": "Вебхук", "config": map[string]any{"url": hook.URL},
	}), http.StatusCreated)
	chID := ch["id"].(string)

	// Rule 1: silence mentions. Created first, so it sits ahead of rule 2.
	mute := c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{"kinds": []string{"mention"}},
		"options": map[string]any{"mute": true}, "channel_ids": []string{},
		"enabled": true,
	}), http.StatusCreated)
	muteID := mute["id"].(string)

	// Rule 2: a narrow rule the user has switched off for now.
	narrow := c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{"kinds": []string{"assigned"}},
		"options": map[string]any{}, "channel_ids": []string{chID},
		"enabled": false,
	}), http.StatusCreated)
	narrowID := narrow["id"].(string)
	wantPos := narrow["position"].(float64)

	// The whole request: turn rule 2 back on. Nothing else was asked for.
	c.expect(t, c.patch("/notification-routes/"+narrowID, map[string]any{
		"enabled": true,
	}), http.StatusOK)

	routes := c.get("/notification-routes").listBody(t)
	got := byID(t, routes, narrowID)

	if pos, _ := got["position"].(float64); pos != wantPos {
		t.Errorf("position after a partial PATCH = %v, want %v (the rule moved in a first-match-wins list)", got["position"], wantPos)
	}
	m, _ := got["matcher"].(map[string]any)
	kinds, _ := m["kinds"].([]any)
	if len(kinds) != 1 || kinds[0] != "assigned" {
		t.Errorf("matcher after a partial PATCH = %v, want kinds [assigned] (an empty matcher is a catch-all)", got["matcher"])
	}
	ids, _ := got["channel_ids"].([]any)
	if len(ids) != 1 || ids[0] != chID {
		t.Errorf("channel_ids after a partial PATCH = %v, want [%s]", got["channel_ids"], chID)
	}
	if on, _ := got["enabled"].(bool); !on {
		t.Errorf("enabled after a partial PATCH = %v, want true (that was the one field asked for)", got["enabled"])
	}

	// The consequence the field checks stand for: the mute rule must still be the
	// one a mention meets first.
	if len(routes) < 2 || routes[0]["id"] != muteID {
		t.Errorf("rule order after a partial PATCH: first is %v, want the mute rule %s", routes[0]["id"], muteID)
	}
}

// TestPatchRouteStillReplacesWhatItNames guards the other half: a field that IS
// present must land, including the ones whose zero value is meaningful — an
// empty channel list (what a mute rule stores) and enabled:false.
func TestPatchRouteStillReplacesWhatItNames(t *testing.T) {
	t.Parallel()
	c := signup(t)

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": "Вебхук", "config": map[string]any{"url": hook.URL},
	}), http.StatusCreated)
	chID := ch["id"].(string)

	rt := c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{"kinds": []string{"assigned"}},
		"options": map[string]any{}, "channel_ids": []string{chID},
		"enabled": true,
	}), http.StatusCreated)
	rtID := rt["id"].(string)

	// Turn it into a mute rule: no channels, a wider matcher, switched off, moved.
	c.expect(t, c.patch("/notification-routes/"+rtID, map[string]any{
		"matcher": map[string]any{}, "channel_ids": []string{},
		"options": map[string]any{"mute": true}, "enabled": false, "position": 0.5,
	}), http.StatusOK)

	got := byID(t, c.get("/notification-routes").listBody(t), rtID)
	if ids, _ := got["channel_ids"].([]any); len(ids) != 0 {
		t.Errorf("an explicitly empty channel_ids was ignored: %v", got["channel_ids"])
	}
	if m, _ := got["matcher"].(map[string]any); len(m) != 0 {
		t.Errorf("an explicitly empty matcher was ignored: %v", got["matcher"])
	}
	if o, _ := got["options"].(map[string]any); o["mute"] != true {
		t.Errorf("options after PATCH = %v, want mute:true", got["options"])
	}
	if on, _ := got["enabled"].(bool); on {
		t.Errorf("enabled:false was ignored: %v", got["enabled"])
	}
	if pos, _ := got["position"].(float64); pos != 0.5 {
		t.Errorf("position after PATCH = %v, want 0.5", got["position"])
	}
}
