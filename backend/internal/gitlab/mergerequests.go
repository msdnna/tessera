package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Merge requests and pipelines (#2964) ───────────────────
//
// The task window's «Разработка» shows the merge requests that work on a task's
// issue, with their review state and the status of their latest pipeline. This
// file is the GitLab half of that: reading MRs (GraphQL poll, webhook payloads),
// deciding which issues an MR is about, and translating GitLab's state words into
// the provider-neutral vocabulary of task_code_links. Nothing here touches the
// database — the handlers own storage.

// Neutral change states (task_code_links.state).
const (
	ChangeOpen   = "open"
	ChangeMerged = "merged"
	ChangeClosed = "closed"
)

// Neutral CI states (task_code_links.ci_status). "" means "no pipeline known".
const (
	CISuccess  = "success"
	CIRunning  = "running"
	CIFailed   = "failed"
	CICanceled = "canceled"
	CIManual   = "manual"
)

// How an MR names an issue (task_code_links.via), strongest first.
const (
	ViaCloses  = "closes"
	ViaMention = "mention"
	ViaBranch  = "branch"
)

// ViaRank orders the link reasons: an MR that both closes an issue and lives on
// its branch is a "closes" link. Unknown reasons rank below everything.
func ViaRank(via string) int {
	switch via {
	case ViaCloses:
		return 3
	case ViaMention:
		return 2
	case ViaBranch:
		return 1
	}
	return 0
}

// MergeRequest is a GitLab MR reduced to what the task window shows.
type MergeRequest struct {
	GlobalID     string // gid://gitlab/MergeRequest/<id>
	IID          int64
	Title        string
	Description  string
	WebURL       string
	State        string // raw GitLab state: opened | closed | merged | locked
	Draft        bool
	SourceBranch string
	TargetBranch string
	UpdatedAt    *time.Time
	// Pipeline is the MR's head pipeline, nil when it has none or the source
	// (a merge_request hook) does not report one.
	Pipeline *Pipeline
}

// Pipeline is one CI run as GitLab reports it.
type Pipeline struct {
	ID     int64
	Status string // raw GitLab status, lower case: success, running, failed…
	WebURL string
}

// ChangeState maps a GitLab MR state onto the neutral one. A locked MR is one
// GitLab is in the middle of merging — still open from the reader's side.
func ChangeState(glState string) string {
	switch strings.ToLower(glState) {
	case "merged":
		return ChangeMerged
	case "closed":
		return ChangeClosed
	default:
		return ChangeOpen
	}
}

// CIState maps a GitLab pipeline status onto the neutral one. Everything that is
// still on its way to a verdict (queued, preparing, waiting for a runner or a
// resource, scheduled) reads as running; a skipped pipeline never ran, which to
// the reader is the same as a cancelled one. Unknown statuses map to "" rather
// than to a guess.
func CIState(glStatus string) string {
	switch strings.ToLower(strings.TrimSpace(glStatus)) {
	case "success":
		return CISuccess
	case "failed":
		return CIFailed
	case "canceled", "cancelled", "canceling", "skipped":
		return CICanceled
	case "manual":
		return CIManual
	case "running", "pending", "created", "preparing", "scheduled",
		"waiting_for_resource", "waiting_for_callback":
		return CIRunning
	}
	return ""
}

// ── Which issues an MR is about ────────────────────────────

var (
	// closingRe is GitLab's default closing pattern: a keyword, an optional colon,
	// then one or more references ("Closes #12, #13 and #14").
	closingRe = regexp.MustCompile(`(?i)\b(?:clos(?:e|es|ed|ing)|fix(?:|es|ed|ing)|resolv(?:e|es|ed|ing)|implement(?:|s|ed|ing))\b:?\s+((?:(?:[\w.\-/]+)?#\d+|https?://\S+/-/issues/\d+)(?:(?:\s*,\s*|\s+and\s+|\s+)(?:(?:[\w.\-/]+)?#\d+|https?://\S+/-/issues/\d+))*)`)
	// refRe finds an issue reference: a bare "#12" or a qualified "group/proj#12".
	// The character in front must not continue a word or a path, so a URL fragment
	// ("page#12"), an MR ("!12") or an epic ("&12") are not issue references.
	refRe = regexp.MustCompile(`(?:^|[^\w#&!/.\-])((?:[\w.\-]+/)*[\w.\-]+)?#(\d+)\b`)
	// urlRe finds a full issue URL; the path in front of /-/issues/ is the project.
	urlRe = regexp.MustCompile(`https?://[^\s/]+/([^\s]+?)/-/(?:issues|work_items)/(\d+)\b`)
	// branchRe is the branch GitLab cuts from an issue: "<iid>-<slug>". The slug
	// has to carry a letter, so "release/2-0" is not issue 2.
	branchRe = regexp.MustCompile(`^(\d+)-(.*[A-Za-z].*)$`)
)

// TextIssueRefs lists the iids of this project's issues that text refers to, and
// which of them it refers to with a closing keyword. References to other
// projects are skipped: a task here mirrors an issue of projectPath only.
func TextIssueRefs(projectPath, text string) (mentioned, closing []int64) {
	if text == "" {
		return nil, nil
	}
	// Code spans and blocks quote other things (logs, examples); they don't refer.
	text = stripCode(text)
	seen := map[int64]bool{}
	add := func(out *[]int64, set map[int64]bool, iid int64) {
		if iid > 0 && !set[iid] {
			set[iid] = true
			*out = append(*out, iid)
		}
	}
	for _, m := range refRe.FindAllStringSubmatch(text, -1) {
		if m[1] != "" && !strings.EqualFold(m[1], projectPath) {
			continue
		}
		iid, _ := strconv.ParseInt(m[2], 10, 64)
		add(&mentioned, seen, iid)
	}
	for _, m := range urlRe.FindAllStringSubmatch(text, -1) {
		if !strings.EqualFold(strings.Trim(m[1], "/"), projectPath) {
			continue
		}
		iid, _ := strconv.ParseInt(m[2], 10, 64)
		add(&mentioned, seen, iid)
	}
	closed := map[int64]bool{}
	for _, m := range closingRe.FindAllStringSubmatch(text, -1) {
		// The closing clause is itself a list of references — run the reference
		// finders over it (with a leading space so a ref at its start matches).
		clause := " " + m[1]
		for _, r := range refRe.FindAllStringSubmatch(clause, -1) {
			if r[1] != "" && !strings.EqualFold(r[1], projectPath) {
				continue
			}
			iid, _ := strconv.ParseInt(r[2], 10, 64)
			add(&closing, closed, iid)
		}
		for _, r := range urlRe.FindAllStringSubmatch(clause, -1) {
			if !strings.EqualFold(strings.Trim(r[1], "/"), projectPath) {
				continue
			}
			iid, _ := strconv.ParseInt(r[2], 10, 64)
			add(&closing, closed, iid)
		}
	}
	return mentioned, closing
}

// stripCode blanks out fenced code blocks and inline code spans.
func stripCode(s string) string {
	if !strings.Contains(s, "`") {
		return s
	}
	var b strings.Builder
	inFence := false
	for _, line := range strings.SplitAfter(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			b.WriteString("\n")
			continue
		}
		if inFence {
			b.WriteString("\n")
			continue
		}
		b.WriteString(inlineCodeRe.ReplaceAllString(line, " "))
	}
	return b.String()
}

