package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRelationKind(t *testing.T) {
	cases := []struct {
		linkType string
		want     string
		ok       bool
	}{
		// REST forms (/issues/:iid/links).
		{"relates_to", "relates", true},
		{"blocks", "blocks", true},
		{"is_blocked_by", "blocked_by", true},
		// GraphQL WorkItemRelatedLinkType enum — the actual values are RELATED /
		// BLOCKS / BLOCKED_BY (NOT the upper-case of the REST forms). Dropping these
		// is what made linked items vanish on modern GitLab (#2591 rework).
		{"RELATED", "relates", true},
		{"BLOCKS", "blocks", true},
		{"BLOCKED_BY", "blocked_by", true},
		{" blocks ", "blocks", true},
		{"", "", false},
		{"duplicates", "", false}, // GitLab has no such link type — never guess
	}
	for _, tc := range cases {
		got, ok := RelationKind(tc.linkType)
		if got != tc.want || ok != tc.ok {
			t.Errorf("RelationKind(%q) = (%q, %v), want (%q, %v)", tc.linkType, got, ok, tc.want, tc.ok)
		}
	}
}

func TestInverseLinkType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"blocks", "is_blocked_by"},
		{"is_blocked_by", "blocks"},
		{"relates_to", "relates_to"}, // symmetric
		{"RELATED", "relates_to"},    // GraphQL form → canonical REST inverse
		{"BLOCKS", "is_blocked_by"},
		{"BLOCKED_BY", "blocks"},
		{"duplicates", "duplicates"}, // unknown → unchanged
	}
	for _, tc := range cases {
		if got := InverseLinkType(tc.in); got != tc.want {
			t.Errorf("InverseLinkType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// stubGraphQL serves one canned GraphQL response.
func stubGraphQL(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok-123")
}

func TestLinkedItems_ParsesWidget(t *testing.T) {
	c := stubGraphQL(t, `{"data":{"project":{"workItems":{"nodes":[
      {"iid":"1","widgets":[
        {},
        {"linkedItems":{"nodes":[
          {"linkType":"blocks","linkId":"gid://gitlab/IssueLink/5","workItem":{"iid":"2","webUrl":"https://gl/g/p/-/issues/2","namespace":{"fullPath":"g/p"}}},
          {"linkType":"relates_to","linkId":"gid://gitlab/IssueLink/6","workItem":{"iid":"9","webUrl":"https://gl/g/other/-/issues/9","namespace":{"fullPath":"g/other"}}}
        ]}}
      ]},
      {"iid":"2","widgets":[{"linkedItems":{"nodes":[]}}]}
    ]}}}}`)
	got, supported, err := c.LinkedItems(context.Background(), "g/p", []string{"1", "2"})
	if err != nil {
		t.Fatalf("LinkedItems: %v", err)
	}
	if !supported {
		t.Fatal("supported = false, want true (the widget was present)")
	}
	if len(got[1]) != 2 {
		t.Fatalf("issue #1 links = %d, want 2", len(got[1]))
	}
	if got[1][0].LinkType != "blocks" || got[1][0].IID != 2 || got[1][0].ProjectPath != "g/p" {
		t.Errorf("first link = %+v", got[1][0])
	}
	if got[1][0].LinkID != "gid://gitlab/IssueLink/5" {
		t.Errorf("link id = %q", got[1][0].LinkID)
	}
	// A cross-project link keeps the other project's path, not the source's.
	if got[1][1].ProjectPath != "g/other" || got[1][1].IID != 9 {
		t.Errorf("cross-project link = %+v", got[1][1])
	}
	if len(got[2]) != 0 {
		t.Errorf("issue #2 links = %d, want 0", len(got[2]))
	}
}

// An instance whose schema predates the widget answers with the work items but no
// linkedItems member — the caller must be told to fall back to REST rather than
// concluding there are no links.
func TestLinkedItems_WidgetAbsentReportsUnsupported(t *testing.T) {
	c := stubGraphQL(t, `{"data":{"project":{"workItems":{"nodes":[
      {"iid":"1","widgets":[{}]}
    ]}}}}`)
	got, supported, err := c.LinkedItems(context.Background(), "g/p", []string{"1"})
	if err != nil {
		t.Fatalf("LinkedItems: %v", err)
	}
	if supported {
		t.Error("supported = true, want false")
	}
	if len(got) != 0 {
		t.Errorf("links = %v, want none", got)
	}
}

func TestLinkedItems_NoIIDsSkipsRequest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	got, supported, err := c.LinkedItems(context.Background(), "g/p", nil)
	if err != nil || !supported || len(got) != 0 {
		t.Fatalf("LinkedItems(nil) = (%v, %v, %v)", got, supported, err)
	}
	if calls != 0 {
		t.Errorf("made %d GitLab calls for an empty batch, want 0", calls)
	}
}

func TestIssueLinksREST(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`[
          {"iid":2,"issue_link_id":5,"link_type":"blocks","web_url":"https://gl/g/p/-/issues/2","references":{"full":"g/p#2"}},
          {"iid":9,"issue_link_id":6,"link_type":"relates_to","web_url":"https://gl/g/other/-/issues/9","references":{"full":"g/other#9"}}
        ]`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	got, err := c.IssueLinksREST(context.Background(), "g/p", 1)
	if err != nil {
		t.Fatalf("IssueLinksREST: %v", err)
	}
	if gotPath != "/api/v4/projects/g%2Fp/issues/1/links" {
		t.Errorf("path = %s", gotPath)
	}
	if len(got) != 2 {
		t.Fatalf("links = %d, want 2", len(got))
	}
	if got[0].LinkType != "blocks" || got[0].IID != 2 || got[0].ProjectPath != "g/p" || got[0].LinkID != "5" {
		t.Errorf("first link = %+v", got[0])
	}
	if got[1].ProjectPath != "g/other" {
		t.Errorf("cross-project path = %q, want g/other", got[1].ProjectPath)
	}
}

// Without a references block there is nothing to qualify the project with, so the
// link is attributed to the project it was read from.
func TestParseIssueLinksREST_FallsBackToSourceProject(t *testing.T) {
	got, err := parseIssueLinksREST([]byte(`[{"iid":3,"link_type":"is_blocked_by"}]`), "g/p")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || got[0].ProjectPath != "g/p" || got[0].IID != 3 || got[0].LinkID != "" {
		t.Errorf("got %+v", got)
	}
}
