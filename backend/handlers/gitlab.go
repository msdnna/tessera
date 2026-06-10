package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// ── Per-user connection (PAT) ──────────────────────────────

// GetGitlabConnection reports whether the current user has linked a GitLab
// account (never returns the token itself).
func (h *API) GetGitlabConnection(c *gin.Context) {
	cred, err := h.q.GetGitlabCredential(c, middleware.CurrentUser(c))
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusOK, gin.H{"connected": false})
		return
	}
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"connected": true, "base_url": cred.BaseUrl, "gl_username": cred.GlUsername,
	})
}

// ConnectGitlab validates a base URL + personal access token by resolving the
// token's owner, then stores the token encrypted for the current user.
func (h *API) ConnectGitlab(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url" binding:"required"`
		Token   string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")

	client := gitlab.New(req.BaseURL, req.Token)
	user, err := client.CurrentUser(c)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not authenticate with GitLab: " + err.Error()})
		return
	}

	enc, err := h.sealer.Encrypt(req.Token)
	if err != nil {
		fail(c)
		return
	}
	cred, err := h.q.UpsertGitlabCredential(c, db.UpsertGitlabCredentialParams{
		UserID:     middleware.CurrentUser(c),
		BaseUrl:    req.BaseURL,
		TokenEnc:   enc,
		GlUserID:   user.ID,
		GlUsername: user.Username,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"connected": true, "base_url": cred.BaseUrl, "gl_username": cred.GlUsername,
	})
}

// DisconnectGitlab removes the current user's stored credential.
func (h *API) DisconnectGitlab(c *gin.Context) {
	if err := h.q.DeleteGitlabCredential(c, middleware.CurrentUser(c)); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Per-workspace integration config ───────────────────────

// gitlabIntegrationView is the JSON shape returned to the client (label_rules
// decoded from JSONB into the typed rule engine config).
type gitlabIntegrationView struct {
	Configured  bool         `json:"configured"`
	ProjectPath string       `json:"project_path"`
	BoardID     *uuid.UUID   `json:"board_id"`
	Enabled     bool         `json:"enabled"`
	LabelRules  gitlab.Rules `json:"label_rules"`
}

// GetGitlabIntegration returns the workspace's integration config, or an
// unconfigured view pre-filled with default rules so the UI can render a form.
func (h *API) GetGitlabIntegration(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	integ, err := h.q.GetGitlabIntegrationByWorkspace(c, wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusOK, gitlabIntegrationView{LabelRules: gitlab.DefaultRules()})
		return
	}
	if err != nil {
		fail(c)
		return
	}
	bid := integ.BoardID
	c.JSON(http.StatusOK, gitlabIntegrationView{
		Configured: true, ProjectPath: integ.ProjectPath, BoardID: &bid,
		Enabled: integ.Enabled, LabelRules: parseRules(integ.LabelRules),
	})
}

