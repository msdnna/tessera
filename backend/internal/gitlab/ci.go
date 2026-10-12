package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

// ── Running CI (#3099) ──────────────────────────────────────
//
// The palette's «>deploy» / «>ci» commands start a pipeline of the bound project
// on a ref with variables, or retry its last failed pipeline. The handler layer
// reaches these through a provider-neutral seam (handlers/ci_run.go).

// PipelineRun is a pipeline as the run answers it.
type PipelineRun struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	WebURL string `json:"web_url"`
}

// DefaultBranch returns the project's default branch.
func (c *Client) DefaultBranch(ctx context.Context, projectPath string) (string, error) {
	body, err := c.restGet(ctx, "projects/"+projectPathEsc(projectPath))
	if err != nil {
		return "", err
	}
	var p struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", err
	}
	return p.DefaultBranch, nil
}

// CreatePipeline starts a pipeline on ref with variables (POST /projects/:id/pipeline).
func (c *Client) CreatePipeline(ctx context.Context, projectPath, ref string, vars map[string]string) (PipelineRun, error) {
	form := url.Values{"ref": {ref}}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		form.Add("variables[][key]", k)
		form.Add("variables[][value]", vars[k])
	}
	body, err := c.restForm(ctx, http.MethodPost, "projects/"+projectPathEsc(projectPath)+"/pipeline", form)
	if err != nil {
		return PipelineRun{}, err
	}
	var run PipelineRun
	err = json.Unmarshal(body, &run)
	return run, err
}

// RetryLastFailedPipeline retries the newest failed pipeline (on ref, when given).
// ok=false when there is none.
func (c *Client) RetryLastFailedPipeline(ctx context.Context, projectPath, ref string) (PipelineRun, bool, error) {
	q := url.Values{"status": {"failed"}, "per_page": {"1"}, "order_by": {"id"}, "sort": {"desc"}}
	if ref != "" {
		q.Set("ref", ref)
	}
	base := "projects/" + projectPathEsc(projectPath) + "/pipelines"
	body, err := c.restGet(ctx, base+"?"+q.Encode())
	if err != nil {
		return PipelineRun{}, false, err
	}
	var list []PipelineRun
	if err := json.Unmarshal(body, &list); err != nil {
		return PipelineRun{}, false, fmt.Errorf("pipelines: %w", err)
	}
	if len(list) == 0 {
		return PipelineRun{}, false, nil
	}
	body, err = c.restForm(ctx, http.MethodPost, base+"/"+strconv.FormatInt(list[0].ID, 10)+"/retry", url.Values{})
	if err != nil {
		return PipelineRun{}, false, err
	}
	var run PipelineRun
	err = json.Unmarshal(body, &run)
	return run, true, err
}