var inlineCodeRe = regexp.MustCompile("`[^`\n]*`")

// BranchIssue returns the iid a branch name was cut for ("12-fix-login",
// "feat/12-fix-login"), or 0. Only the last path segment counts: the prefix is
// the team's convention, the "<iid>-" is GitLab's.
func BranchIssue(branch string) int64 {
	seg := branch
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	m := branchRe.FindStringSubmatch(seg)
	if m == nil {
		return 0
	}
	iid, _ := strconv.ParseInt(m[1], 10, 64)
	return iid
}

// IssueTargets decides which of the project's issues an MR works on and why:
// GitLab's own "closes issues" list (closes, may be nil when unknown), closing
// keywords and plain references in the title and description, and the branch
// name. Each iid keeps its strongest reason.
func IssueTargets(projectPath string, mr MergeRequest, closes []int64) map[int64]string {
	out := map[int64]string{}
	put := func(iid int64, via string) {
		if iid <= 0 {
			return
		}
		if ViaRank(via) > ViaRank(out[iid]) {
			out[iid] = via
		}
	}
	for _, iid := range closes {
		put(iid, ViaCloses)
	}
	mentioned, closing := TextIssueRefs(projectPath, mr.Title+"\n"+mr.Description)
	for _, iid := range closing {
		put(iid, ViaCloses)
	}
	for _, iid := range mentioned {
		put(iid, ViaMention)
	}
	put(BranchIssue(mr.SourceBranch), ViaBranch)
	return out
}

// ── GraphQL poll ───────────────────────────────────────────

const mergeRequestsQuery = `
query($path: ID!, $iids: [String!], $after: String, $updatedAfter: Time, $first: Int!) {
  project(fullPath: $path) {
    mergeRequests(iids: $iids, updatedAfter: $updatedAfter, first: $first, after: $after, sort: UPDATED_DESC) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        iid
        title
        description
        webUrl
        state
        draft
        sourceBranch
        targetBranch
        updatedAt
        headPipeline { id status path }
      }
    }
  }
}`

