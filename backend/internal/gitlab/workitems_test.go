package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestWorkItemByIID_ReadsGIDAndParent(t *testing.T) {
	c := stubGraphQL(t, `{"data":{"project":{"workItems":{"nodes":[
      {"id":"gid://gitlab/WorkItem/771","widgets":[
        {},
        {"parent":{"id":"gid://gitlab/WorkItem/700"}}
      ]}
    ]}}}}`)
	wi, supported, err := c.WorkItemByIID(context.Background(), "grp/project", 12)
	if err != nil || !supported {
		t.Fatalf("WorkItemByIID = (%+v, %v, %v)", wi, supported, err)
	}
	if wi.GID != "gid://gitlab/WorkItem/771" {
		t.Errorf("GID = %q", wi.GID)
	}
	if wi.ParentGID != "gid://gitlab/WorkItem/700" {
		t.Errorf("ParentGID = %q", wi.ParentGID)
	}
}

func TestWorkItemByIID_TopLevelHasNoParent(t *testing.T) {
	c := stubGraphQL(t, `{"data":{"project":{"workItems":{"nodes":[
      {"id":"gid://gitlab/WorkItem/771","widgets":[{"parent":null}]}
    ]}}}}`)
	wi, supported, err := c.WorkItemByIID(context.Background(), "grp/project", 12)
	if err != nil || !supported {
		t.Fatalf("WorkItemByIID = (%+v, %v, %v)", wi, supported, err)
	}
	if wi.ParentGID != "" {
		t.Errorf("ParentGID = %q, want empty", wi.ParentGID)
	}
}

// An unknown iid (or a project the token cannot see) is not an error — there is simply
// nothing to attach, and the caller degrades instead of retrying forever.
func TestWorkItemByIID_MissingIsUnsupportedNotError(t *testing.T) {
	for name, body := range map[string]string{
		"no project":   `{"data":{"project":null}}`,
		"no such iid":  `{"data":{"project":{"workItems":{"nodes":[]}}}}`,
		"node with no id": `{"data":{"project":{"workItems":{"nodes":[{"id":"","widgets":[]}]}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := stubGraphQL(t, body)
			_, supported, err := c.WorkItemByIID(context.Background(), "grp/project", 12)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if supported {
				t.Fatal("supported = true, want false")
			}
		})
	}
}

func TestSetWorkItemParent_SendsIDsVerbatim(t *testing.T) {
	var got struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"workItemUpdate":{"errors":[],"workItem":{"id":"gid://gitlab/WorkItem/771","widgets":[{"parent":{"id":"gid://gitlab/WorkItem/700"}}]}}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	ok, err := c.SetWorkItemParent(context.Background(), "gid://gitlab/WorkItem/771", "gid://gitlab/WorkItem/700")
	if err != nil || !ok {
		t.Fatalf("SetWorkItemParent = (%v, %v)", ok, err)
	}
	// The gids must travel exactly as read from GitLab: rebuilding one from the issue
	// number ("gid://gitlab/Issue/771") is what the mutation rejects.
	if got.Variables["id"] != "gid://gitlab/WorkItem/771" {
		t.Errorf("id var = %v", got.Variables["id"])
	}
	if got.Variables["parentId"] != "gid://gitlab/WorkItem/700" {
		t.Errorf("parentId var = %v", got.Variables["parentId"])
	}
}

// Detaching is the same mutation with a null parentId — not a separate endpoint.
func TestSetWorkItemParent_DetachSendsNull(t *testing.T) {
	var got struct {
		Variables map[string]any `json:"variables"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"workItemUpdate":{"errors":[],"workItem":{"id":"gid://gitlab/WorkItem/771","widgets":[]}}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	ok, err := c.SetWorkItemParent(context.Background(), "gid://gitlab/WorkItem/771", "")
	if err != nil || !ok {
		t.Fatalf("SetWorkItemParent = (%v, %v)", ok, err)
	}
	if v, present := got.Variables["parentId"]; !present || v != nil {
		t.Errorf("parentId var = %v (present=%v), want explicit null", v, present)
	}
}

// An instance whose schema has no hierarchy widget must report supported=false, so the
// caller degrades ("child created, but not in GitLab's hierarchy") instead of retrying
// a mutation that can never succeed there.
func TestSetWorkItemParent_UnsupportedSchema(t *testing.T) {
	c := stubGraphQL(t, `{"errors":[{"message":"Field 'hierarchyWidget' doesn't exist on type 'WorkItemUpdateInput'"}]}`)
	ok, err := c.SetWorkItemParent(context.Background(), "gid://gitlab/WorkItem/771", "gid://gitlab/WorkItem/700")
	if err != nil {
		t.Fatalf("err = %v, want nil (unsupported, not failed)", err)
	}
	if ok {
		t.Fatal("supported = true, want false")
	}
}

// A mutation that ran and was refused (wrong type pair, no permission) is a real
// failure with a message worth surfacing — not silent degradation.
func TestSetWorkItemParent_MutationErrorsSurface(t *testing.T) {
	c := stubGraphQL(t, `{"data":{"workItemUpdate":{"errors":["only Task items can be children of an Issue"],"workItem":null}}}`)
	ok, err := c.SetWorkItemParent(context.Background(), "gid://gitlab/WorkItem/771", "gid://gitlab/WorkItem/700")
	if ok {
		t.Fatal("supported = true, want false on a refused mutation")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Status != 422 {
		t.Errorf("status = %d, want 422", apiErr.Status)
	}
}

func TestSetWorkItemParent_EmptyChildIsNoop(t *testing.T) {
	c := stubGraphQL(t, `{"data":{}}`)
	ok, err := c.SetWorkItemParent(context.Background(), "", "gid://gitlab/WorkItem/700")
	if ok || err != nil {
		t.Fatalf("SetWorkItemParent(\"\") = (%v, %v), want (false, nil)", ok, err)
	}
}

// CreateIssue must send issue_type=task so the new issue is eligible to be a child;
// GitLab refuses to nest a plain issue under another issue.
func TestCreateIssue_SendsIssueType(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":771,"iid":12,"web_url":"https://gl/x/-/issues/12","state":"opened"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.CreateIssue(context.Background(), "grp/project", "Child", "", nil, "", nil, "task"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got := form.Get("issue_type"); got != "task" {
		t.Errorf("issue_type = %q, want \"task\"", got)
	}
}

// The existing "create issue from task" path passes "" and must keep sending no
// issue_type at all, so GitLab applies its own default.
func TestCreateIssue_EmptyTypeOmitsParam(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":771,"iid":12,"web_url":"https://gl/x/-/issues/12","state":"opened"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.CreateIssue(context.Background(), "grp/project", "Plain", "", nil, "", nil, ""); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, present := form["issue_type"]; present {
		t.Errorf("issue_type sent as %q, want the param absent", form.Get("issue_type"))
	}
}
