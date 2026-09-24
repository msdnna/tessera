// A refusal must be ONE JSON document, not two glued together (#2932).
//
// `notFound(c, err)` is a writer, not a predicate: it answers the request with
// 404 {"error":"not found"} and reports that it did. Two routes read it as a
// question — «is this a no-rows error?» — and, on true, wrote their own, better
// sentence right after it. gin does not replace the first body; it appends, so
// the wire carried `{"error":"not found"}{"error":"задача #5 не найдена"}`.
//
// Nothing downstream survives that. axios' default transform calls JSON.parse,
// which throws on the trailing document, and hands the caller the raw string
// instead of an object — `refusalText` then reads `.error` off a string, gets
// undefined, and the toast falls back to axios' own «Request failed with status
// code 404» (the exact English-in-a-Russian-UI failure that the blob branch of
// refusalText was written to stop). Gson does the same on Android: fromJson
// refuses a document that is not fully consumed.
//
// So the localized sentence both routes went out of their way to write — the one
// naming WHICH task number or email is missing — was unreachable from either
// client, and the price of trying was a malformed body.
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// soleError decodes a refusal body that must be exactly one JSON object and
// returns its `error` field. Uses a Decoder rather than Unmarshal on purpose:
// Unmarshal already rejects trailing data, but the decoder can say whether the
// FIRST document parsed and a second one followed — which is the actual defect,
// and a plain «invalid character '{'» would hide it.
func soleError(t *testing.T, body []byte) string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	var first map[string]any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("refusal body is not JSON: %v\n%s", err, body)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == nil {
		t.Fatalf("refusal carries a SECOND JSON document (%s) after the first (%v) — "+
			"JSON.parse/Gson reject the whole body, so neither client can read the message\n%s",
			extra, first, body)
	}
	s, _ := first["error"].(string)
	return s
}

// TestRelationToMissingNumberRefusesOnce pins the «Связи» tab's refusal: linking
// to a number nobody has must answer one document, naming that number.
func TestRelationToMissingNumberRefusesOnce(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	colID := s.col(t, 0)

	src := mkTask(t, c, s.Board, colID, "Источник связи")
	srcID := src["id"].(string)

	// A number far past anything this workspace has issued.
	r := c.post("/tasks/"+srcID+"/relations", map[string]any{"number": 999999, "kind": "relates"})
	if r.Status != http.StatusNotFound {
		t.Fatalf("status %d, want 404\n%s", r.Status, r.Body)
	}
	msg := soleError(t, r.Body)
	if msg != "задача #999999 не найдена" {
		t.Fatalf("error = %q, want the sentence naming the number — the generic "+
			"«not found» is the one the handler tried to replace", msg)
	}
}

// TestAddMemberUnknownEmailRefusesOnce is the same wound on the members dialog:
// the route tells the manager the address matches nobody, and that has to arrive
// as the only body.
func TestAddMemberUnknownEmailRefusesOnce(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	r := c.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": "nobody-at-all@example.invalid", "role": "member"})
	if r.Status != http.StatusNotFound {
		t.Fatalf("status %d, want 404\n%s", r.Status, r.Body)
	}
	if msg := soleError(t, r.Body); msg != "no user with that email" {
		t.Fatalf("error = %q, want %q", msg, "no user with that email")
	}
}