// mrPageSize is the first page of the MR query. It is lean (no discussions), so
// GitLab's own deadline is rarely a concern — but a page that does time out is
// halved, the same way the issue query recovers (#3105).
const (
	mrPageSize = 50
	// MaxMergeRequestsPerPoll bounds one poll. The query runs newest first, so a
	// backlog past the cap loses its oldest MRs, never the ones being worked on.
	MaxMergeRequestsPerPoll = 1000
)

var mrPageSizes sync.Map // baseURL + "\x00" + projectPath → int

type mrNode struct {
	ID           string     `json:"id"`
	IID          string     `json:"iid"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	WebURL       string     `json:"webUrl"`
	State        string     `json:"state"`
	Draft        bool       `json:"draft"`
	SourceBranch string     `json:"sourceBranch"`
	TargetBranch string     `json:"targetBranch"`
	UpdatedAt    *time.Time `json:"updatedAt"`
	HeadPipeline *struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Path   string `json:"path"`
	} `json:"headPipeline"`
}

func (n mrNode) toMergeRequest(base string) MergeRequest {
	iid, _ := strconv.ParseInt(n.IID, 10, 64)
	mr := MergeRequest{
		GlobalID: n.ID, IID: iid, Title: n.Title, Description: n.Description,
		WebURL: n.WebURL, State: strings.ToLower(n.State), Draft: n.Draft,
		SourceBranch: n.SourceBranch, TargetBranch: n.TargetBranch, UpdatedAt: n.UpdatedAt,
	}
	if p := n.HeadPipeline; p != nil {
		mr.Pipeline = &Pipeline{ID: parseGID(p.ID), Status: strings.ToLower(p.Status), WebURL: absAvatar(base, p.Path)}
	}
	return mr
}

// MergeRequestsSince returns the project's MRs updated after since (nil: all),
// newest first, at most MaxMergeRequestsPerPoll of them.
func (c *Client) MergeRequestsSince(ctx context.Context, projectPath string, since *time.Time) ([]MergeRequest, error) {
	return c.queryMergeRequests(ctx, projectPath, withUpdatedAfter(map[string]any{}, since))
}

// MergeRequestsByIIDs returns the given MRs — the poll re-reads the ones whose
// pipeline is still running, since a pipeline finishing does not touch the MR's
// own updatedAt and would otherwise never surface in the delta.
func (c *Client) MergeRequestsByIIDs(ctx context.Context, projectPath string, iids []int64) ([]MergeRequest, error) {
	if len(iids) == 0 {
		return nil, nil
	}
	s := make([]string, 0, len(iids))
	for _, id := range iids {
		s = append(s, strconv.FormatInt(id, 10))
	}
	return c.queryMergeRequests(ctx, projectPath, map[string]any{"iids": s})
}

func (c *Client) queryMergeRequests(ctx context.Context, projectPath string, filter map[string]any) ([]MergeRequest, error) {
	var out []MergeRequest
	var after *string
	sizeKey := c.baseURL + "\x00" + projectPath
	first := mrPageSize
	if v, ok := mrPageSizes.Load(sizeKey); ok {
		first = v.(int)
	}
	for len(out) < MaxMergeRequestsPerPoll {
		var data struct {
			Project *struct {
				MergeRequests *struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []mrNode `json:"nodes"`
				} `json:"mergeRequests"`
			} `json:"project"`
		}
		vars := map[string]any{"path": projectPath, "after": after, "first": first}
		for k, v := range filter {
			vars[k] = v
		}
		if err := c.do(ctx, mergeRequestsQuery, vars, &data); err != nil {
			if isGraphQLTimeout(err) && first > 1 {
				first = max(first/2, 1)
				mrPageSizes.Store(sizeKey, first)
				continue
			}
			return nil, err
		}
		if data.Project == nil {
			return nil, fmt.Errorf("gitlab: project %q not found or not accessible", projectPath)
		}
		if data.Project.MergeRequests == nil {
			break // an instance (or a stand-in) without the connection: nothing to read
		}
		for _, n := range data.Project.MergeRequests.Nodes {
			out = append(out, n.toMergeRequest(c.baseURL))
		}
		if !data.Project.MergeRequests.PageInfo.HasNextPage {
			break
		}
		cursor := data.Project.MergeRequests.PageInfo.EndCursor
		after = &cursor
	}
	if len(out) > MaxMergeRequestsPerPoll {
		out = out[:MaxMergeRequestsPerPoll]
	}
	return out, nil
}

// ClosesIssues asks GitLab which of this project's issues the MR will close when
// merged (GET …/merge_requests/:iid/closes_issues) — GitLab's own reading of the
// closing pattern, including the issue an MR was created from. Issues of other
// projects are dropped.
func (c *Client) ClosesIssues(ctx context.Context, projectPath string, mrIID int64) ([]int64, error) {
	body, err := c.restGet(ctx, "projects/"+projectPathEsc(projectPath)+"/merge_requests/"+strconv.FormatInt(mrIID, 10)+"/closes_issues")
	if err != nil {
		return nil, err
	}
	var items []struct {
		IID    int64  `json:"iid"`
		WebURL string `json:"web_url"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	var out []int64
	for _, it := range items {
		if it.WebURL != "" && !issueURLInProject(it.WebURL, projectPath) {
			continue
		}
		out = append(out, it.IID)
	}
	return out, nil
}

