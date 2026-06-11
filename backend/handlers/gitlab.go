package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
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
	Configured      bool         `json:"configured"`
	ProjectPath     string       `json:"project_path"`
	BoardID         *uuid.UUID   `json:"board_id"`
	Enabled         bool         `json:"enabled"`
	SyncIntervalSec int32        `json:"sync_interval_sec"`
	LastSyncedAt    *time.Time   `json:"last_synced_at"`
	LabelRules      gitlab.Rules `json:"label_rules"`
}

// integrationView projects a stored integration row into its JSON view.
func integrationView(integ db.GitlabIntegration) gitlabIntegrationView {
	bid := integ.BoardID
	return gitlabIntegrationView{
		Configured: true, ProjectPath: integ.ProjectPath, BoardID: &bid,
		Enabled: integ.Enabled, SyncIntervalSec: integ.SyncIntervalSec,
		LastSyncedAt: integ.LastSyncedAt, LabelRules: parseRules(integ.LabelRules),
	}
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
	c.JSON(http.StatusOK, integrationView(integ))
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
		ProjectPath     string          `json:"project_path" binding:"required"`
		BoardID         uuid.UUID       `json:"board_id" binding:"required"`
		Enabled         bool            `json:"enabled"`
		SyncIntervalSec int32           `json:"sync_interval_sec"`
		LabelRules      json.RawMessage `json:"label_rules"`
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
	if req.SyncIntervalSec < 0 {
		req.SyncIntervalSec = 0
	}

	rules := req.LabelRules
	if len(rules) == 0 || string(rules) == "null" || string(rules) == "{}" {
		// Seed with defaults on first configuration.
		def, _ := json.Marshal(gitlab.DefaultRules())
		rules = def
	}

	// The configuring user's credential drives unattended sync.
	owner := middleware.CurrentUser(c)
	integ, err := h.q.UpsertGitlabIntegration(c, db.UpsertGitlabIntegrationParams{
		WorkspaceID:     wsID,
		ProjectPath:     strings.TrimSpace(req.ProjectPath),
		BoardID:         req.BoardID,
		LabelRules:      rules,
		Enabled:         req.Enabled,
		OwnerUserID:     &owner,
		SyncIntervalSec: req.SyncIntervalSec,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, integrationView(integ))
}

// ── Sync (pull) ────────────────────────────────────────────

// SyncGitlab runs an on-demand pull for the workspace's integration, using the
// requesting user's own credential.
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

	uid := middleware.CurrentUser(c)
	cred, err := h.q.GetGitlabCredential(c, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect your GitLab account first"})
		return
	}
	if err != nil {
		fail(c)
		return
	}

	created, updated, err := h.runSync(c, integ, cred, uid)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitLab sync failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": created + updated, "created": created, "updated": updated})
}