// SetGitlabIntegration creates or updates the workspace's integration config.
func (h *API) SetGitlabIntegration(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		ProjectPath string          `json:"project_path" binding:"required"`
		BoardID     uuid.UUID       `json:"board_id" binding:"required"`
		Enabled     bool            `json:"enabled"`
		LabelRules  json.RawMessage `json:"label_rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// The target board must live in this workspace.
	boardWs, err := h.q.WorkspaceIDForBoard(c, req.BoardID)
	if err != nil || boardWs != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board does not belong to this workspace"})
		return
	}

	rules := req.LabelRules
	if len(rules) == 0 || string(rules) == "null" || string(rules) == "{}" {
		// Seed with defaults on first configuration.
		def, _ := json.Marshal(gitlab.DefaultRules())
		rules = def
	}

	integ, err := h.q.UpsertGitlabIntegration(c, db.UpsertGitlabIntegrationParams{
		WorkspaceID: wsID,
		ProjectPath: strings.TrimSpace(req.ProjectPath),
		BoardID:     req.BoardID,
		LabelRules:  rules,
		Enabled:     req.Enabled,
	})
	if err != nil {
		fail(c)
		return
	}
	bid := integ.BoardID
	c.JSON(http.StatusOK, gitlabIntegrationView{
		Configured: true, ProjectPath: integ.ProjectPath, BoardID: &bid,
		Enabled: integ.Enabled, LabelRules: parseRules(integ.LabelRules),
	})
}

// ── Manual sync (pull) ─────────────────────────────────────

// SyncGitlab pulls the issues assigned to the current user from the configured
// GitLab project and mirrors them onto the integration's board: status label →
// column, priority label → priority, other labels → tags. Pull-only — never
// writes back to GitLab. Existing links are updated in place; new issues create
// tasks.
func (h *API) SyncGitlab(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}

	integ, err := h.q.GetGitlabIntegrationByWorkspace(c, wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no GitLab integration configured for this workspace"})
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !integ.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "GitLab integration is disabled"})
		return
	}

	client, cred, ok := h.gitlabClient(c)
	if !ok {
		return
	}

	issues, err := client.AssignedIssues(c, integ.ProjectPath, cred.GlUsername)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitLab fetch failed: " + err.Error()})
		return
	}

	board, err := h.q.GetBoard(c, integ.BoardID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "integration board no longer exists"})
		return
	}
	cols, err := h.q.ListColumns(c, board.ID)
	if err != nil || len(cols) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "integration board has no columns"})
		return
	}
	colByName := make(map[string]db.BoardColumn, len(cols))
	for _, col := range cols {
		colByName[col.Name] = col
	}
	doneID := h.doneColumnID(c, board)
	rules := parseRules(integ.LabelRules)
	uid := middleware.CurrentUser(c)

	var created, updated int
	for _, issue := range issues {
		res := rules.Resolve(issue.Labels)
		col, found := colByName[res.ColumnName]
		if !found {
			col = cols[0] // leftmost column as a last-resort fallback
		}
		var completedAt *time.Time
		if doneID != nil && col.ID == *doneID {
			now := time.Now()
			completedAt = &now
		}

		link, lerr := h.q.GetGitlabLinkByGlobalID(c, db.GetGitlabLinkByGlobalIDParams{
			IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
		})
		switch {
		case errors.Is(lerr, pgx.ErrNoRows):
			t, ok := h.syncCreateTask(c, wsID, board.ID, col.ID, uid, issue, res.Priority, completedAt)
			if !ok {
				return
			}
			if _, err := h.q.CreateGitlabLink(c, db.CreateGitlabLinkParams{
				TaskID: t.ID, IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
				GlIid: issue.IID, GlProjectPath: integ.ProjectPath, GlWebUrl: issue.WebURL,
				GlUpdatedAt: issue.UpdatedAt,
				TitleHash:   hashStr(issue.Title), DescHash: hashStr(issue.Description),
				LabelsHash: hashStr(strings.Join(issue.Labels, "\n")),
			}); err != nil {
				fail(c)
				return
			}
			h.applyTags(c, t.ID, wsID, res.TagNames)
			h.logEvent(c, t.ID, "synced", map[string]any{"source": "gitlab", "iid": issue.IID, "url": issue.WebURL})
			h.broadcast(wsID, "task.created", t)
			created++
		case lerr != nil:
			fail(c)
			return
		default:
			t, err := h.q.SyncUpdateTask(c, db.SyncUpdateTaskParams{
				ID: link.TaskID, Title: issue.Title, Description: issue.Description,
				Priority: res.Priority, ColumnID: col.ID, CompletedAt: completedAt,
			})
			if err != nil {
				fail(c)
				return
			}
			if _, err := h.q.UpdateGitlabLink(c, db.UpdateGitlabLinkParams{
				TaskID: link.TaskID, GlIid: issue.IID, GlWebUrl: issue.WebURL,
				GlUpdatedAt: issue.UpdatedAt,
				TitleHash:   hashStr(issue.Title), DescHash: hashStr(issue.Description),
				LabelsHash: hashStr(strings.Join(issue.Labels, "\n")),
			}); err != nil {
				fail(c)
				return
			}
			h.applyTags(c, link.TaskID, wsID, res.TagNames)
			h.broadcast(wsID, "task.updated", t)
			updated++
		}
	}

	c.JSON(http.StatusOK, gin.H{"total": len(issues), "created": created, "updated": updated})
}

// ── helpers ────────────────────────────────────────────────

// gitlabClient loads the current user's credential and builds an authenticated
// client. Writes a 400 and returns ok=false when no credential is linked.
func (h *API) gitlabClient(c *gin.Context) (*gitlab.Client, db.GitlabCredential, bool) {
	cred, err := h.q.GetGitlabCredential(c, middleware.CurrentUser(c))
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect your GitLab account first"})
		return nil, db.GitlabCredential{}, false
	}
	if err != nil {
		fail(c)
		return nil, db.GitlabCredential{}, false
	}
	token, err := h.sealer.Decrypt(cred.TokenEnc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stored GitLab token could not be decrypted"})
		return nil, db.GitlabCredential{}, false
	}
	return gitlab.New(cred.BaseUrl, token), cred, true
}

// syncCreateTask creates a mirrored task at the end of its target column.
func (h *API) syncCreateTask(c *gin.Context, wsID, boardID, columnID, uid uuid.UUID, issue gitlab.Issue, priority int32, completedAt *time.Time) (db.Task, bool) {
	pos, err := h.nextTaskPosition(c, columnID, nil)
	if err != nil {
		fail(c)
		return db.Task{}, false
	}
	num, err := h.q.NextWorkspaceTaskNumber(c, wsID)
	if err != nil {
		fail(c)
		return db.Task{}, false
	}
	t, err := h.q.CreateTask(c, db.CreateTaskParams{
		BoardID: boardID, ColumnID: columnID, ParentID: nil,
		Title: issue.Title, Description: issue.Description, Priority: priority,
		DueDate: nil, Position: pos, CreatedBy: &uid, Number: &num,
	})
	if err != nil {
		fail(c)
		return db.Task{}, false
	}
	if completedAt != nil {
		if done, derr := h.q.SyncUpdateTask(c, db.SyncUpdateTaskParams{
			ID: t.ID, Title: t.Title, Description: t.Description, Priority: t.Priority,
			ColumnID: t.ColumnID, CompletedAt: completedAt,
		}); derr == nil {
			t = done
		}
	}
	return t, true
}

// applyTags ensures each named tag exists in the workspace and attaches it to
// the task. Additive only: tags the user applied manually are never removed,
// so synced labels never clobber scope tags. Best-effort per tag.
func (h *API) applyTags(c *gin.Context, taskID, wsID uuid.UUID, names []string) {
	for _, name := range names {
		tag, err := h.q.EnsureTag(c, db.EnsureTagParams{WorkspaceID: wsID, Name: name, Color: ""})
		if err != nil {
			continue
		}
		_ = h.q.AddTaskTag(c, db.AddTaskTagParams{TaskID: taskID, TagID: tag.ID})
	}
}

// parseRules decodes the JSONB rule config, falling back to defaults when it's
// empty or unset.
func parseRules(raw []byte) gitlab.Rules {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return gitlab.DefaultRules()
	}
	var r gitlab.Rules
	if err := json.Unmarshal(raw, &r); err != nil {
		return gitlab.DefaultRules()
	}
	if r.StatusPrefix == "" && len(r.StatusToColumn) == 0 && len(r.PriorityMap) == 0 {
		return gitlab.DefaultRules()
	}
	return r
}

// hashStr is a content snapshot helper (sha256 hex) for the link's *_hash
// columns — unused in the pull-only slice but recorded for a future write-back.
func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
