package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A page of issues carries every discussion and note, so on a busy project GitLab
// gives up on its own deadline ("Timeout on Discussion.id"). The sync used to hand
// that straight back and fail the same way every minute; it now asks again with a
// smaller page and carries on from the same cursor.
func TestQueryIssues_HalvesPageOnGraphQLTimeout(t *testing.T) {
	var firsts []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		first := int(req.Variables["first"].(float64))
		firsts = append(firsts, first)
		w.Header().Set("Content-Type", "application/json")
		if first > 12 {
			_, _ = w.Write([]byte(`{"data":{"project":null},"errors":[{"message":"Timeout on Discussion.id"}]}`))
			return
		}
		after, _ := req.Variables["after"].(string)
		if after == "" {
			_, _ = fmt.Fprint(w, `{"data":{"project":{"issues":{"pageInfo":{"hasNextPage":true,"endCursor":"c1"},
				"nodes":[{"id":"gid://gitlab/Issue/1","iid":"1","title":"one"}]}}}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":{"project":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":""},
			"nodes":[{"id":"gid://gitlab/Issue/2","iid":"2","title":"two"}]}}}}`)
	}))
	t.Cleanup(srv.Close)

	issues, err := New(srv.URL, "tok").AllIssues(context.Background(), "grp/proj")
	if err != nil {
		t.Fatalf("AllIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}
	// 50 and 25 time out, 12 succeeds and stays for the next page.
	want := []int{50, 25, 12, 12}
	if fmt.Sprint(firsts) != fmt.Sprint(want) {
		t.Fatalf("page sizes = %v, want %v", firsts, want)
	}

	// The next run (a fresh Client, as every sync makes) starts where this one
	// ended instead of timing out through 50 and 25 again.
	firsts = nil
	if _, err := New(srv.URL, "tok").AllIssues(context.Background(), "grp/proj"); err != nil {
		t.Fatalf("second AllIssues: %v", err)
	}
	if fmt.Sprint(firsts) != fmt.Sprint([]int{12, 12}) {
		t.Fatalf("second run page sizes = %v, want [12 12]", firsts)
	}
}

func TestQueryIssues_GivesUpAtSmallestPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"errors":[{"message":"Timeout on Note.id"}]}`))
	}))
	t.Cleanup(srv.Close)

	_, err := New(srv.URL, "tok").AllIssues(context.Background(), "grp/proj")
	if err == nil || !strings.Contains(err.Error(), "Timeout on Note.id") {
		t.Fatalf("err = %v, want the GitLab timeout", err)
	}
	// 50 → 25 → 12 → 6 → 3 → 1: one request per size, then the error surfaces.
	if calls != 6 {
		t.Fatalf("calls = %d, want 6", calls)
	}
}

func TestQueryIssues_OtherErrorsAreNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"errors":[{"message":"Field 'x' doesn't exist"}]}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := New(srv.URL, "tok").AllIssues(context.Background(), "grp/proj"); err == nil {
		t.Fatal("want an error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
