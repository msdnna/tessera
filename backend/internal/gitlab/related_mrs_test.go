package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMRProjectPath(t *testing.T) {
	cases := []struct{ ref, url, want string }{
		{"grp/sub/api!12", "", "grp/sub/api"},
		{"", "https://gl.example.com/grp/web/-/merge_requests/7", "grp/web"},
		{"grp/api!3", "https://gl.example.com/other/x/-/merge_requests/3", "grp/api"}, // the reference wins
		{"!12", "", ""}, // a short reference names no project
		{"", "https://gl.example.com/grp/web/-/issues/7", ""},
	}
	for _, c := range cases {
		if got := mrProjectPath(c.ref, c.url); got != c.want {
			t.Errorf("mrProjectPath(%q, %q) = %q, want %q", c.ref, c.url, got, c.want)
		}
	}
	if MRKey("/Grp/API/", 12) != "grp/api!12" {
		t.Errorf("MRKey = %q", MRKey("/Grp/API/", 12))
	}
}

// The issue side: related_merge_requests and closed_by across projects, one MR
// per (project, iid) with the closing ones marked, and the head pipeline of the
// single-MR read (or of the deprecated "pipeline" field).
func TestIssueMergeRequestsAndMRByPath(t *testing.T) {
	mr := func(id, iid int64, full, extra string) string {
		return fmt.Sprintf(`{"id":%d,"iid":%d,"title":"MR %d","description":"","state":"opened","draft":false,
			"source_branch":"feat-%d","target_branch":"main","web_url":"https://gl/%s/-/merge_requests/%d",
			"updated_at":"2026-10-11T10:00:00+03:00","references":{"full":"%s!%d"}%s}`,
			id, iid, iid, iid, full, iid, full, iid, extra)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.EscapedPath()
		switch {
		case strings.HasSuffix(p, "/projects/scrum%2Fboard/issues/5/related_merge_requests"):
			fmt.Fprint(w, "["+mr(1, 12, "grp/api", "")+","+mr(2, 12, "grp/web", "")+","+mr(3, 4, "scrum/board", "")+"]")
		case strings.HasSuffix(p, "/projects/scrum%2Fboard/issues/5/closed_by"):
			fmt.Fprint(w, "["+mr(1, 12, "grp/api", "")+"]")
		case strings.HasSuffix(p, "/projects/grp%2Fapi/merge_requests/12"):
			fmt.Fprint(w, mr(1, 12, "grp/api", `,"head_pipeline":{"id":77,"status":"running","web_url":"https://gl/grp/api/-/pipelines/77"}`))
		case strings.HasSuffix(p, "/projects/grp%2Fweb/merge_requests/12"):
			fmt.Fprint(w, mr(2, 12, "grp/web", `,"pipeline":{"id":78,"status":"success","web_url":"u"}`))
		case strings.Contains(p, "/projects/secret%2F"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"403 Forbidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	ctx := context.Background()

	mrs, closes, err := c.IssueMergeRequests(ctx, "scrum/board", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(mrs) != 3 {
		t.Fatalf("mrs = %+v, want 3 (one per project and iid)", mrs)
	}
	if mrs[0].ProjectPath != "grp/api" || mrs[0].IID != 12 || mrs[0].GlobalID != "gid://gitlab/MergeRequest/1" ||
		mrs[0].SourceBranch != "feat-12" || mrs[0].UpdatedAt == nil || mrs[0].UpdatedAt.Hour() != 7 {
		t.Fatalf("first MR = %+v", mrs[0])
	}
	if mrs[1].ProjectPath != "grp/web" || mrs[2].ProjectPath != "scrum/board" {
		t.Fatalf("projects = %q, %q", mrs[1].ProjectPath, mrs[2].ProjectPath)
	}
	if !closes["grp/api!12"] || closes["grp/web!12"] || len(closes) != 1 {
		t.Fatalf("closes = %v", closes)
	}

	got, err := c.MergeRequestByPath(ctx, "grp/api", 12)
	if err != nil || got.Pipeline == nil || got.Pipeline.ID != 77 || got.Pipeline.Status != "running" {
		t.Fatalf("MergeRequestByPath = %+v, %v", got, err)
	}
	got, err = c.MergeRequestByPath(ctx, "grp/web", 12)
	if err != nil || got.Pipeline == nil || got.Pipeline.ID != 78 {
		t.Fatalf("deprecated pipeline field: %+v, %v", got, err)
	}

	_, err = c.MergeRequestByPath(ctx, "secret/repo", 1)
	if !IsNoAccess(err) {
		t.Fatalf("403 is not read as no access: %v", err)
	}
	if _, _, err = c.IssueMergeRequests(ctx, "gone/repo", 1); !IsNoAccess(err) {
		t.Fatalf("404 is not read as no access: %v", err)
	}
	if IsNoAccess(&APIError{Status: http.StatusBadGateway}) || IsNoAccess(errors.New("boom")) {
		t.Fatal("a server failure is not «no access»")
	}
}