// runSync is the credential-driven pull engine, decoupled from the HTTP request
// so the background worker can call it too. It mirrors the issues assigned to
// cred's GitLab user onto the integration's board (status→column, priority→
// priority, others→tags), attributing events to actorID. Pull-only. Per-issue
// errors are logged and skipped; only a fetch/board-level failure aborts.
func (h *API) runSync(ctx context.Context, integ db.GitlabIntegration, cred db.GitlabCredential, actorID uuid.UUID) (created, updated int, err error) {
	token, err := h.sealer.Decrypt(cred.TokenEnc)
	if err != nil {
		return 0, 0, fmt.Errorf("decrypt stored token: %w", err)
	}
	client := gitlab.New(cred.BaseUrl, token)
	issues, err := client.AssignedIssues(ctx, integ.ProjectPath, cred.GlUsername)
	if err != nil {
		return 0, 0, err
	}

	board, err := h.q.GetBoard(ctx, integ.BoardID)
	if err != nil {
		return 0, 0, fmt.Errorf("integration board no longer exists")
	}
	cols, err := h.q.ListColumns(ctx, board.ID)
	if err != nil || len(cols) == 0 {
		return 0, 0, fmt.Errorf("integration board has no columns")
	}
	colByName := make(map[string]db.BoardColumn, len(cols))
	for _, col := range cols {
		colByName[col.Name] = col
	}
	doneID := h.resolveDoneColumn(ctx, board)
	rules := parseRules(integ.LabelRules)
	wsID := integ.WorkspaceID

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

		link, lerr := h.q.GetGitlabLinkByGlobalID(ctx, db.GetGitlabLinkByGlobalIDParams{
			IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
		})
		switch {
		case errors.Is(lerr, pgx.ErrNoRows):
			t, cerr := h.syncCreateTask(ctx, wsID, board.ID, col.ID, issue, res.Priority, completedAt)
			if cerr != nil {
				log.Printf("gitlab sync: create task for issue !%d failed: %v", issue.IID, cerr)
				continue
			}
			if _, cerr := h.q.CreateGitlabLink(ctx, db.CreateGitlabLinkParams{
				TaskID: t.ID, IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
				GlIid: issue.IID, GlProjectPath: integ.ProjectPath, GlWebUrl: issue.WebURL,
				GlUpdatedAt: issue.UpdatedAt,
				TitleHash:   hashStr(issue.Title), DescHash: hashStr(issue.Description),
				LabelsHash: hashStr(labelsKey(issue.Labels)),
				GlAuthor:   issue.AuthorLogin, GlAuthorName: issue.AuthorName,
			}); cerr != nil {
				log.Printf("gitlab sync: link issue !%d failed: %v", issue.IID, cerr)
				continue
			}
			h.applyTags(ctx, t.ID, wsID, res.Tags)
			h.logEventActor(ctx, t.ID, actorID, "synced", map[string]any{"source": "gitlab", "iid": issue.IID, "url": issue.WebURL})
			h.broadcast(wsID, "task.created", t)
			created++
		case lerr != nil:
			log.Printf("gitlab sync: lookup link for issue !%d failed: %v", issue.IID, lerr)
			continue
		default:
			t, uerr := h.q.SyncUpdateTask(ctx, db.SyncUpdateTaskParams{
				ID: link.TaskID, Title: issue.Title, Description: issue.Description,
				Priority: res.Priority, ColumnID: col.ID, CompletedAt: completedAt,
			})
			if uerr != nil {
				log.Printf("gitlab sync: update task for issue !%d failed: %v", issue.IID, uerr)
				continue
			}
			if _, uerr := h.q.UpdateGitlabLink(ctx, db.UpdateGitlabLinkParams{
				TaskID: link.TaskID, GlIid: issue.IID, GlWebUrl: issue.WebURL,
				GlUpdatedAt: issue.UpdatedAt,
				TitleHash:   hashStr(issue.Title), DescHash: hashStr(issue.Description),
				LabelsHash: hashStr(labelsKey(issue.Labels)),
				GlAuthor:   issue.AuthorLogin, GlAuthorName: issue.AuthorName,
			}); uerr != nil {
				log.Printf("gitlab sync: update link for issue !%d failed: %v", issue.IID, uerr)
				continue
			}
			// Sync the due date only when GitLab has one — otherwise a
			// manually-set Tessera due date is preserved (never reset).
			if issue.DueDate != nil {
				_ = h.q.UpdateTaskDueDate(ctx, db.UpdateTaskDueDateParams{ID: link.TaskID, DueDate: issue.DueDate})
				t.DueDate = issue.DueDate
			}
			h.applyTags(ctx, link.TaskID, wsID, res.Tags)
			h.broadcast(wsID, "task.updated", t)
			updated++
		}
	}

	_ = h.q.MarkGitlabSynced(ctx, integ.ID)
	return created, updated, nil
}

// ── Background auto-sync worker ────────────────────────────

// RunSyncWorker periodically pulls integrations that are due by their configured
// interval, driven by the owner's stored credential. Blocks until ctx is done;
// start it in a goroutine.
func (h *API) RunSyncWorker(ctx context.Context) {
	const tick = 30 * time.Second
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.autoSyncDue(ctx)
		}
	}
}

// autoSyncDue syncs every integration whose interval has elapsed (the query
// already filters for enabled + interval>0 + owner set + due).
func (h *API) autoSyncDue(ctx context.Context) {
	integs, err := h.q.ListAutoSyncIntegrations(ctx)
	if err != nil {
		return
	}
	for _, integ := range integs {
		if integ.OwnerUserID == nil {
			continue
		}
		cred, err := h.q.GetGitlabCredential(ctx, *integ.OwnerUserID)
		if err != nil {
			continue // owner disconnected — skip until reconfigured
		}
		created, updated, serr := h.runSync(ctx, integ, cred, *integ.OwnerUserID)
		if serr != nil {
			log.Printf("gitlab auto-sync ws=%s: %v", integ.WorkspaceID, serr)
			continue
		}
		if created+updated > 0 {
			log.Printf("gitlab auto-sync ws=%s: +%d new, ~%d updated", integ.WorkspaceID, created, updated)
		}
	}
}

// ── helpers ────────────────────────────────────────────────

