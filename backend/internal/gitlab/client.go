// Package gitlab is a minimal GraphQL client for a self-hosted GitLab, plus the
// label rule engine that maps work items onto Tessera board state. Phase A is
// pull-only: it reads issues assigned to a user and never writes back.
package gitlab

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// newTransport builds the HTTP transport shared by the GraphQL and asset clients.
// The short dial + TLS-handshake timeouts are deliberate: when the self-hosted
// GitLab is unreachable (e.g. a dropped tunnel), a proxy/sync request must fail
// fast instead of holding a connection — otherwise the browser's per-origin
// connection pool fills with stalled asset/avatar fetches and starves real API
// calls, freezing the UI behind the "connecting…" overlay.
func newTransport() *http.Transport {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout: 4 * time.Second,
	}
	if strings.EqualFold(os.Getenv("GITLAB_INSECURE_TLS"), "true") {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-hosted private CAs
	}
	return tr
}

// Client talks to <baseURL>/api/graphql with a personal access token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client. baseURL is the GitLab root (trailing slash trimmed).
// Set GITLAB_INSECURE_TLS=true to skip certificate verification for a
// self-hosted instance with a private/self-signed CA (dev convenience).
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		// A large pull (hundreds of issues, each with up to 100 notes, plus all
		// linked issues) can take well over 15s; the sync now runs in the
		// background so it can afford a generous ceiling. A dead host still fails
		// fast via the transport dial/TLS timeouts (3s/4s).
		http: &http.Client{Timeout: 120 * time.Second, Transport: newTransport()},
	}
}

// NewHTTPClient returns a plain HTTP client configured like the GraphQL client
// (honours GITLAB_INSECURE_TLS), for streaming asset downloads. The overall
// timeout is short so a dead GitLab fails fast (see newTransport).
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second, Transport: newTransport()}
}

// graphqlError is one entry of a GraphQL `errors` array.
type graphqlError struct {
	Message string `json:"message"`
}

// do executes a GraphQL query and unmarshals `data` into out.
func (c *Client) do(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gitlab: unauthorized (check token)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab: http %d", resp.StatusCode)
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []graphqlError  `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("gitlab graphql: %s", envelope.Errors[0].Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

// User is the GitLab identity a token authenticates as.
type User struct {
	ID       int64
	Username string
}

// CurrentUser resolves the token's owner (used to verify a connection and to
// derive the "assigned to me" filter).
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	var data struct {
		CurrentUser *struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"currentUser"`
	}
	if err := c.do(ctx, `query { currentUser { id username } }`, nil, &data); err != nil {
		return User{}, err
	}
	if data.CurrentUser == nil {
		return User{}, fmt.Errorf("gitlab: token resolved no user")
	}
	return User{ID: parseGID(data.CurrentUser.ID), Username: data.CurrentUser.Username}, nil
}

// Person is a GitLab user reference (assignee / note author).
type Person struct {
	Login     string
	Name      string
	AvatarURL string // absolute (or instance-relative) avatar URL, "" when none
}

// Note is a GitLab issue comment (non-system note).
type Note struct {
	GlobalID  string
	Body      string
	Author    Person
	CreatedAt time.Time
}

// Issue is a GitLab issue reduced to the fields the sync needs.
type Issue struct {
	GlobalID       string
	IID            int64
	Title          string
	Description    string
	WebURL         string
	State          string // opened | closed
	UpdatedAt      *time.Time
	CreatedAt      *time.Time // issue/task creation timestamp (always present)
	DueDate        *time.Time // issue's own due date (date-only), nil when unset
	TimeEstimate   int64      // issue time estimate in seconds (0 when unset)
	MilestoneDue   *time.Time // due date (End date) of the issue's milestone, if any
	MilestoneStart *time.Time // start date of the issue's milestone (date-only), if any
	MilestoneGID   string     // milestone GraphQL global id (gid://gitlab/Milestone/<n>), "" when none
	MilestoneIID   int64      // milestone iid (0 when none/unavailable)
	MilestoneTitle string     // milestone title
	MilestoneState string     // active | closed
	MilestoneURL   string     // milestone web path/url
	Labels         []Label
	AuthorLogin    string // GitLab username of the issue author (may not be a Tessera user)
	AuthorName     string
	AuthorAvatar   string // author avatar URL, "" when none
	Assignees      []Person
	Notes          []Note // user comments (system notes filtered out)
}

