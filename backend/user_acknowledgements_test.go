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

// ── clearing flags (#3100, #3101) ──

// ackSeed writes every key for the caller and fails the test on a refusal.
func ackSeed(t *testing.T, c *client, keys ...string) {
	t.Helper()
	for _, k := range keys {
		c.expect(t, c.post("/users/me/acknowledgements", map[string]any{"key": k}), http.StatusOK)
	}
}

// «Показать снова» on one hint clears exactly that key and nothing next to it —
// not even a longer key that starts with it.
func TestForgetOneAcknowledgement(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ackSeed(t, c, "spotlight:inbox", "spotlight:inbox-capture", "whatsnew:web:0.190.0")

	got := c.expect(t, c.del("/users/me/acknowledgements?key=spotlight:inbox"), http.StatusOK)
	if got["deleted"] != float64(1) {
		t.Fatalf("deleted: %v", got)
	}
	list := ackList(t, c)
	if _, ok := list["spotlight:inbox"]; ok {
		t.Fatalf("the key is still there: %v", list)
	}
	if _, ok := list["spotlight:inbox-capture"]; !ok {
		t.Fatalf("a longer key went with it: %v", list)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 flags left, got %v", list)
	}

	// Again: nothing to clear is not an error — a switch turned on twice.
	again := c.expect(t, c.del("/users/me/acknowledgements?key=spotlight:inbox"), http.StatusOK)
	if again["deleted"] != float64(0) {
		t.Fatalf("second delete: %v", again)
	}
}

// «Показать подсказки заново» clears the whole spotlight: namespace and leaves
// the rest — the read releases, the tour, and the switches of hint kinds
// (hints:off:*, #3101), which the user set on purpose and a reset of the hints
// themselves must not flip back.
func TestForgetAcknowledgementsByPrefix(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ackSeed(t, c,
		"spotlight:inbox", "spotlight:stage-strip", "spotlight:palette",
		"hints:off:ctx", "getstarted:done", "whatsnew:web:0.190.0",
	)

	got := c.expect(t, c.del("/users/me/acknowledgements?prefix=spotlight:"), http.StatusOK)
	if got["deleted"] != float64(3) {
		t.Fatalf("deleted: %v", got)
	}
	list := ackList(t, c)
	for _, k := range []string{"hints:off:ctx", "getstarted:done", "whatsnew:web:0.190.0"} {
		if _, ok := list[k]; !ok {
			t.Fatalf("%s went with the spotlight reset: %v", k, list)
		}
	}
	if len(list) != 3 {
		t.Fatalf("want 3 flags left, got %v", list)
	}
}

// The prefix is matched literally: a key with LIKE metacharacters in it can't
// widen a reset into other namespaces.
func TestForgetByPrefixIsLiteral(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ackSeed(t, c, "getstarted:item:basics.task", "getstarted_x:keep", "getstartedXitem:keep")

	c.expect(t, c.del("/users/me/acknowledgements?prefix=getstarted_%25:"), http.StatusOK)
	if list := ackList(t, c); len(list) != 3 {
		t.Fatalf("a pattern prefix matched something: %v", list)
	}
	got := c.expect(t, c.del("/users/me/acknowledgements?prefix=getstarted:"), http.StatusOK)
	if got["deleted"] != float64(1) {
		t.Fatalf("deleted: %v", got)
	}
}

// One of key/prefix, never both or neither, and a prefix must name a namespace:
// a blank or colon-less prefix would wipe every flag the account has.
func TestForgetAcknowledgementsValidates(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ackSeed(t, c, "spotlight:inbox", "whatsnew:web:0.190.0")

	for _, q := range []string{
		"",
		"?key=spotlight:inbox&prefix=spotlight:",
		"?prefix=",
		"?prefix=%20",
		"?prefix=s",
		"?prefix=spotlight",
		"?prefix=:",
		"?key=" + strings.Repeat("k", 201),
	} {
		if r := c.del("/users/me/acknowledgements" + q); r.Status != http.StatusBadRequest {
			t.Fatalf("%q: %d\n%s", q, r.Status, r.Body)
		}
	}
	if list := ackList(t, c); len(list) != 2 {
		t.Fatalf("a refused request cleared something: %v", list)
	}
}

// Clearing is per caller, like everything else here: B's reset leaves A's
// flags alone, and an anonymous caller is turned away.
func TestForgetAcknowledgementsIsScopedToTheCaller(t *testing.T) {
	t.Parallel()
	a, b := signup(t), signup(t)
	ackSeed(t, a, "spotlight:inbox")
	ackSeed(t, b, "spotlight:inbox")

	b.expect(t, b.del("/users/me/acknowledgements?prefix=spotlight:"), http.StatusOK)
	if list := ackList(t, a); len(list) != 1 {
		t.Fatalf("B's reset reached A: %v", list)
	}
	if r := doReq(t, "", http.MethodDelete, "/users/me/acknowledgements?key=spotlight:inbox", nil); r.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous delete: %d\n%s", r.Status, r.Body)
	}
}
