package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestChangeAndCIState(t *testing.T) {
	for in, want := range map[string]string{
		"opened": ChangeOpen, "OPENED": ChangeOpen, "locked": ChangeOpen, "": ChangeOpen,
		"merged": ChangeMerged, "closed": ChangeClosed,
	} {
		if got := ChangeState(in); got != want {
			t.Errorf("ChangeState(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"success": CISuccess, "SUCCESS": CISuccess,
		"failed":  CIFailed,
		"running": CIRunning, "pending": CIRunning, "created": CIRunning, "preparing": CIRunning,
		"scheduled": CIRunning, "waiting_for_resource": CIRunning, "WAITING_FOR_CALLBACK": CIRunning,
		"canceled": CICanceled, "canceling": CICanceled, "skipped": CICanceled,
		"manual": CIManual,
		"":       "", "something_new": "",
	} {
		if got := CIState(in); got != want {
			t.Errorf("CIState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTextIssueRefs(t *testing.T) {
	const proj = "grp/app"
	cases := []struct {
		name               string
		text               string
		mentioned, closing []int64
	}{
		{"closing keyword", "Closes #12", []int64{12}, []int64{12}},
		{"closing list", "Fixes: #3, #4 and grp/app#5", []int64{3, 4, 5}, []int64{3, 4, 5}},
		{"plain mention", "Follow-up to #7 (see also #8)", []int64{7, 8}, nil},
		{"other project skipped", "Closes other/repo#9 and #10", []int64{10}, []int64{10}},
		{"qualified same project, any case", "Resolves GRP/App#11", []int64{11}, []int64{11}},
		{"issue URL", "Implements https://gl.example.com/grp/app/-/issues/21", []int64{21}, []int64{21}},
		{"URL of another project", "see https://gl.example.com/grp/other/-/issues/22", nil, nil},
		{"MR and epic are not issues", "after !14, part of &3", nil, nil},
		{"URL fragment is not a ref", "https://wiki.example.com/page#13", nil, nil},
		{"code is not a ref", "run `make #15`\n```\nexit #16\n```\nCloses #17", []int64{17}, []int64{17}},
		{"heading is not a ref", "# Summary\n## 2 steps", nil, nil},
		{"word-glued is not a ref", "abc#18", nil, nil},
		{"prefix is not a keyword", "prefix #19", []int64{19}, nil},
		{"duplicates collapse", "#20 #20, closes #20", []int64{20}, []int64{20}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, cl := TextIssueRefs(proj, c.text)
			if !slices.Equal(sorted(m), sorted(c.mentioned)) {
				t.Errorf("mentioned = %v, want %v", m, c.mentioned)
			}
			if !slices.Equal(sorted(cl), sorted(c.closing)) {
				t.Errorf("closing = %v, want %v", cl, c.closing)
			}
		})
	}
}

func sorted(s []int64) []int64 {
	c := slices.Clone(s)
	slices.Sort(c)
	return c
}

func TestBranchIssue(t *testing.T) {
	for in, want := range map[string]int64{
		"12-fix-login":        12,
		"feat/2964-gitlab-mr": 2964,
		"418-redesign":        418,
		"12-2fa":              12,
		"release/2-0":         0,
		"main":                0,
		"feat/board-v2":       0,
		"v1-2-3":              0,
		"":                    0,
	} {
		if got := BranchIssue(in); got != want {
			t.Errorf("BranchIssue(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIssueTargets(t *testing.T) {
	mr := MergeRequest{
		Title:        "Redesign board, refs #3",
		Description:  "Closes #2\n\nAlso touches #4",
		SourceBranch: "4-board",
	}
	got := IssueTargets("grp/app", mr, []int64{1, 3})
	want := map[int64]string{1: ViaCloses, 2: ViaCloses, 3: ViaCloses, 4: ViaMention}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IssueTargets = %v, want %v", got, want)
	}
	// Branch only: the weakest reason still links.
	got = IssueTargets("grp/app", MergeRequest{Title: "wip", SourceBranch: "9-thing"}, nil)
	if !reflect.DeepEqual(got, map[int64]string{9: ViaBranch}) {
		t.Fatalf("branch-only targets = %v", got)
	}
}

func TestParseMergeRequestHook(t *testing.T) {
	body := []byte(`{"object_kind":"merge_request","object_attributes":{"id":5501,"iid":418,
		"title":"Редизайн доски v2","description":"Closes #12","url":"https://gl/grp/app/-/merge_requests/418",
		"state":"opened","draft":true,"work_in_progress":false,"source_branch":"feat/board-v2","target_branch":"main",
		"updated_at":"2026-10-02 10:11:12 UTC"}}`)
	mr, err := ParseMergeRequestHook(body)
	if err != nil {
		t.Fatal(err)
	}
	if mr.IID != 418 || mr.GlobalID != "gid://gitlab/MergeRequest/5501" || !mr.Draft || mr.State != "opened" ||
		mr.SourceBranch != "feat/board-v2" || mr.TargetBranch != "main" || mr.Pipeline != nil {
		t.Fatalf("parsed = %+v", mr)
	}
	if mr.UpdatedAt == nil || !mr.UpdatedAt.Equal(time.Date(2026, 10, 2, 10, 11, 12, 0, time.UTC)) {
		t.Fatalf("updated_at = %v", mr.UpdatedAt)
	}
	// Older GitLab: no `draft`, only work_in_progress; RFC 3339 timestamps.
	old, err := ParseMergeRequestHook([]byte(`{"object_attributes":{"id":1,"iid":2,"state":"merged","work_in_progress":true,"updated_at":"2026-10-02T10:11:12Z"}}`))
	if err != nil || !old.Draft || old.UpdatedAt == nil {
		t.Fatalf("old payload = %+v, %v", old, err)
	}
	if _, err := ParseMergeRequestHook([]byte(`{"object_kind":"merge_request"}`)); err == nil {
		t.Fatal("a hook without attributes must not parse")
	}
}

func TestParsePipelineHook(t *testing.T) {
	ev, err := ParsePipelineHook([]byte(`{"object_kind":"pipeline","object_attributes":{"id":77,"ref":"feat/board-v2","status":"running"},
		"merge_request":{"iid":418},"project":{"web_url":"https://gl/grp/app"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev.MRIID != 418 || ev.Ref != "feat/board-v2" || ev.Pipeline.ID != 77 || ev.Pipeline.Status != "running" ||
		ev.Pipeline.WebURL != "https://gl/grp/app/-/pipelines/77" {
		t.Fatalf("parsed = %+v", ev)
	}
	branch, err := ParsePipelineHook([]byte(`{"object_attributes":{"id":78,"ref":"main","status":"SUCCESS","url":"https://gl/p/78"},"merge_request":null}`))
	if err != nil || branch.MRIID != 0 || branch.Pipeline.WebURL != "https://gl/p/78" || branch.Pipeline.Status != "success" {
		t.Fatalf("branch pipeline = %+v, %v", branch, err)
	}
	if _, err := ParsePipelineHook([]byte(`{"object_attributes":{}}`)); err == nil {
		t.Fatal("a hook without a pipeline id must not parse")
	}
}

// The poll query: pagination, the head pipeline, and the missing-connection
// tolerance (an instance that does not answer mergeRequests reads as "none").
func TestMergeRequestsSince(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Query, "mergeRequests(") {
			t.Errorf("unexpected query %s", req.Query)
		}
		if req.Variables["updatedAfter"] != "2026-10-01T00:00:00Z" {
			t.Errorf("updatedAfter = %v", req.Variables["updatedAfter"])
		}
		page := map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "c1"},
			"nodes": []any{map[string]any{
				"id": "gid://gitlab/MergeRequest/1", "iid": "418", "title": "A", "state": "opened", "draft": false,
				"sourceBranch": "feat/a", "targetBranch": "main",
				"headPipeline": map[string]any{"id": "gid://gitlab/Ci::Pipeline/90", "status": "SUCCESS", "path": "/grp/app/-/pipelines/90"},
			}},
		}
		if req.Variables["after"] == "c1" {
			page = map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes":    []any{map[string]any{"id": "gid://gitlab/MergeRequest/2", "iid": "419", "state": "merged"}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"project": map[string]any{"mergeRequests": page}}})
	}))
	defer srv.Close()
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mrs, err := New(srv.URL, "tok").MergeRequestsSince(context.Background(), "grp/app", &since)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(mrs) != 2 {
		t.Fatalf("calls=%d mrs=%d", calls, len(mrs))
	}
	p := mrs[0].Pipeline
	if mrs[0].IID != 418 || p == nil || p.ID != 90 || p.Status != "success" || p.WebURL != srv.URL+"/grp/app/-/pipelines/90" {
		t.Fatalf("first MR = %+v, pipeline %+v", mrs[0], p)
	}
	if mrs[1].State != "merged" || mrs[1].Pipeline != nil {
		t.Fatalf("second MR = %+v", mrs[1])
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"project": map[string]any{"issues": map[string]any{}}}})
	}))
	defer empty.Close()
	none, err := New(empty.URL, "tok").MergeRequestsSince(context.Background(), "grp/app", nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("no connection: %v, %v", none, err)
	}
}

func TestClosesIssuesDropsOtherProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/merge_requests/418/closes_issues") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]any{
			map[string]any{"iid": 12, "web_url": "https://gl/grp/app/-/issues/12"},
			map[string]any{"iid": 3, "web_url": "https://gl/grp/other/-/issues/3"},
		})
	}))
	defer srv.Close()
	got, err := New(srv.URL, "tok").ClosesIssues(context.Background(), "grp/app", 418)
	if err != nil || !slices.Equal(got, []int64{12}) {
		t.Fatalf("ClosesIssues = %v, %v", got, err)
	}
}
