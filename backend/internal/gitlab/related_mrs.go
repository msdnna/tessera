package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ── Merge requests of other projects (#2964 rework) ────────
//
// A binding mirrors one project's issues, but the code that works on them often
// lives elsewhere: a scrum project's issue is closed by MRs in the service
// repositories. The project poll (mergerequests.go) can't see those — it lists
// the bound project's MRs only, and the other projects' webhooks never reach us.
// So for those we ask the issue side: GitLab itself knows which MRs mention an
// issue (related_merge_requests, from the cross-reference notes) and which will
// close it (closed_by), across every project the token can read.

// relatedPerPage is the page asked for: an issue with more related MRs than this
// is not one anybody reads a «Разработка» list of.
const relatedPerPage = 100

// restMR is a merge request in the REST v4 shape (related_merge_requests,
// closed_by and the single-MR read all answer with it).
type restMR struct {
	ID             int64      `json:"id"`
	IID            int64      `json:"iid"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	State          string     `json:"state"`
	Draft          *bool      `json:"draft"`
	WorkInProgress bool       `json:"work_in_progress"`
	SourceBranch   string     `json:"source_branch"`
	TargetBranch   string     `json:"target_branch"`
	WebURL         string     `json:"web_url"`
	UpdatedAt      *time.Time `json:"updated_at"`
	References     struct {
		Full string `json:"full"`
	} `json:"references"`
	HeadPipeline *restPipeline `json:"head_pipeline"`
	// Pipeline is the older (deprecated) name of the same thing; read when
	// head_pipeline is absent.
	Pipeline *restPipeline `json:"pipeline"`
}

type restPipeline struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	WebURL string `json:"web_url"`
}

func (m restMR) toMergeRequest() MergeRequest {
	draft := m.WorkInProgress
	if m.Draft != nil {
		draft = *m.Draft
	}
	mr := MergeRequest{
		GlobalID: mrGID(m.ID), IID: m.IID, Title: m.Title, Description: m.Description,
		WebURL: m.WebURL, State: strings.ToLower(m.State), Draft: draft,
		SourceBranch: m.SourceBranch, TargetBranch: m.TargetBranch, UpdatedAt: m.UpdatedAt,
		ProjectPath: mrProjectPath(m.References.Full, m.WebURL),
	}
	if mr.UpdatedAt != nil {
		t := mr.UpdatedAt.UTC()
		mr.UpdatedAt = &t
	}
	p := m.HeadPipeline
	if p == nil {
		p = m.Pipeline
	}
	if p != nil && p.ID > 0 {
		mr.Pipeline = &Pipeline{ID: p.ID, Status: strings.ToLower(p.Status), WebURL: p.WebURL}
	}
	return mr
}

// mrProjectPath is the full path of the project an MR lives in: from its full
// reference ("group/sub/proj!12"), else from its web URL
// (".../group/sub/proj/-/merge_requests/12"). "" when neither says.
func mrProjectPath(fullRef, webURL string) string {
	if i := strings.LastIndex(fullRef, "!"); i > 0 {
		return fullRef[:i]
	}
	if m := mrURLRe.FindStringSubmatch(webURL); m != nil {
		return strings.Trim(m[1], "/")
	}
	return ""
}

// MRKey identifies a merge request across projects: "group/proj!12". Paths are
// case-insensitive in GitLab, so the key is too.
func MRKey(projectPath string, iid int64) string {
	return strings.ToLower(strings.Trim(projectPath, "/")) + "!" + strconv.FormatInt(iid, 10)
}

// IssueMergeRequests lists the merge requests GitLab relates to an issue, from
// any project the token can read, and which of them close it on merge (keyed by
// MRKey). GitLab leaves out MRs the token may not see; the error is the issue's
// own read failing (both reads must succeed — the caller prunes by the answer).
func (c *Client) IssueMergeRequests(ctx context.Context, projectPath string, issueIID int64) ([]MergeRequest, map[string]bool, error) {
	base := issuePath(projectPath, issueIID)
	related, err := c.restMRList(ctx, base+"/related_merge_requests?per_page="+strconv.Itoa(relatedPerPage))
	if err != nil {
		return nil, nil, err
	}
	closing, err := c.restMRList(ctx, base+"/closed_by?per_page="+strconv.Itoa(relatedPerPage))
	if err != nil {
		return nil, nil, err
	}
	closes := make(map[string]bool, len(closing))
	seen := make(map[string]bool, len(related)+len(closing))
	out := make([]MergeRequest, 0, len(related)+len(closing))
	for _, mr := range closing {
		k := MRKey(mr.ProjectPath, mr.IID)
		closes[k] = true
		if !seen[k] {
			seen[k] = true
			out = append(out, mr)
		}
	}
	for _, mr := range related {
		if k := MRKey(mr.ProjectPath, mr.IID); !seen[k] {
			seen[k] = true
			out = append(out, mr)
		}
	}
	return out, closes, nil
}

func (c *Client) restMRList(ctx context.Context, path string) ([]MergeRequest, error) {
	body, err := c.restGet(ctx, path)
	if err != nil {
		return nil, err
	}
	var items []restMR
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	out := make([]MergeRequest, 0, len(items))
	for _, it := range items {
		mr := it.toMergeRequest()
		if mr.IID <= 0 || mr.ProjectPath == "" {
			continue
		}
		out = append(out, mr)
	}
	return out, nil
}

// MergeRequestByPath reads one merge request of any project, with its head
// pipeline — the list endpoints don't always carry it.
func (c *Client) MergeRequestByPath(ctx context.Context, projectPath string, iid int64) (MergeRequest, error) {
	body, err := c.restGet(ctx, "projects/"+projectPathEsc(projectPath)+"/merge_requests/"+strconv.FormatInt(iid, 10))
	if err != nil {
		return MergeRequest{}, err
	}
	var it restMR
	if err := json.Unmarshal(body, &it); err != nil {
		return MergeRequest{}, err
	}
	mr := it.toMergeRequest()
	if mr.ProjectPath == "" {
		mr.ProjectPath = strings.Trim(projectPath, "/")
	}
	return mr, nil
}

// IsNoAccess reports a GitLab refusal that means «this project is not yours to
// read» (or gone): 401, 403, 404. Callers skip such a project quietly.
func IsNoAccess(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden || ae.Status == http.StatusNotFound
}
