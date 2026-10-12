package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/gitlab"
	"tessera/middleware"
)

// ── Running CI from a command (#3099) ───────────────────────
//
// POST /boards/:id/ci/run {mode: "run"|"retry_failed", ref?, variables?,
// integration_id?} — what «>deploy» and «>ci» call. The core knows only a
// provider-neutral CI runner; GitLab is today's one (the board's binding, under
// the acting user's connection, like every other write to GitLab). A second
// provider plugs in behind ciRunner without touching the route.

// ciRun is the provider-neutral answer.
type ciRun struct {
	Provider string `json:"provider"`
	Project  string `json:"project"`
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Ref      string `json:"ref"`
	URL      string `json:"url"`
}

// ciRunner starts a pipeline or retries the last failed one. ok=false from
// RetryFailed: there was nothing to retry.
type ciRunner interface {
	Run(ctx context.Context, ref string, vars map[string]string) (ciRun, error)
	RetryFailed(ctx context.Context, ref string) (ciRun, bool, error)
}

type gitlabCI struct {
	client  *gitlab.Client
	project string
}

func (g gitlabCI) view(p gitlab.PipelineRun) ciRun {
	return ciRun{Provider: "gitlab", Project: g.project, ID: p.ID, Status: p.Status, Ref: p.Ref, URL: p.WebURL}
}

func (g gitlabCI) Run(ctx context.Context, ref string, vars map[string]string) (ciRun, error) {
	if ref == "" {
		def, err := g.client.DefaultBranch(ctx, g.project)
		if err != nil {
			return ciRun{}, err
		}
		ref = def
	}
	p, err := g.client.CreatePipeline(ctx, g.project, ref, vars)
	return g.view(p), err
}

func (g gitlabCI) RetryFailed(ctx context.Context, ref string) (ciRun, bool, error) {
	p, ok, err := g.client.RetryLastFailedPipeline(ctx, g.project, ref)
	return g.view(p), ok, err
}

// RunCI — POST /boards/:id/ci/run.
func (h *API) RunCI(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, boardID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Mode          string            `json:"mode"`
		Ref           string            `json:"ref"`
		Variables     map[string]string `json:"variables"`
		IntegrationID *uuid.UUID        `json:"integration_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Ref = strings.TrimSpace(req.Ref)
	if req.Mode != "run" && req.Mode != "retry_failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode: run | retry_failed"})
		return
	}
	if len(req.Variables) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many variables"})
		return
	}
	integ, ok := h.boardIntegration(c, uuid.Nil, boardID, req.IntegrationID)
	if !ok {
		return
	}
	actor := middleware.CurrentUser(c)
	baseURL, token, sudo, ok := h.writeGitlabConn(c, &actor, integ.OwnerUserID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect a GitLab account first, or ask an admin to set a service token"})
		return
	}
	var runner ciRunner = gitlabCI{client: gitlab.New(baseURL, token).WithSudo(sudo), project: integ.ProjectPath}

	var run ciRun
	if req.Mode == "retry_failed" {
		var found bool
		run, found, err = runner.RetryFailed(c, req.Ref)
		if err == nil && !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "no failed pipeline to retry", "code": "no_failed_pipeline"})
			return
		}
	} else {
		run, err = runner.Run(c, req.Ref, req.Variables)
	}
	if err != nil {
		var apiErr *gitlab.APIError
		if errors.As(err, &apiErr) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "GitLab refused the pipeline", "status": apiErr.Status, "detail": apiErr.Body})
			return
		}
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}
