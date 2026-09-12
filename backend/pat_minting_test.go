// Revoking a personal access token has to be the end of it (#2932).
//
// A PAT is the one credential this system hands out that is long-lived by
// design and lives at rest — in a CI secret, an MCP client's config file, a
// shell profile. The codebase accepts that on purpose: TestPasswordChangeKeepsPATs
// pins that a routine password rotation does not sweep them away, on the grounds
// that a PAT "has its own revoke button". That reasoning only holds while the
// button actually works, and it does not if the token it revokes could mint
// siblings first: the owner revokes the one they know about, the clones stay.
//
// So the minting endpoint is the one place a PAT must not reach. Everything else
// a PAT can do it can already do; issuing fresh credentials is different in kind,
// because it is what turns a leak into tenure.
package main

import (
	"net/http"
	"testing"
)

// TestPATCannotMintPATs is the whole finding: the headless credential may use
// the API, but not the credential factory.
func TestPATCannotMintPATs(t *testing.T) {
	t.Parallel()
	c := signup(t)
	pat := mkPAT(t, c, "ci-runner")

	// Sanity: the token works at all, so a refusal below is about the endpoint
	// and not about a broken fixture.
	if r := pat.get("/auth/me"); r.Status != http.StatusOK {
		t.Fatalf("PAT on /auth/me: status %d\n%s", r.Status, r.Body)
	}

	r := pat.post("/auth/tokens", map[string]any{"name": "second-hand"})
	if r.Status != http.StatusForbidden {
		t.Fatalf("PAT minted another PAT: status %d, want 403\n%s", r.Status, r.Body)
	}
	// A refused mint must not leave a token behind either — a 403 carrying a
	// working plaintext would be worse than no guard at all.
	if r.Status == http.StatusCreated {
		t.Fatalf("refused mint still returned a token: %s", r.Body)
	}

	// The session door stays open: this is a restriction on one credential kind,
	// not on the endpoint.
	if rr := c.post("/auth/tokens", map[string]any{"name": "from-session"}); rr.Status != http.StatusCreated {
		t.Fatalf("session minting a PAT: status %d, want 201\n%s", rr.Status, rr.Body)
	}
}

// TestPATRevocationIsFinal states the consequence directly, in the shape the
// owner experiences it: they notice the leak, they revoke the token, and the
// thief must be out. Without the guard above the thief has already minted a
// second token and this still passes for them.
func TestPATRevocationIsFinal(t *testing.T) {
	t.Parallel()
	c := signup(t)
	leaked := mkPAT(t, c, "leaked-into-a-build-log")

	// The thief's move, before anyone notices.
	minted := leaked.post("/auth/tokens", map[string]any{"name": "quietly-mine"})

	// The owner's move: find it in the list, revoke it.
	list := c.get("/auth/tokens")
	if list.Status != http.StatusOK {
		t.Fatalf("list tokens: status %d\n%s", list.Status, list.Body)
	}
	var id string
	for _, m := range list.listBody(t) {
		if m["name"] == "leaked-into-a-build-log" {
			id, _ = m["id"].(string)
		}
	}
	if id == "" {
		t.Fatalf("leaked token not in the owner's list")
	}
	if r := c.del("/auth/tokens/" + id); r.Status != http.StatusNoContent {
		t.Fatalf("revoke: status %d\n%s", r.Status, r.Body)
	}

	if r := leaked.get("/auth/me"); r.Status == http.StatusOK {
		t.Fatalf("revoked PAT still works: %s", r.Body)
	}
	// And nothing the thief made with it outlives the revocation, because they
	// were never able to make anything.
	if minted.Status == http.StatusCreated {
		token, _ := minted.mapBody(t)["token"].(string)
		if r := doReq(t, token, http.MethodGet, "/auth/me", nil); r.Status == http.StatusOK {
			t.Fatalf("a token minted by the leaked PAT outlived its revocation: %s", r.Body)
		}
	}
}
