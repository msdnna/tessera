// Package client is a thin, read-only HTTP client for the Tessera REST API.
// It authenticates with a Personal Access Token (tsra_… bearer) and never
// touches the database directly — the API is the integration seam.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"

	"tessera-mcp/internal/model"
)

// Client talks to a Tessera backend over HTTP.
type Client struct {
	baseURL string // e.g. http://localhost:8090/api
	token   string // personal access token (tsra_…)
	http    *http.Client

	selfMu     sync.Mutex // guards the memoised /auth/me lookup
	selfLoaded bool
	selfID     string
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

// send performs an authenticated request with an optional JSON body and decodes
// the JSON response into out (out may be nil). Any 2xx is accepted.
func (c *Client) send(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, method, path, out)
}

// do executes a prepared request, treating any non-2xx as an error, and decodes
// the body into out when non-nil.
func (c *Client) do(req *http.Request, method, path string, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SelfID returns the user id owning the token (via /auth/me), memoised for the
// process. Best-effort: returns "" if the lookup fails, so callers must treat an
// empty id as "unknown" rather than a match.
func (c *Client) SelfID(ctx context.Context) string {
	c.selfMu.Lock()
	defer c.selfMu.Unlock()
	if c.selfLoaded {
		return c.selfID
	}
	c.selfLoaded = true
	var out struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := c.get(ctx, "/auth/me", nil, &out); err == nil {
		c.selfID = out.User.ID
	}
	return c.selfID
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

// ListAttachments returns a task's file attachments.
func (c *Client) ListAttachments(ctx context.Context, taskID string) ([]model.Attachment, error) {
	var out []model.Attachment
	return out, c.get(ctx, "/tasks/"+taskID+"/attachments", nil, &out)
}

// ListMembers returns a workspace's members (for resolving assignees by name/email).
func (c *Client) ListMembers(ctx context.Context, workspaceID string) ([]model.Member, error) {
	var out []model.Member
	return out, c.get(ctx, "/workspaces/"+workspaceID+"/members", nil, &out)
}

// ── writes ────────────────────────────────────────────────────────────────────

// CreateComment posts a Markdown comment on a task (or subtask) and returns it.
func (c *Client) CreateComment(ctx context.Context, taskID, body string) (model.Comment, error) {
	var out model.Comment
	return out, c.send(ctx, http.MethodPost, "/tasks/"+taskID+"/comments",
		map[string]any{"body": body}, &out)
}

// MoveTask moves a task to a column. Position is left to the server (appended);
// note the backend auto-completes a task moved into the board's done column.
func (c *Client) MoveTask(ctx context.Context, taskID, columnID string) error {
	return c.send(ctx, http.MethodPatch, "/tasks/"+taskID+"/move",
		map[string]any{"column_id": columnID}, nil)
}

// AddAssignee assigns a user to a task (idempotent on the backend).
func (c *Client) AddAssignee(ctx context.Context, taskID, userID string) error {
	return c.send(ctx, http.MethodPost, "/tasks/"+taskID+"/assignees",
		map[string]any{"user_id": userID}, nil)
}

// RemoveAssignee unassigns a user from a task.
func (c *Client) RemoveAssignee(ctx context.Context, taskID, userID string) error {
	return c.send(ctx, http.MethodDelete, "/tasks/"+taskID+"/assignees/"+userID, nil, nil)
}

// TaskPatch is a partial body for PATCH /tasks/:id, which is tri-state: a field
// the body doesn't carry is left alone, and an explicit null clears the
// nullable ones (due_date, start_date, estimate). Building the body field by
// field means a partial edit can't clobber a value it never mentioned — the
// full-replace shape this replaced re-sent every field and lost a concurrent
// edit made in between.
type TaskPatch map[string]any

// Set marks a field for update. Clear sends an explicit null, which only
// due_date, start_date and estimate honour — for the others the backend reads
// null as "absent" (see optJSON.ptr), so an empty string is how you blank a
// description.
func (p TaskPatch) Set(field string, v any) TaskPatch { p[field] = v; return p }
func (p TaskPatch) Clear(field string) TaskPatch      { p[field] = nil; return p }

// UpdateTask applies a partial task update; an empty patch is a no-op.
func (c *Client) UpdateTask(ctx context.Context, taskID string, p TaskPatch) error {
	if len(p) == 0 {
		return nil
	}
	return c.send(ctx, http.MethodPatch, "/tasks/"+taskID, map[string]any(p), nil)
}

// NewTaskParams is the body of POST /boards/:id/tasks. ColumnID and Title are
// required; the backend derives position and the workspace task number itself,
// and ignores tags/assignees (those are separate calls after creation).
type NewTaskParams struct {
	ColumnID    string     `json:"column_id"`
	ParentID    *string    `json:"parent_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Priority    int        `json:"priority,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	Estimate    *float64   `json:"estimate,omitempty"`
}

// CreateTask adds a task (or, with ParentID, a subtask) to a board.
func (c *Client) CreateTask(ctx context.Context, boardID string, p NewTaskParams) (model.Task, error) {
	var out model.Task
	return out, c.send(ctx, http.MethodPost, "/boards/"+boardID+"/tasks", p, &out)
}

// SetParent re-parents a task; a nil parentID detaches it back to a top-level
// card. A task attached to a parent inherits the parent's board and column.
func (c *Client) SetParent(ctx context.Context, taskID string, parentID *string) error {
	return c.send(ctx, http.MethodPatch, "/tasks/"+taskID+"/parent",
		map[string]any{"parent_id": parentID}, nil)
}

// CreateTag adds a project-scoped tag (color may be "" for the backend default).
func (c *Client) CreateTag(ctx context.Context, projectID, name, color string) (model.Tag, error) {
	var out model.Tag
	return out, c.send(ctx, http.MethodPost, "/projects/"+projectID+"/tags",
		map[string]any{"name": name, "color": color}, &out)
}

// AddTaskTag attaches a tag to a task (idempotent on the backend).
func (c *Client) AddTaskTag(ctx context.Context, taskID, tagID string) error {
	return c.send(ctx, http.MethodPost, "/tasks/"+taskID+"/tags",
		map[string]any{"tag_id": tagID}, nil)
}

// RemoveTaskTag detaches a tag from a task.
func (c *Client) RemoveTaskTag(ctx context.Context, taskID, tagID string) error {
	return c.send(ctx, http.MethodDelete, "/tasks/"+taskID+"/tags/"+tagID, nil, nil)
}

// ListRelations returns the relations recorded *on* a task. Relations are
// stored one-way, so a link between two tasks may live on either end — see
// DeleteRelation's callers.
func (c *Client) ListRelations(ctx context.Context, taskID string) ([]model.Relation, error) {
	var out []model.Relation
	return out, c.get(ctx, "/tasks/"+taskID+"/relations", nil, &out)
}

// AddRelation links a task to another one identified by its workspace #number.
func (c *Client) AddRelation(ctx context.Context, taskID string, number int64, kind string) error {
	return c.send(ctx, http.MethodPost, "/tasks/"+taskID+"/relations",
		map[string]any{"number": number, "kind": kind}, nil)
}

// DeleteRelation removes a single relation row by its id.
func (c *Client) DeleteRelation(ctx context.Context, relationID string) error {
	return c.send(ctx, http.MethodDelete, "/relations/"+relationID, nil, nil)
}

// UploadAttachment posts a file to a task's attachments and returns the record.
func (c *Client) UploadAttachment(ctx context.Context, taskID, filename, mimeType string, data []byte) (model.Attachment, error) {
	var out model.Attachment
	return out, c.postMultipart(ctx, "/tasks/"+taskID+"/attachments", filename, mimeType, data, &out)
}

// DownloadAttachment streams an attachment's bytes into w and returns how many
// were written. Streaming (rather than returning a []byte like FetchImage) keeps
// a 25 MiB attachment off the heap.
func (c *Client) DownloadAttachment(ctx context.Context, attachmentID string, w io.Writer) (int64, error) {
	path := "/attachments/" + attachmentID + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return 0, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.Copy(w, resp.Body)
}

// imageMIME maps a lowercase file extension to the MIME type the backend accepts
// for inline media uploads.
var imageMIME = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml", ".bmp": "image/bmp",
}

// postMultipart uploads data as the "file" part of a multipart POST, which is
// the shape both /uploads (inline media) and /tasks/:id/attachments expect.
func (c *Client) postMultipart(ctx context.Context, path, filename, mimeType string, data []byte, out any) error {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	if mimeType != "" {
		h.Set("Content-Type", mimeType)
	}
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.do(req, http.MethodPost, path, out)
}

// UploadMedia uploads image bytes as inline media and returns the URL to embed
// in Markdown (e.g. "/api/uploads/<uuid>.png"). mimeType may be "" to infer from
// the filename extension.
func (c *Client) UploadMedia(ctx context.Context, filename, mimeType string, data []byte) (string, error) {
	if mimeType == "" {
		mimeType = imageMIME[strings.ToLower(ext(filename))]
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.postMultipart(ctx, "/uploads", filename, mimeType, data, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// FetchImage downloads image bytes for a reference taken from a task: an
// "attachment:<id>" ref (auth-protected download) or a URL — either "/api/…"
// relative to the host, a "/…" path relative to the API base, or an absolute
// http(s) URL. Returns the bytes and their MIME type.
func (c *Client) FetchImage(ctx context.Context, ref string) ([]byte, string, error) {
	var target string
	switch {
	case strings.HasPrefix(ref, "attachment:"):
		id := strings.TrimPrefix(ref, "attachment:")
		target = c.baseURL + "/attachments/" + id + "/download"
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		target = ref
	case strings.HasPrefix(ref, "/api/"):
		target = c.WebBaseURL() + ref // WebBaseURL drops the trailing /api
	case strings.HasPrefix(ref, "/"):
		target = c.baseURL + ref
	default:
		return nil, "", fmt.Errorf("unrecognised image ref %q", ref)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token) // harmless for public /uploads, required for attachments
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("GET %s: HTTP %d: %s", target, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	const maxImageBytes = 12 << 20 // guardrail; backend caps media at 8 MiB, attachments at 25 MiB
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxImageBytes {
		return nil, "", fmt.Errorf("image %q exceeds %d bytes", ref, maxImageBytes)
	}
	mime := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	if mime == "" || mime == "application/octet-stream" {
		if m := imageMIME[strings.ToLower(ext(ref))]; m != "" {
			mime = m
		}
	}
	return data, mime, nil
}

// ext returns the lowercase filename extension (including the dot), or "".
func ext(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}
