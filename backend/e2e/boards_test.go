//go:build e2e

// Board resolution by deep link. Both entry points are the ones a client hits
// with nothing but a URL in hand, so their answer has to say which workspace the
// board lives in — the client scopes its realtime filter and its member/tag
// loads by exactly that id (#2721).
package e2e

import (
	"net/http"
	"testing"
)

func TestBoardResolveCarriesWorkspaceID(t *testing.T) {
	s := startServer(t, nil)
	a := s.register(t, "board-scope")
	st := s.mkStack(t, a)

	// Slugs come from the board and its project, so read them back rather than
	// guessing how the server transliterates «Доска».
	board := expect(t, s.get(t, "/boards/"+st.Board, a.Access), http.StatusOK)
	if board["workspace_id"] != st.WS {
		t.Errorf("GET /boards/<id> workspace_id = %v, want %s", board["workspace_id"], st.WS)
	}
	// The board's own fields must survive being wrapped in the scope view.
	if board["id"] != st.Board || board["slug"] == nil || board["project_id"] != st.Project {
		t.Errorf("GET /boards/<id> lost board fields: %v", board)
	}

	proj := expect(t, s.get(t, "/projects/"+st.Project, a.Access), http.StatusOK)
	pSlug, _ := proj["slug"].(string)
	bSlug, _ := board["slug"].(string)
	if pSlug == "" || bSlug == "" {
		t.Fatalf("missing slugs: project %q board %q", pSlug, bSlug)
	}

	resolved := expect(t, s.get(t, "/board-by-slug?project="+pSlug+"&board="+bSlug, a.Access), http.StatusOK)
	if resolved["workspace_id"] != st.WS {
		t.Errorf("GET /board-by-slug workspace_id = %v, want %s", resolved["workspace_id"], st.WS)
	}
	if resolved["id"] != st.Board {
		t.Errorf("GET /board-by-slug resolved to %v, want %s", resolved["id"], st.Board)
	}
}