// issueURLInProject reports whether an issue web URL belongs to projectPath.
func issueURLInProject(webURL, projectPath string) bool {
	m := urlRe.FindStringSubmatch(webURL)
	return m != nil && strings.EqualFold(strings.Trim(m[1], "/"), projectPath)
}

// ── Webhook payloads ───────────────────────────────────────

// hookTime parses the timestamps GitLab puts into hook payloads: RFC 3339 on
// current versions, "2006-01-02 15:04:05 UTC" on older ones.
func hookTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05 MST", "2006-01-02 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

// mrGID renders a numeric MR id as its GraphQL global id, the identity the poll
// stores — so a hook and a poll describe one MR the same way.
func mrGID(id int64) string {
	if id <= 0 {
		return ""
	}
	return "gid://gitlab/MergeRequest/" + strconv.FormatInt(id, 10)
}

// ParseMergeRequestHook reads a merge_request hook body. The hook carries no
// pipeline status — Pipeline stays nil and the stored one is kept.
func ParseMergeRequestHook(body []byte) (MergeRequest, error) {
	var p struct {
		ObjectAttributes *struct {
			ID             int64  `json:"id"`
			IID            int64  `json:"iid"`
			Title          string `json:"title"`
			Description    string `json:"description"`
			URL            string `json:"url"`
			State          string `json:"state"`
			Draft          *bool  `json:"draft"`
			WorkInProgress bool   `json:"work_in_progress"`
			SourceBranch   string `json:"source_branch"`
			TargetBranch   string `json:"target_branch"`
			UpdatedAt      string `json:"updated_at"`
		} `json:"object_attributes"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return MergeRequest{}, err
	}
	a := p.ObjectAttributes
	if a == nil || a.IID <= 0 {
		return MergeRequest{}, fmt.Errorf("merge_request hook without object_attributes.iid")
	}
	draft := a.WorkInProgress
	if a.Draft != nil {
		draft = *a.Draft
	}
	return MergeRequest{
		GlobalID: mrGID(a.ID), IID: a.IID, Title: a.Title, Description: a.Description,
		WebURL: a.URL, State: strings.ToLower(a.State), Draft: draft,
		SourceBranch: a.SourceBranch, TargetBranch: a.TargetBranch, UpdatedAt: hookTime(a.UpdatedAt),
	}, nil
}

// PipelineEvent is a pipeline hook reduced to what the task window shows: the
// run, and either the MR it ran for or the branch it ran on.
type PipelineEvent struct {
	Pipeline Pipeline
	MRIID    int64  // the MR of a merge-request pipeline, 0 for a branch pipeline
	Ref      string // the branch (or tag) the pipeline ran on
}

// ParsePipelineHook reads a pipeline hook body. GitLab sends the pipeline's web
// URL only on newer versions; older ones get it built from the project URL.
func ParsePipelineHook(body []byte) (PipelineEvent, error) {
	var p struct {
		ObjectAttributes *struct {
			ID     int64  `json:"id"`
			Ref    string `json:"ref"`
			Status string `json:"status"`
			URL    string `json:"url"`
		} `json:"object_attributes"`
		MergeRequest *struct {
			IID int64 `json:"iid"`
		} `json:"merge_request"`
		Project struct {
			WebURL string `json:"web_url"`
		} `json:"project"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return PipelineEvent{}, err
	}
	a := p.ObjectAttributes
	if a == nil || a.ID <= 0 {
		return PipelineEvent{}, fmt.Errorf("pipeline hook without object_attributes.id")
	}
	ev := PipelineEvent{
		Pipeline: Pipeline{ID: a.ID, Status: strings.ToLower(a.Status), WebURL: a.URL},
		Ref:      a.Ref,
	}
	if ev.Pipeline.WebURL == "" && p.Project.WebURL != "" {
		ev.Pipeline.WebURL = strings.TrimRight(p.Project.WebURL, "/") + "/-/pipelines/" + strconv.FormatInt(a.ID, 10)
	}
	if p.MergeRequest != nil {
		ev.MRIID = p.MergeRequest.IID
	}
	return ev, nil
}