// resolveDoneColumn is the context-based form of doneColumnID for use off the
// request path: the board's explicit done column, else its rightmost column.
func (h *API) resolveDoneColumn(ctx context.Context, board db.Board) *uuid.UUID {
	if board.DoneColumnID != nil {
		return board.DoneColumnID
	}
	col, err := h.q.RightmostColumn(ctx, board.ID)
	if err != nil {
		return nil
	}
	return &col.ID
}

// syncCreateTask creates a mirrored task at the end of its target column. The
// task has no Tessera creator (created_by stays null) — the GitLab author is
// recorded on the link instead, since it may not be a Tessera user.
func (h *API) syncCreateTask(ctx context.Context, wsID, boardID, columnID uuid.UUID, issue gitlab.Issue, priority int32, completedAt *time.Time) (db.Task, error) {
	maxPos, err := h.q.MaxTaskPositionInColumn(ctx, columnID)
	if err != nil {
		return db.Task{}, err
	}
	num, err := h.q.NextWorkspaceTaskNumber(ctx, wsID)
	if err != nil {
		return db.Task{}, err
	}
	t, err := h.q.CreateTask(ctx, db.CreateTaskParams{
		BoardID: boardID, ColumnID: columnID, ParentID: nil,
		Title: issue.Title, Description: issue.Description, Priority: priority,
		DueDate: issue.DueDate, Position: positionBetween(&maxPos, nil), CreatedBy: nil, Number: &num,
	})
	if err != nil {
		return db.Task{}, err
	}
	if completedAt != nil {
		if done, derr := h.q.SyncUpdateTask(ctx, db.SyncUpdateTaskParams{
			ID: t.ID, Title: t.Title, Description: t.Description, Priority: t.Priority,
			ColumnID: t.ColumnID, CompletedAt: completedAt,
		}); derr == nil {
			t = done
		}
	}
	return t, nil
}

// applyTags ensures each resolved tag exists in the workspace (with the GitLab
// label colour, or a stable auto-colour when GitLab supplies none) and attaches
// it to the task. Additive only: tags the user applied manually are never
// removed, so synced labels never clobber scope tags. Best-effort per tag.
func (h *API) applyTags(ctx context.Context, taskID, wsID uuid.UUID, tags []gitlab.Tag) {
	for _, t := range tags {
		color := t.Color
		if color == "" {
			color = autoTagColor(t.Name)
		}
		tag, err := h.q.EnsureTag(ctx, db.EnsureTagParams{WorkspaceID: wsID, Name: t.Name, Color: color})
		if err != nil {
			continue
		}
		_ = h.q.AddTaskTag(ctx, db.AddTaskTagParams{TaskID: taskID, TagID: tag.ID})
	}
}

// tagPalette is a small set of pleasant accent colours used to auto-colour a
// synced tag when its GitLab label has no colour.
var tagPalette = []string{
	"#7c5cff", "#2f80ed", "#18a058", "#e0a200", "#d0021b", "#9013fe",
	"#0fb5ba", "#f25f4c", "#3aaed8", "#c2596b", "#5a8f3c", "#b06f2f",
}

// autoTagColor picks a deterministic palette colour from a tag name.
func autoTagColor(name string) string {
	hsh := fnv.New32a()
	_, _ = hsh.Write([]byte(name))
	return tagPalette[int(hsh.Sum32())%len(tagPalette)]
}

// labelsKey joins label titles for the link's snapshot hash.
func labelsKey(labels []gitlab.Label) string {
	titles := make([]string, len(labels))
	for i, l := range labels {
		titles[i] = l.Title
	}
	return strings.Join(titles, "\n")
}

// gitlabLinkView is the GitLab provenance attached to a synced task: the issue
// number, its web URL and the author login (a GitLab identity that may not be a
// Tessera user).
type gitlabLinkView struct {
	IID         int64  `json:"iid"`
	WebURL      string `json:"web_url"`
	Author      string `json:"author"`
	AuthorName  string `json:"author_name"`
	ProjectPath string `json:"project_path"`
}

// gitlabLinkForTask returns the GitLab link view for a task, or nil when the
// task isn't mirrored from GitLab. Best-effort.
func (h *API) gitlabLinkForTask(c *gin.Context, taskID uuid.UUID) *gitlabLinkView {
	link, err := h.q.GetGitlabLinkByTask(c, taskID)
	if err != nil {
		return nil
	}
	return &gitlabLinkView{
		IID: link.GlIid, WebURL: link.GlWebUrl, Author: link.GlAuthor,
		AuthorName: link.GlAuthorName, ProjectPath: link.GlProjectPath,
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
