package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The read returns GitLab's current milestone — null dates and description read
// as empty, the state is lower-cased.
func TestGetProjectMilestone(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"id":41,"iid":4,"title":"Sprint 1","description":null,
			"state":"Active","start_date":"2026-08-01","due_date":null,"web_url":"https://g/x/-/milestones/4"}`))
	}))
	t.Cleanup(srv.Close)
	m, err := New(srv.URL, "tok").GetProjectMilestone(context.Background(), "grp/project", 41)
	if err != nil {
		t.Fatalf("GetProjectMilestone: %v", err)
	}
	if path != "/api/v4/projects/grp%2Fproject/milestones/41" {
		t.Errorf("path = %q", path)
	}
	want := Milestone{ID: 41, IID: 4, Title: "Sprint 1", State: "active", StartDate: "2026-08-01", WebURL: "https://g/x/-/milestones/4"}
	if m != want {
		t.Errorf("milestone = %+v, want %+v", m, want)
	}
	if m.GlobalID() != "gid://gitlab/Milestone/41" {
		t.Errorf("GlobalID = %q", m.GlobalID())
	}
}

// Only the fields the update names reach GitLab; an empty date clears it; the
// state goes as state_event.
func TestUpdateProjectMilestone_SendsOnlyNamedFields(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusOK, &got)
	desc, due := "## Goals", ""
	if _, err := c.WithSudo("ann").UpdateProjectMilestone(context.Background(), "grp/project", 41, MilestoneUpdate{
		Description: &desc, DueDate: &due, StateEvent: "close",
	}); err != nil {
		t.Fatalf("UpdateProjectMilestone: %v", err)
	}
	if got.method != http.MethodPut || got.path != "/api/v4/projects/grp%2Fproject/milestones/41" {
		t.Errorf("request = %s %s", got.method, got.path)
	}
	if got.sudo != "ann" {
		t.Errorf("Sudo = %q, want ann", got.sudo)
	}
	want := map[string]string{"description": "## Goals", "due_date": "", "state_event": "close"}
	if len(got.form) != len(want) {
		t.Fatalf("form = %v, want %v", got.form, want)
	}
	for k, v := range want {
		if gv, ok := got.form[k]; !ok || gv != v {
			t.Errorf("form[%s] = %q (present %v), want %q", k, gv, ok, v)
		}
	}
}

// A refusal comes back as an *APIError carrying the status.
func TestUpdateProjectMilestone_Error(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusForbidden, &got)
	title := "x"
	_, err := c.UpdateProjectMilestone(context.Background(), "grp/project", 41, MilestoneUpdate{Title: &title})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		t.Fatalf("err = %v, want APIError 403", err)
	}
}

func TestMilestoneUpdate_Empty(t *testing.T) {
	if !(MilestoneUpdate{}).Empty() {
		t.Error("zero update should be empty")
	}
	s := ""
	if (MilestoneUpdate{StartDate: &s}).Empty() {
		t.Error("clearing a date is a change")
	}
}
