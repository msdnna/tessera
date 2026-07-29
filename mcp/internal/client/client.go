// Package client is a thin, read-only HTTP client for the Tessera REST API.
// It authenticates with a Personal Access Token (tsra_… bearer) and never
// touches the database directly — the API is the integration seam.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tessera-mcp/internal/model"
)

// Client talks to a Tessera backend over HTTP.
type Client struct {
	baseURL string // e.g. http://localhost:8090/api
	token   string // personal access token (tsra_…)
	http    *http.Client
}

// New builds a client for the given API base URL and token. Timeouts mirror the
// backend's GitLab client: short dial/handshake so an unreachable host fails
// fast, with a generous overall ceiling for larger list responses.
func New(baseURL, token string) *Client {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second, Transport: tr},
	}
}

// WebBaseURL returns the human-facing base (API base minus the trailing /api),
// used to build /project/<slug>/board/<slug> deep links.
func (c *Client) WebBaseURL() string {
	return strings.TrimSuffix(c.baseURL, "/api")
}

// get performs an authenticated GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ListWorkspaces returns the workspaces the token's owner belongs to.
func (c *Client) ListWorkspaces(ctx context.Context) ([]model.Workspace, error) {
	var out []model.Workspace
	return out, c.get(ctx, "/workspaces", nil, &out)
}

// ListProjects returns the projects in a workspace.
func (c *Client) ListProjects(ctx context.Context, workspaceID string) ([]model.Project, error) {
	var out []model.Project
	return out, c.get(ctx, "/workspaces/"+workspaceID+"/projects", nil, &out)
}

// GetProject returns a single project.
func (c *Client) GetProject(ctx context.Context, projectID string) (model.Project, error) {
	var out model.Project
	return out, c.get(ctx, "/projects/"+projectID, nil, &out)
}

// ListBoards returns the boards in a project.
func (c *Client) ListBoards(ctx context.Context, projectID string) ([]model.Board, error) {
	var out []model.Board
	return out, c.get(ctx, "/projects/"+projectID+"/boards", nil, &out)
}

// GetBoard returns a single board (accepts a UUID id).
func (c *Client) GetBoard(ctx context.Context, boardID string) (model.Board, error) {
	var out model.Board
	return out, c.get(ctx, "/boards/"+boardID, nil, &out)
}

// ResolveBoardBySlug maps a /project/<slug>/board/<slug> pair to its board.
func (c *Client) ResolveBoardBySlug(ctx context.Context, projectSlug, boardSlug string) (model.Board, error) {
	q := url.Values{"project": {projectSlug}, "board": {boardSlug}}
	var out model.Board
	return out, c.get(ctx, "/board-by-slug", q, &out)
}

// ListColumns returns a board's columns, ordered by position.
func (c *Client) ListColumns(ctx context.Context, boardID string) ([]model.Column, error) {
	var out []model.Column
	return out, c.get(ctx, "/boards/"+boardID+"/columns", nil, &out)
}

// ListProjectTags returns a project's tags.
func (c *Client) ListProjectTags(ctx context.Context, projectID string) ([]model.Tag, error) {
	var out []model.Tag
	return out, c.get(ctx, "/projects/"+projectID+"/tags", nil, &out)
}

// ListWorkspaceTags returns every tag across a workspace's projects.
func (c *Client) ListWorkspaceTags(ctx context.Context, workspaceID string) ([]model.Tag, error) {
	var out []model.Tag
	return out, c.get(ctx, "/workspaces/"+workspaceID+"/tags", nil, &out)
}

// ListBoardTasks returns a board's top-level tasks. milestone is optional: a
// UUID scopes to that milestone, "backlog" limits to tasks without one, and ""
// returns all.
func (c *Client) ListBoardTasks(ctx context.Context, boardID, milestone string) ([]model.Task, error) {
	var q url.Values
	if milestone != "" {
		q = url.Values{"milestone": {milestone}}
	}
	var out []model.Task
	return out, c.get(ctx, "/boards/"+boardID+"/tasks", q, &out)
}

// ListWorkspaceTasks returns tasks across a workspace. When assigneeMe is true
// the server scopes to tasks assigned to the token's owner.
func (c *Client) ListWorkspaceTasks(ctx context.Context, workspaceID string, assigneeMe bool) ([]model.Task, error) {
	var q url.Values
	if assigneeMe {
		q = url.Values{"assignee": {"me"}}
	}
	var out []model.Task
	return out, c.get(ctx, "/workspaces/"+workspaceID+"/tasks", q, &out)
}

// GetTask returns the full detail of a task by UUID.
func (c *Client) GetTask(ctx context.Context, taskID string) (model.TaskDetail, error) {
	var out model.TaskDetail
	return out, c.get(ctx, "/tasks/"+taskID, nil, &out)
}

// GetTaskByNumber resolves a per-workspace task number (e.g. #252) to its detail.
func (c *Client) GetTaskByNumber(ctx context.Context, workspaceID string, number int64) (model.TaskDetail, error) {
	var out model.TaskDetail
	return out, c.get(ctx, fmt.Sprintf("/workspaces/%s/tasks/by-number/%d", workspaceID, number), nil, &out)
}

// ListBoardDependencies returns the blocks/blocked_by links among a board's tasks.
func (c *Client) ListBoardDependencies(ctx context.Context, boardID string) ([]model.Dependency, error) {
	var out []model.Dependency
	return out, c.get(ctx, "/boards/"+boardID+"/dependencies", nil, &out)
}

// ListComments returns a task's comments.
func (c *Client) ListComments(ctx context.Context, taskID string) ([]model.Comment, error) {
	var out []model.Comment
	return out, c.get(ctx, "/tasks/"+taskID+"/comments", nil, &out)
}
