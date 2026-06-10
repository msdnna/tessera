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
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

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
	tr := &http.Transport{}
	if strings.EqualFold(os.Getenv("GITLAB_INSECURE_TLS"), "true") {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-hosted private CAs
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second, Transport: tr},
	}
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

// Issue is a GitLab issue reduced to the fields the sync needs.
type Issue struct {
	GlobalID    string
	IID         int64
	Title       string
	Description string
	WebURL      string
	State       string // opened | closed
	UpdatedAt   *time.Time
	Labels      []string
	AuthorLogin string // GitLab username of the issue author (may not be a Tessera user)
	AuthorName  string
}

const assignedIssuesQuery = `
query($path: ID!, $username: String!, $after: String) {
  project(fullPath: $path) {
    issues(assigneeUsername: $username, first: 100, after: $after, sort: UPDATED_DESC) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        iid
        title
        description
        webUrl
        state
        updatedAt
        author { username name }
        labels { nodes { title } }
      }
    }
  }
}`

// AssignedIssues returns every issue in projectPath assigned to username,
// following pagination. Uses the issues resolver (stable in GitLab 16.x);
// work-item children/tasks are a later addition.
func (c *Client) AssignedIssues(ctx context.Context, projectPath, username string) ([]Issue, error) {
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
					Nodes []struct {
						ID          string     `json:"id"`
						IID         string     `json:"iid"`
						Title       string     `json:"title"`
						Description string     `json:"description"`
						WebURL      string     `json:"webUrl"`
						State       string     `json:"state"`
						UpdatedAt   *time.Time `json:"updatedAt"`
						Author      *struct {
							Username string `json:"username"`
							Name     string `json:"name"`
						} `json:"author"`
						Labels struct {
							Nodes []struct {
								Title string `json:"title"`
							} `json:"nodes"`
						} `json:"labels"`
					} `json:"nodes"`
				} `json:"issues"`
			} `json:"project"`
		}
		vars := map[string]any{"path": projectPath, "username": username, "after": after}
		if err := c.do(ctx, assignedIssuesQuery, vars, &data); err != nil {
			return nil, err
		}
		if data.Project == nil {
			return nil, fmt.Errorf("gitlab: project %q not found or not accessible", projectPath)
		}
		for _, n := range data.Project.Issues.Nodes {
			labels := make([]string, 0, len(n.Labels.Nodes))
			for _, l := range n.Labels.Nodes {
				labels = append(labels, l.Title)
			}
			iid, _ := strconv.ParseInt(n.IID, 10, 64)
			issue := Issue{
				GlobalID: n.ID, IID: iid, Title: n.Title, Description: n.Description,
				WebURL: n.WebURL, State: n.State, UpdatedAt: n.UpdatedAt, Labels: labels,
			}
			if n.Author != nil {
				issue.AuthorLogin = n.Author.Username
				issue.AuthorName = n.Author.Name
			}
			out = append(out, issue)
		}
		if !data.Project.Issues.PageInfo.HasNextPage {
			break
		}
		cursor := data.Project.Issues.PageInfo.EndCursor
		after = &cursor
	}
	return out, nil
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