const issuesQuery = `
query($path: ID!, $username: String, $iids: [String!], $after: String) {
  project(fullPath: $path) {
    issues(assigneeUsername: $username, iids: $iids, first: 100, after: $after, sort: UPDATED_DESC) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        iid
        title
        description
        webUrl
        state
        updatedAt
        createdAt
        dueDate
        timeEstimate
        milestone { id iid title state startDate dueDate webPath }
        author { username name avatarUrl }
        assignees { nodes { username name avatarUrl } }
        labels { nodes { title color } }
        notes(first: 100) { nodes { id body system createdAt author { username name avatarUrl } } }
      }
    }
  }
}`

// AssignedIssues returns the issues in projectPath assigned to username.
func (c *Client) AssignedIssues(ctx context.Context, projectPath, username string) ([]Issue, error) {
	return c.queryIssues(ctx, projectPath, map[string]any{"username": username})
}

// IssuesByIIDs returns the issues with the given iids regardless of assignee
// (used to keep already-linked tasks fresh after a reassignment). Empty iids
// returns nothing.
func (c *Client) IssuesByIIDs(ctx context.Context, projectPath string, iids []string) ([]Issue, error) {
	if len(iids) == 0 {
		return nil, nil
	}
	return c.queryIssues(ctx, projectPath, map[string]any{"iids": iids})
}

// queryIssues runs the issues query with the given filter vars, following
// pagination and decoding each node into an Issue.
func (c *Client) queryIssues(ctx context.Context, projectPath string, filter map[string]any) ([]Issue, error) {
	var out []Issue
	var after *string
	for {
		var data struct {
			Project *struct {
				Issues struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []issueNode `json:"nodes"`
				} `json:"issues"`
			} `json:"project"`
		}
		vars := map[string]any{"path": projectPath, "after": after}
		for k, v := range filter {
			vars[k] = v
		}
		if err := c.do(ctx, issuesQuery, vars, &data); err != nil {
			return nil, err
		}
		if data.Project == nil {
			return nil, fmt.Errorf("gitlab: project %q not found or not accessible", projectPath)
		}
		for _, n := range data.Project.Issues.Nodes {
			out = append(out, n.toIssue(c.baseURL))
		}
		if !data.Project.Issues.PageInfo.HasNextPage {
			break
		}
		cursor := data.Project.Issues.PageInfo.EndCursor
		after = &cursor
	}
	return out, nil
}

