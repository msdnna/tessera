package main

import (
	"net/http"
	"strings"
	"testing"
)

// Per-user acknowledgements (/users/me/acknowledgements) — the one-shot flags the
// client keeps for the What's-New modal, the sidebar spotlights and the Get
// Started tour. Both endpoints shipped without a single test, which is how the
// rune/byte slip below survived: the handler promises "1-200 chars" and measured
// bytes, so a Cyrillic key of 101 characters was refused as too long (#2932).
//
// ackList reads the caller's flags as key → ack_at.
func ackList(t *testing.T, c *client) map[string]string {
	t.Helper()
	r := c.get("/users/me/acknowledgements")
	if r.Status != http.StatusOK {
		t.Fatalf("list: %d\n%s", r.Status, r.Body)
	}
	rows := r.listBody(t)
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		key, _ := r["key"].(string)
		at, _ := r["ack_at"].(string)
		out[key] = at
	}
	return out
}

// A fresh account has no flags, and the endpoint says so with an empty array —
// not null, which the client would have to special-case before iterating.
func TestAcknowledgementsStartEmpty(t *testing.T) {
	t.Parallel()
	c := signup(t)

	r := c.get("/users/me/acknowledgements")
	if r.Status != http.StatusOK {
		t.Fatalf("list: %d\n%s", r.Status, r.Body)
	}
	if got := strings.TrimSpace(string(r.Body)); got != "[]" {
		t.Fatalf("a fresh account should answer with an empty array, got %s", got)
	}
}

// Writing a flag returns it and makes it visible to the next read. Writing it a
// second time keeps the FIRST ack_at: "Понятно" pressed twice must not re-date
// the record, or "when did this user first see it" stops being answerable.
func TestAcknowledgeIsIdempotent(t *testing.T) {
	t.Parallel()
	c := signup(t)

	first := c.expect(t, c.post("/users/me/acknowledgements", map[string]any{
		"key": "whatsnew:0.108.0",
	}), http.StatusOK)
	if first["key"] != "whatsnew:0.108.0" {
		t.Fatalf("echoed key: %v", first)
	}
	at, _ := first["ack_at"].(string)
	if at == "" {
		t.Fatalf("no ack_at in %v", first)
	}

	again := c.expect(t, c.post("/users/me/acknowledgements", map[string]any{
		"key": "whatsnew:0.108.0",
	}), http.StatusOK)
	if again["ack_at"] != at {
		t.Fatalf("a repeated ack re-dated the flag: %v → %v", at, again["ack_at"])
	}

	list := ackList(t, c)
	if len(list) != 1 || list["whatsnew:0.108.0"] != at {
		t.Fatalf("list after two acks: %v", list)
	}
}

// The key is trimmed before it is stored, so a client that pads it does not get
// a second row for the same flag — and the padded form reads back trimmed.
func TestAcknowledgeTrimsKey(t *testing.T) {
	t.Parallel()
	c := signup(t)

	c.expect(t, c.post("/users/me/acknowledgements", map[string]any{"key": "spotlight:tags"}), http.StatusOK)
	c.expect(t, c.post("/users/me/acknowledgements", map[string]any{"key": "  spotlight:tags\n"}), http.StatusOK)

	list := ackList(t, c)
	if len(list) != 1 {
		t.Fatalf("padding the key made a second row: %v", list)
	}
	if _, ok := list["spotlight:tags"]; !ok {
		t.Fatalf("stored key is not the trimmed one: %v", list)
	}
}

// The bound is on characters, as the endpoint's own error says. Measured in
// bytes it refused any key past 200 *bytes*, which for Cyrillic is 100
// characters — half the documented allowance, and silently: the client gets a
// 400 for a key it built correctly, so the affordance it guards reappears on
// every launch.
func TestAcknowledgeKeyBounds(t *testing.T) {
	t.Parallel()
	c := signup(t)

	for _, bad := range []any{"", "   ", strings.Repeat("x", 201)} {
		r := c.post("/users/me/acknowledgements", map[string]any{"key": bad})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("key %q: expected 400, got %d\n%s", bad, r.Status, r.Body)
		}
	}

	atLimit := strings.Repeat("x", 200)
	c.expect(t, c.post("/users/me/acknowledgements", map[string]any{"key": atLimit}), http.StatusOK)

	// 200 characters of Cyrillic — 400 bytes, and every bit as legal a key.
	cyrillic := strings.Repeat("ф", 200)
	c.expect(t, c.post("/users/me/acknowledgements", map[string]any{"key": cyrillic}), http.StatusOK)
	// One character past the limit is refused whatever the alphabet.
	if r := c.post("/users/me/acknowledgements", map[string]any{
		"key": strings.Repeat("ф", 201),
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("201 Cyrillic characters: expected 400, got %d\n%s", r.Status, r.Body)
	}

	list := ackList(t, c)
	if _, ok := list[cyrillic]; !ok {
		t.Fatalf("the Cyrillic key did not survive the round trip: %d rows", len(list))
	}
}

// Flags are per user and the endpoint takes the caller from the token — there is
// no user id anywhere in the path, so this is the only thing keeping one
// account's Get Started state out of another's.
func TestAcknowledgementsAreScopedToTheCaller(t *testing.T) {
	t.Parallel()
	a, b := signup(t), signup(t)

	a.expect(t, a.post("/users/me/acknowledgements", map[string]any{"key": "getstarted:done"}), http.StatusOK)

	if list := ackList(t, b); len(list) != 0 {
		t.Fatalf("the other account sees %v", list)
	}
	b.expect(t, b.post("/users/me/acknowledgements", map[string]any{"key": "getstarted:step-1"}), http.StatusOK)

	listA, listB := ackList(t, a), ackList(t, b)
	if _, ok := listA["getstarted:step-1"]; ok {
		t.Fatalf("A picked up B's flag: %v", listA)
	}
	if _, ok := listB["getstarted:done"]; ok {
		t.Fatalf("B picked up A's flag: %v", listB)
	}
}

// Both endpoints are behind auth: an anonymous caller can neither read nor write
// flags.
func TestAcknowledgementsRequireAuth(t *testing.T) {
	t.Parallel()

	if r := doReq(t, "", http.MethodGet, "/users/me/acknowledgements", nil); r.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d\n%s", r.Status, r.Body)
	}
	if r := doReq(t, "", http.MethodPost, "/users/me/acknowledgements", map[string]any{
		"key": "whatsnew:0.1.0",
	}); r.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous ack: %d\n%s", r.Status, r.Body)
	}
}
