package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Project milestones over REST v4 (#3022): read one milestone and change it —
// title, description, dates and state. The pull still learns milestones through
// the issues that carry them (GraphQL, client.go); these two calls serve the
// write path, which must compare against GitLab's *current* value before it
// overwrites anything (three-way, like the issue write-back).

// Milestone is a project milestone as REST v4 reports it. Dates are
// "YYYY-MM-DD" or "" when unset; State is "active" or "closed".
type Milestone struct {
	ID          int64
	IID         int64
	Title       string
	Description string
	State       string
	StartDate   string
	DueDate     string
	WebURL      string
}

// GlobalID is the GraphQL global id ("gid://gitlab/Milestone/<id>").
func (m Milestone) GlobalID() string {
	return "gid://gitlab/Milestone/" + strconv.FormatInt(m.ID, 10)
}

// milestoneJSON is the REST shape; the dates are nullable strings.
type milestoneJSON struct {
	ID          int64   `json:"id"`
	IID         int64   `json:"iid"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	State       string  `json:"state"`
	StartDate   *string `json:"start_date"`
	DueDate     *string `json:"due_date"`
	WebURL      string  `json:"web_url"`
}

func (j milestoneJSON) toMilestone() Milestone {
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	return Milestone{
		ID: j.ID, IID: j.IID, Title: j.Title, Description: deref(j.Description),
		State: strings.ToLower(j.State), StartDate: deref(j.StartDate), DueDate: deref(j.DueDate),
		WebURL: j.WebURL,
	}
}

func milestonePath(projectPath string, id int64) string {
	return "projects/" + projectPathEsc(projectPath) + "/milestones/" + strconv.FormatInt(id, 10)
}

// GetProjectMilestone reads one project milestone by its global numeric id (the
// id REST takes; not the per-project iid).
func (c *Client) GetProjectMilestone(ctx context.Context, projectPath string, id int64) (Milestone, error) {
	body, err := c.restGet(ctx, milestonePath(projectPath, id))
	if err != nil {
		return Milestone{}, err
	}
	var j milestoneJSON
	if uerr := json.Unmarshal(body, &j); uerr != nil {
		return Milestone{}, uerr
	}
	return j.toMilestone(), nil
}

// MilestoneUpdate names the fields one PUT changes. A nil pointer leaves the
// field alone; a pointer to "" clears a date (GitLab casts the empty string to
// null) or empties the description. StateEvent is "close", "activate" or "" for
// no state change.
type MilestoneUpdate struct {
	Title       *string
	Description *string
	StartDate   *string
	DueDate     *string
	StateEvent  string
}

// Empty reports whether the update would change nothing.
func (u MilestoneUpdate) Empty() bool {
	return u.Title == nil && u.Description == nil && u.StartDate == nil && u.DueDate == nil && u.StateEvent == ""
}

// UpdateProjectMilestone changes a project milestone and returns it as GitLab
// stored it.
func (c *Client) UpdateProjectMilestone(ctx context.Context, projectPath string, id int64, u MilestoneUpdate) (Milestone, error) {
	form := url.Values{}
	if u.Title != nil {
		form.Set("title", *u.Title)
	}
	if u.Description != nil {
		form.Set("description", *u.Description)
	}
	if u.StartDate != nil {
		form.Set("start_date", *u.StartDate)
	}
	if u.DueDate != nil {
		form.Set("due_date", *u.DueDate)
	}
	if u.StateEvent != "" {
		form.Set("state_event", u.StateEvent)
	}
	body, err := c.restForm(ctx, http.MethodPut, milestonePath(projectPath, id), form)
	if err != nil {
		return Milestone{}, err
	}
	var j milestoneJSON
	if uerr := json.Unmarshal(body, &j); uerr != nil {
		return Milestone{}, uerr
	}
	return j.toMilestone(), nil
}