// issueNode mirrors the GraphQL issue node shape.
type issueNode struct {
	ID           string     `json:"id"`
	IID          string     `json:"iid"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	WebURL       string     `json:"webUrl"`
	State        string     `json:"state"`
	UpdatedAt    *time.Time `json:"updatedAt"`
	CreatedAt    *time.Time `json:"createdAt"`
	DueDate      string     `json:"dueDate"`
	TimeEstimate int64      `json:"timeEstimate"` // seconds (0 when unset)
	Milestone    *struct {
		ID        string `json:"id"`
		IID       string `json:"iid"`
		Title     string `json:"title"`
		State     string `json:"state"`
		StartDate string `json:"startDate"`
		DueDate   string `json:"dueDate"`
		WebPath   string `json:"webPath"`
	} `json:"milestone"`
	Author *struct {
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"author"`
	Assignees struct {
		Nodes []struct {
			Username  string `json:"username"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatarUrl"`
		} `json:"nodes"`
	} `json:"assignees"`
	Labels struct {
		Nodes []struct {
			Title string `json:"title"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
	Notes struct {
		Nodes []struct {
			ID        string    `json:"id"`
			Body      string    `json:"body"`
			System    bool      `json:"system"`
			CreatedAt time.Time `json:"createdAt"`
			Author    *struct {
				Username  string `json:"username"`
				Name      string `json:"name"`
				AvatarURL string `json:"avatarUrl"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"notes"`
}

// absAvatar resolves a possibly instance-relative avatar URL against base.
// Absolute URLs (gravatar, already-qualified) pass through unchanged.
func absAvatar(base, u string) string {
	if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return base + "/" + strings.TrimLeft(u, "/")
}

func (n issueNode) toIssue(base string) Issue {
	labels := make([]Label, 0, len(n.Labels.Nodes))
	for _, l := range n.Labels.Nodes {
		labels = append(labels, Label{Title: l.Title, Color: l.Color})
	}
	iid, _ := strconv.ParseInt(n.IID, 10, 64)
	issue := Issue{
		GlobalID: n.ID, IID: iid, Title: n.Title, Description: n.Description,
		WebURL: n.WebURL, State: n.State, UpdatedAt: n.UpdatedAt,
		CreatedAt: n.CreatedAt, DueDate: parseDate(n.DueDate), TimeEstimate: n.TimeEstimate, Labels: labels,
	}
	if n.Milestone != nil {
		issue.MilestoneStart = parseDate(n.Milestone.StartDate)
		issue.MilestoneDue = parseDate(n.Milestone.DueDate)
		issue.MilestoneGID = n.Milestone.ID
		issue.MilestoneIID, _ = strconv.ParseInt(n.Milestone.IID, 10, 64)
		issue.MilestoneTitle = n.Milestone.Title
		issue.MilestoneState = strings.ToLower(n.Milestone.State)
		issue.MilestoneURL = absAvatar(base, n.Milestone.WebPath)
	}
	if n.Author != nil {
		issue.AuthorLogin = n.Author.Username
		issue.AuthorName = n.Author.Name
		issue.AuthorAvatar = absAvatar(base, n.Author.AvatarURL)
	}
	for _, a := range n.Assignees.Nodes {
		issue.Assignees = append(issue.Assignees, Person{
			Login: a.Username, Name: a.Name, AvatarURL: absAvatar(base, a.AvatarURL),
		})
	}
	for _, note := range n.Notes.Nodes {
		if note.System || note.ID == "" {
			continue // skip system notes ("changed status…")
		}
		nt := Note{GlobalID: note.ID, Body: note.Body, CreatedAt: note.CreatedAt}
		if note.Author != nil {
			nt.Author = Person{
				Login: note.Author.Username, Name: note.Author.Name,
				AvatarURL: absAvatar(base, note.Author.AvatarURL),
			}
		}
		issue.Notes = append(issue.Notes, nt)
	}
	return issue
}

const childItemsQuery = `
query($path: ID!, $iid: String!) {
  project(fullPath: $path) {
    workItems(iid: $iid) {
      nodes {
        widgets {
          ... on WorkItemWidgetHierarchy {
            children { nodes { iid } }
          }
        }
      }
    }
  }
}`

// ChildIIDs returns the iids of a work item's child items (the Hierarchy widget),
// for syncing GitLab grouped issues as Tessera subtasks. Best-effort: returns
// nil on any schema/transport error so the caller can skip children gracefully.
func (c *Client) ChildIIDs(ctx context.Context, projectPath string, parentIID int64) ([]int64, error) {
	var data struct {
		Project *struct {
			WorkItems struct {
				Nodes []struct {
					Widgets []struct {
						Children *struct {
							Nodes []struct {
								IID string `json:"iid"`
							} `json:"nodes"`
						} `json:"children"`
					} `json:"widgets"`
				} `json:"nodes"`
			} `json:"workItems"`
		} `json:"project"`
	}
	vars := map[string]any{"path": projectPath, "iid": strconv.FormatInt(parentIID, 10)}
	if err := c.do(ctx, childItemsQuery, vars, &data); err != nil {
		return nil, err
	}
	if data.Project == nil || len(data.Project.WorkItems.Nodes) == 0 {
		return nil, nil
	}
	var out []int64
	for _, w := range data.Project.WorkItems.Nodes[0].Widgets {
		if w.Children == nil {
			continue
		}
		for _, ch := range w.Children.Nodes {
			if n, err := strconv.ParseInt(ch.IID, 10, 64); err == nil {
				out = append(out, n)
			}
		}
	}
	return out, nil
}

// parseDate parses a GitLab Date scalar ("YYYY-MM-DD") into a UTC time, or nil
// when empty/invalid.
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// parseGID extracts the trailing numeric id from a GraphQL global id such as
// "gid://gitlab/User/42" → 42. Returns 0 when absent.
func parseGID(gid string) int64 {
	i := strings.LastIndex(gid, "/")
	if i < 0 || i+1 >= len(gid) {
		return 0
	}
	n, _ := strconv.ParseInt(gid[i+1:], 10, 64)
	return n
}

// ── write-back (REST) ──
// Mutations go through the REST v4 API (not GraphQL): it takes label titles and
// the state_event as plain strings, so there's no label-ID resolution step. The
// caller (write-back worker) decides retry vs. give-up from *APIError.Status.

// APIError is a non-2xx response from the GitLab REST API. Status carries the HTTP
// status so the caller can classify permanent (4xx) vs. transient (5xx) failures.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gitlab: http %d: %s", e.Status, strings.TrimSpace(e.Body))
}

// issuePath is the REST path for one issue, with the project path URL-encoded
// ("group/project" → "group%2Fproject").
func issuePath(projectPath string, iid int64) string {
	return "projects/" + projectPathEsc(projectPath) + "/issues/" + strconv.FormatInt(iid, 10)
}

// restForm sends a form-encoded request to <baseURL>/api/v4/<path> with the PAT,
// returning the body on 2xx or an *APIError otherwise.
func (c *Client) restForm(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v4/"+strings.TrimPrefix(path, "/"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

// restGet performs a GET against the REST v4 API and returns the body (up to a
// generous cap, member pages can be large) or an *APIError. body limit larger
// than restForm's since list endpoints return arrays.
func (c *Client) restGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v4/"+strings.TrimPrefix(path, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

// Member is a GitLab project member (for the assignee picker + assignee write-back).
type Member struct {
	ID          int64
	Username    string
	Name        string
	AvatarURL   string
	AccessLevel int
}

// ProjectMembers lists the project's members (including inherited), paginated.
// Caps at 10 pages (1000 members) — plenty for a self-hosted project.
func (c *Client) ProjectMembers(ctx context.Context, projectPath string) ([]Member, error) {
	base := "projects/" + url.PathEscape(strings.Trim(projectPath, "/")) + "/members/all?per_page=100&page="
	var out []Member
	for page := 1; page <= 10; page++ {
		body, err := c.restGet(ctx, base+strconv.Itoa(page))
		if err != nil {
			return nil, err
		}
		var rows []struct {
			ID          int64  `json:"id"`
			Username    string `json:"username"`
			Name        string `json:"name"`
			AvatarURL   string `json:"avatar_url"`
			AccessLevel int    `json:"access_level"`
		}
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, Member{ID: r.ID, Username: r.Username, Name: r.Name, AvatarURL: r.AvatarURL, AccessLevel: r.AccessLevel})
		}
		if len(rows) < 100 {
			break
		}
	}
	return out, nil
}

// SetIssueAssignees replaces the issue's assignees with the given numeric user
// ids (GitLab's assignee_ids is replace-all). An empty slice clears assignees.
func (c *Client) SetIssueAssignees(ctx context.Context, projectPath string, iid int64, ids []int64) error {
	form := url.Values{}
	if len(ids) == 0 {
		form.Set("assignee_ids", "0") // GitLab convention: a single 0 unassigns all
	} else {
		strs := make([]string, len(ids))
		for i, id := range ids {
			strs[i] = strconv.FormatInt(id, 10)
		}
		form["assignee_ids[]"] = strs
	}
	_, err := c.restForm(ctx, http.MethodPut, issuePath(projectPath, iid), form)
	return err
}

// UpdateIssueState closes or reopens an issue. event is "close" or "reopen".
func (c *Client) UpdateIssueState(ctx context.Context, projectPath string, iid int64, event string) error {
	_, err := c.restForm(ctx, http.MethodPut, issuePath(projectPath, iid), url.Values{"state_event": {event}})
	return err
}

// UpdateIssueDueDate sets the issue's own due date (date is "YYYY-MM-DD"); an
// empty date clears it (GitLab accepts an empty due_date to unset).
func (c *Client) UpdateIssueDueDate(ctx context.Context, projectPath string, iid int64, date string) error {
	_, err := c.restForm(ctx, http.MethodPut, issuePath(projectPath, iid), url.Values{"due_date": {date}})
	return err
}

// SetIssueMilestone assigns the issue to the milestone with the given numeric id, or
// clears it when milestoneID <= 0 (GitLab unassigns on milestone_id=0).
func (c *Client) SetIssueMilestone(ctx context.Context, projectPath string, iid, milestoneID int64) error {
	v := "0"
	if milestoneID > 0 {
		v = strconv.FormatInt(milestoneID, 10)
	}
	_, err := c.restForm(ctx, http.MethodPut, issuePath(projectPath, iid), url.Values{"milestone_id": {v}})
	return err
}

// UpdateIssueTitleDescription overwrites the issue's title and description. Used by
// the title_desc write-back, which is three-way conflict-checked before this call.
func (c *Client) UpdateIssueTitleDescription(ctx context.Context, projectPath string, iid int64, title, description string) error {
	_, err := c.restForm(ctx, http.MethodPut, issuePath(projectPath, iid),
		url.Values{"title": {title}, "description": {description}})
	return err
}

// SetIssueTimeEstimate sets the issue's time estimate to `minutes` (GitLab parses
// the "<n>m" duration string), or resets it when minutes <= 0.
func (c *Client) SetIssueTimeEstimate(ctx context.Context, projectPath string, iid int64, minutes int64) error {
	if minutes <= 0 {
		_, err := c.restForm(ctx, http.MethodPost, issuePath(projectPath, iid)+"/reset_time_estimate", url.Values{})
		return err
	}
	_, err := c.restForm(ctx, http.MethodPost, issuePath(projectPath, iid)+"/time_estimate",
		url.Values{"duration": {strconv.FormatInt(minutes, 10) + "m"}})
	return err
}

// SetIssueLabels adds and/or removes labels (by title) on an issue. Empty slices
// are skipped; a fully-empty call is a no-op.
func (c *Client) SetIssueLabels(ctx context.Context, projectPath string, iid int64, add, remove []string) error {
	form := url.Values{}
	if len(add) > 0 {
		form.Set("add_labels", strings.Join(add, ","))
	}
	if len(remove) > 0 {
		form.Set("remove_labels", strings.Join(remove, ","))
	}
	if len(form) == 0 {
		return nil
	}
	_, err := c.restForm(ctx, http.MethodPut, issuePath(projectPath, iid), form)
	return err
}

// projectPathEsc URL-encodes a project path for use in a REST path
// ("group/project" → "group%2Fproject").
func projectPathEsc(projectPath string) string {
	return url.PathEscape(strings.Trim(projectPath, "/"))
}

// CreatedIssue is the subset of a freshly-created issue the caller needs to build
// the gitlab_link row (the numeric db id reconstructs the GraphQL global id).
type CreatedIssue struct {
	ID     int64  // global numeric db id, e.g. 123 (NOT the per-project iid)
	IID    int64  // per-project iid (#N)
	WebURL string // https://gitlab.example.com/group/project/-/issues/N
	State  string // "opened"
}

// GlobalID reconstructs the GraphQL global id ("gid://gitlab/Issue/<id>") so the
// created link matches the gl_global_id shape used by the pull path.
func (ci CreatedIssue) GlobalID() string {
	return fmt.Sprintf("gid://gitlab/Issue/%d", ci.ID)
}

// CreatedMilestone is the subset of a freshly-created project milestone the caller
// needs to build the gitlab_milestone_links row.
type CreatedMilestone struct {
	ID     int64 // global numeric milestone id (for REST milestone_id)
	IID    int64 // per-project iid (may be 0)
	WebURL string
	State  string // "active"
}

// GlobalID reconstructs the GraphQL global id ("gid://gitlab/Milestone/<id>").
func (cm CreatedMilestone) GlobalID() string {
	return fmt.Sprintf("gid://gitlab/Milestone/%d", cm.ID)
}

// CreateProjectMilestone creates a project milestone from Tessera-side fields.
// startDate/dueDate are "YYYY-MM-DD" or empty.
func (c *Client) CreateProjectMilestone(ctx context.Context, projectPath, title, description, startDate, dueDate string) (CreatedMilestone, error) {
	form := url.Values{}
	form.Set("title", title)
	if description != "" {
		form.Set("description", description)
	}
	if startDate != "" {
		form.Set("start_date", startDate)
	}
	if dueDate != "" {
		form.Set("due_date", dueDate)
	}
	out, err := c.restForm(ctx, http.MethodPost, "projects/"+projectPathEsc(projectPath)+"/milestones", form)
	if err != nil {
		return CreatedMilestone{}, err
	}
	var resp struct {
		ID     int64  `json:"id"`
		IID    int64  `json:"iid"`
		WebURL string `json:"web_url"`
		State  string `json:"state"`
	}
	if uerr := json.Unmarshal(out, &resp); uerr != nil {
		return CreatedMilestone{}, uerr
	}
	return CreatedMilestone{ID: resp.ID, IID: resp.IID, WebURL: resp.WebURL, State: resp.State}, nil
}

// CreateIssue opens a new issue in the project from Tessera-side fields. labels are
// full label titles (joined into the comma-separated labels param); dueDate is
// "YYYY-MM-DD" or empty; assigneeIDs are numeric GitLab user ids (empty = none).
func (c *Client) CreateIssue(ctx context.Context, projectPath, title, description string, labels []string, dueDate string, assigneeIDs []int64) (CreatedIssue, error) {
	form := url.Values{}
	form.Set("title", title)
	if description != "" {
		form.Set("description", description)
	}
	if len(labels) > 0 {
		form.Set("labels", strings.Join(labels, ","))
	}
	if dueDate != "" {
		form.Set("due_date", dueDate)
	}
	if len(assigneeIDs) > 0 {
		strs := make([]string, len(assigneeIDs))
		for i, id := range assigneeIDs {
			strs[i] = strconv.FormatInt(id, 10)
		}
		form["assignee_ids[]"] = strs
	}
	out, err := c.restForm(ctx, http.MethodPost, "projects/"+projectPathEsc(projectPath)+"/issues", form)
	if err != nil {
		return CreatedIssue{}, err
	}
	var resp struct {
		ID     int64  `json:"id"`
		IID    int64  `json:"iid"`
		WebURL string `json:"web_url"`
		State  string `json:"state"`
	}
	if uerr := json.Unmarshal(out, &resp); uerr != nil {
		return CreatedIssue{}, uerr
	}
	return CreatedIssue{ID: resp.ID, IID: resp.IID, WebURL: resp.WebURL, State: resp.State}, nil
}

// IssueTemplate is a repo issue template (.gitlab/issue_templates/<Name>.md).
type IssueTemplate struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// IssueTemplates lists the project's issue templates with their content. GitLab's
// list endpoint returns names only, so each template's body is fetched separately;
// capped at maxTemplates to bound the fan-out. A project with no templates yields
// an empty slice (not an error).
func (c *Client) IssueTemplates(ctx context.Context, projectPath string) ([]IssueTemplate, error) {
	const maxTemplates = 25
	body, err := c.restGet(ctx, "projects/"+projectPathEsc(projectPath)+"/templates/issues")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	if uerr := json.Unmarshal(body, &rows); uerr != nil {
		return nil, uerr
	}
	out := make([]IssueTemplate, 0, len(rows))
	for i, r := range rows {
		if i >= maxTemplates {
			break
		}
		name := r.Name
		if name == "" {
			name = r.Key
		}
		tb, terr := c.restGet(ctx, "projects/"+projectPathEsc(projectPath)+"/templates/issues/"+url.PathEscape(name))
		if terr != nil {
			continue // skip a template that can't be fetched; keep the rest
		}
		var tpl IssueTemplate
		if json.Unmarshal(tb, &tpl) == nil {
			if tpl.Name == "" {
				tpl.Name = name
			}
			out = append(out, tpl)
		}
	}
	return out, nil
}

// CreateIssueNote posts a comment (note) on an issue and returns the created
// note's GraphQL global id ("gid://gitlab/Note/<id>"), so the caller can tag the
// originating Tessera comment and the next pull dedups instead of duplicating.
// Returns "" (with nil error) if the response id couldn't be read.
func (c *Client) CreateIssueNote(ctx context.Context, projectPath string, iid int64, body string) (string, error) {
	out, err := c.restForm(ctx, http.MethodPost, issuePath(projectPath, iid)+"/notes", url.Values{"body": {body}})
	if err != nil {
		return "", err
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	if uerr := json.Unmarshal(out, &resp); uerr != nil || resp.ID == 0 {
		return "", nil
	}
	return fmt.Sprintf("gid://gitlab/Note/%d", resp.ID), nil
}
