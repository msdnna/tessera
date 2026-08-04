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
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/internal/jobs"
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
	Configured      bool             `json:"configured"`
	ID              *uuid.UUID       `json:"id"`
	Name            string           `json:"name"`
	ProjectPath     string           `json:"project_path"`
	BoardID         *uuid.UUID       `json:"board_id"`
	ProjectID       *uuid.UUID       `json:"project_id"` // the integration board's project (for milestone gating)
	Enabled             bool             `json:"enabled"`
	SyncIntervalSec     int32            `json:"sync_interval_sec"`
	FullSyncIntervalSec int32            `json:"full_sync_interval_sec"`
	DueSource           string           `json:"due_source"`
	StartSource         string           `json:"start_source"`
	Scope               string           `json:"scope"`
	ClosedPolicy        string           `json:"closed_policy"`
	ClosedAfter         *time.Time       `json:"closed_after"`
	LastSyncedAt        *time.Time       `json:"last_synced_at"`
	LastFullSyncedAt    *time.Time       `json:"last_full_synced_at"`
	LabelRules      gitlab.Rules     `json:"label_rules"`
	Writeback       gitlab.Writeback `json:"writeback"`
	// Resolved estimation unit for the integration board (project→workspace→time),
	// so the UI can disable the estimate write-back toggle when it isn't "time".
	EstimationUnit string `json:"estimation_unit,omitempty"`
}

// integrationView projects a stored integration row into its JSON view.
func integrationView(integ db.GitlabIntegration) gitlabIntegrationView {
	bid := integ.BoardID
	iid := integ.ID
	return gitlabIntegrationView{
		Configured: true, ID: &iid, Name: integ.Name, ProjectPath: integ.ProjectPath, BoardID: &bid,
		Enabled: integ.Enabled, SyncIntervalSec: integ.SyncIntervalSec,
		FullSyncIntervalSec: integ.FullSyncIntervalSec, LastFullSyncedAt: integ.LastFullSyncedAt,
		DueSource: integ.DueSource, StartSource: integ.StartSource, LastSyncedAt: integ.LastSyncedAt,
		Scope: integ.Scope, ClosedPolicy: integ.ClosedPolicy, ClosedAfter: integ.ClosedAfter,
		LabelRules: parseRules(integ.LabelRules),
		Writeback:  parseWriteback(integ.Writeback),
	}
}

// serviceGitlabConn returns the instance-wide GitLab connection (admin-configured
// service account: base URL + decrypted token) when set. This is the preferred
// credential for all sync/read/write operations in the OAuth era — it decouples the
// integration from any individual user's personal PAT.
func (h *API) serviceGitlabConn(ctx context.Context) (baseURL, token string, ok bool) {
	p, err := h.q.GetOAuthProvider(ctx, "gitlab")
	if err != nil || p.ServiceTokenEnc == "" || p.GlBaseUrl == "" {
		return "", "", false
	}
	tok, derr := h.sealer.Decrypt(p.ServiceTokenEnc)
	if derr != nil {
		return "", "", false
	}
	return strings.TrimRight(p.GlBaseUrl, "/"), tok, true
}

// effectiveGitlabConn resolves the connection for an operation: the instance
// service account when configured, else the given fallback user's personal PAT.
// ok=false means neither is available (the caller should ask to configure GitLab).
func (h *API) effectiveGitlabConn(ctx context.Context, fallbackUserID *uuid.UUID) (baseURL, token string, ok bool) {
	if b, t, sok := h.serviceGitlabConn(ctx); sok {
		return b, t, true
	}
	if fallbackUserID != nil {
		if cred, err := h.q.GetGitlabCredential(ctx, *fallbackUserID); err == nil {
			if t, derr := h.sealer.Decrypt(cred.TokenEnc); derr == nil {
				return cred.BaseUrl, t, true
			}
		}
	}
	return "", "", false
}

// actorGitlabUsername resolves a Tessera user's GitLab username — from a connected
// PAT credential, else their "Login with GitLab" OAuth identity — for attributing
// actions performed under a shared service token. "" when unknown.
func (h *API) actorGitlabUsername(ctx context.Context, userID uuid.UUID) string {
	if cred, err := h.q.GetGitlabCredential(ctx, userID); err == nil && cred.GlUsername != "" {
		return cred.GlUsername
	}
	if u, err := h.q.GetGitlabUsernameForUser(ctx, userID); err == nil {
		return u
	}
	return ""
}

// fullIntegrationView enriches a stored row with the resolved estimation unit and
// the board's project id.
func (h *API) fullIntegrationView(c *gin.Context, integ db.GitlabIntegration) gitlabIntegrationView {
	view := integrationView(integ)
	view.EstimationUnit = h.integrationEstimationUnit(c, integ)
	if pid, perr := h.q.ProjectIDForBoard(c, integ.BoardID); perr == nil {
		view.ProjectID = &pid
	}
	return view
}

// ListGitlabIntegrations returns every GitLab binding of the workspace plus the
// default label rules so the UI can pre-fill a new-binding form.
func (h *API) ListGitlabIntegrations(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListGitlabIntegrationsByWorkspace(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	views := make([]gitlabIntegrationView, 0, len(rows))
	for _, integ := range rows {
		views = append(views, h.fullIntegrationView(c, integ))
	}
	// service_configured: an instance-wide service token is set, so bindings can be
	// configured/synced without a personal PAT. is_admin: only admins may mutate.
	_, _, serviceOK := h.serviceGitlabConn(c)
	caller, _ := h.q.GetUserByID(c, middleware.CurrentUser(c))
	c.JSON(http.StatusOK, gin.H{
		"integrations":       views,
		"default_rules":      gitlab.DefaultRules(),
		"service_configured": serviceOK,
		"is_admin":           caller.IsAdmin,
	})
}

// integrationRequest is the shared create/update body for a binding.
type integrationRequest struct {
	Name            string          `json:"name"`
	ProjectPath     string          `json:"project_path" binding:"required"`
	BoardID         uuid.UUID       `json:"board_id" binding:"required"`
	Enabled             bool            `json:"enabled"`
	SyncIntervalSec     int32           `json:"sync_interval_sec"`
	FullSyncIntervalSec *int32          `json:"full_sync_interval_sec"` // nil = absent (default 24h); 0 = disabled
	DueSource           string          `json:"due_source"`
	StartSource         string          `json:"start_source"`
	Scope               string          `json:"scope"`
	ClosedPolicy        string          `json:"closed_policy"`
	ClosedAfter         *time.Time      `json:"closed_after"`
	LabelRules          json.RawMessage `json:"label_rules"`
	Writeback           json.RawMessage `json:"writeback"`
}

// normalize validates and defaults the request fields in place.
func (req *integrationRequest) normalize() {
	if req.SyncIntervalSec < 0 {
		req.SyncIntervalSec = 0
	}
	// full_sync_interval_sec: absent (old client) → default 24h so forced full sweeps
	// aren't silently off; explicit 0 disables them; negatives clamp to 0.
	if req.FullSyncIntervalSec == nil {
		def := int32(86400)
		req.FullSyncIntervalSec = &def
	} else if *req.FullSyncIntervalSec < 0 {
		*req.FullSyncIntervalSec = 0
	}
	switch req.DueSource {
	case "issue", "milestone", "off", "issue_milestone":
	default:
		req.DueSource = "issue_milestone"
	}
	switch req.StartSource {
	case "created", "milestone", "off":
	default:
		req.StartSource = "created"
	}
	switch req.Scope {
	case "assigned", "all":
	default:
		req.Scope = "all"
	}
	switch req.ClosedPolicy {
	case "all", "archive_closed_sprints", "period":
	default:
		req.ClosedPolicy = "archive_closed_sprints"
	}
	if req.ClosedPolicy != "period" {
		req.ClosedAfter = nil
	}
	req.ProjectPath = strings.TrimSpace(req.ProjectPath)
	req.Name = strings.TrimSpace(req.Name)
}

// rulesOrDefault seeds label rules with the default taxonomy when empty.
func rulesOrDefault(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		def, _ := json.Marshal(gitlab.DefaultRules())
		return def
	}
	return raw
}

// CreateGitlabIntegration adds a new GitLab project → board binding to the workspace.
func (h *API) CreateGitlabIntegration(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	// GitLab is an instance-wide integration: only a global admin may (re)configure
	// bindings. Members can still see the config (list) and trigger a sync.
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	var req integrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	boardWs, err := h.q.WorkspaceIDForBoard(c, req.BoardID)
	if err != nil || boardWs != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board does not belong to this workspace"})
		return
	}
	req.normalize()
	wbRaw, _ := json.Marshal(parseWriteback(req.Writeback))
	owner := middleware.CurrentUser(c)
	integ, err := h.q.CreateGitlabIntegration(c, db.CreateGitlabIntegrationParams{
		WorkspaceID:     wsID,
		Name:            req.Name,
		ProjectPath:     req.ProjectPath,
		BoardID:         req.BoardID,
		LabelRules:      rulesOrDefault(req.LabelRules),
		Enabled:         req.Enabled,
		OwnerUserID:     &owner,
		SyncIntervalSec:     req.SyncIntervalSec,
		FullSyncIntervalSec: *req.FullSyncIntervalSec,
		DueSource:           req.DueSource,
		StartSource:         req.StartSource,
		Writeback:           wbRaw,
		Scope:               req.Scope,
		ClosedPolicy:        req.ClosedPolicy,
		ClosedAfter:         req.ClosedAfter,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this board or project is already bound to a GitLab integration"})
		return
	}
	c.JSON(http.StatusOK, h.fullIntegrationView(c, integ))
}

// UpdateGitlabIntegration edits an existing binding (selected by :integrationId).
func (h *API) UpdateGitlabIntegration(c *gin.Context) {
	integ, wsID, ok := h.integrationInWorkspace(c)
	if !ok {
		return
	}
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	var req integrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	boardWs, err := h.q.WorkspaceIDForBoard(c, req.BoardID)
	if err != nil || boardWs != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board does not belong to this workspace"})
		return
	}
	req.normalize()
	wbRaw, _ := json.Marshal(parseWriteback(req.Writeback))
	// Keep the existing owner credential unless unset (re-owning is a separate concern).
	owner := integ.OwnerUserID
	if owner == nil {
		u := middleware.CurrentUser(c)
		owner = &u
	}
	updated, err := h.q.UpdateGitlabIntegration(c, db.UpdateGitlabIntegrationParams{
		ID:              integ.ID,
		Name:            req.Name,
		ProjectPath:     req.ProjectPath,
		BoardID:         req.BoardID,
		LabelRules:      rulesOrDefault(req.LabelRules),
		Enabled:         req.Enabled,
		OwnerUserID:     owner,
		SyncIntervalSec:     req.SyncIntervalSec,
		FullSyncIntervalSec: *req.FullSyncIntervalSec,
		DueSource:           req.DueSource,
		StartSource:         req.StartSource,
		Writeback:           wbRaw,
		Scope:               req.Scope,
		ClosedPolicy:        req.ClosedPolicy,
		ClosedAfter:         req.ClosedAfter,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this board or project is already bound to a GitLab integration"})
		return
	}
	c.JSON(http.StatusOK, h.fullIntegrationView(c, updated))
}

// DeleteGitlabIntegration removes a binding (its links/journal/writebacks cascade).
func (h *API) DeleteGitlabIntegration(c *gin.Context) {
	integ, _, ok := h.integrationInWorkspace(c)
	if !ok {
		return
	}
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	if err := h.q.DeleteGitlabIntegration(c, integ.ID); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// integrationInWorkspace resolves the :integrationId path param, verifying the
// caller is a member of :id and the binding belongs to that workspace.
func (h *API) integrationInWorkspace(c *gin.Context) (db.GitlabIntegration, uuid.UUID, bool) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return db.GitlabIntegration{}, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return db.GitlabIntegration{}, uuid.Nil, false
	}
	integID, ok := parseID(c, "integrationId")
	if !ok {
		return db.GitlabIntegration{}, uuid.Nil, false
	}
	integ, err := h.q.GetGitlabIntegration(c, integID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && integ.WorkspaceID != wsID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "integration not found"})
		return db.GitlabIntegration{}, uuid.Nil, false
	}
	if err != nil {
		fail(c)
		return db.GitlabIntegration{}, uuid.Nil, false
	}
	return integ, wsID, true
}

// ── Sync (pull) ────────────────────────────────────────────

// SyncGitlab runs an on-demand pull for one binding (:integrationId), using the
// instance service token when configured, else the requesting user's own PAT.
func (h *API) SyncGitlab(c *gin.Context) {
	integ, _, ok := h.integrationInWorkspace(c)
	if !ok {
		return
	}
	if !integ.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "GitLab integration is disabled"})
		return
	}

	uid := middleware.CurrentUser(c)
	// A service token drives the sync when configured (recommended). Only when there
	// is none do we require the caller's personal PAT.
	var cred db.GitlabCredential
	if _, _, hasService := h.serviceGitlabConn(c); !hasService {
		var err error
		cred, err = h.q.GetGitlabCredential(c, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connect your GitLab account first, or ask an admin to set a service token"})
			return
		}
		if err != nil {
			fail(c)
			return
		}
	}

	// A large batch can take minutes — run it detached from the request so no proxy
	// read-timeout (or browser) drops the long connection mid-sync. The caller does
	// NOT wait: the run appears in the journal as "running" straight away and its
	// outcome arrives as a notification. Registering in the jobs registry both guards
	// against an overlapping run (manual + auto) and makes the run cancelable from the
	// admin jobs panel.
	syncCtx, cancel := context.WithCancel(context.Background())
	handle, started := h.jobs.Begin(gitlabSyncKey(integ.ID), gitlabSyncName(integ), jobs.KindSync, integ.WorkspaceID.String(), cancel)
	if !started {
		cancel()
		c.JSON(http.StatusAccepted, gin.H{"started": false, "already_running": true})
		return
	}
	// Open the journal row synchronously so the response can name the run and the
	// journal shows it as in-flight from the moment the button was pressed. The
	// button's main action is an incremental pull; "Полная синхронизация" (dropdown)
	// sends ?mode=full. A never-fully-synced binding is forced to full regardless.
	aid := uid
	j := h.newJournal(integ.ID, "pull", "manual", &aid)
	j.mode = "incremental"
	if c.Query("mode") == "full" || integ.LastFullSyncedAt == nil {
		j.mode = "full"
	}
	handle.SetOp(j.mode + " pull")
	h.beginJournal(c, j)
	go func() {
		defer cancel()
		ctx, tcancel := context.WithTimeout(syncCtx, 30*time.Minute)
		defer tcancel()
		created, updated, serr := h.runSyncJournal(ctx, integ, cred, uid, j)
		handle.SetCounts(created, updated)
		handle.Finish(serr)
		if serr != nil {
			log.Printf("gitlab manual-sync ws=%s: %v", integ.WorkspaceID, serr)
		}
		h.notifySyncFinished(ctx, integ, j, created, updated, serr)
	}()
	resp := gin.H{"started": true}
	if j.runID != nil {
		resp["run_id"] = *j.runID
	}
	c.JSON(http.StatusAccepted, resp)
}

// notifySyncFinished reports a finished background sync to whoever started it.
// A manual sync is fire-and-forget, so this notification (plus the
// provider-neutral integration.sync event that refreshes the journal and board)
// is the only feedback the user gets. Auto runs stay silent — a 5-minute interval
// would otherwise spam the bell.
func (h *API) notifySyncFinished(ctx context.Context, integ db.GitlabIntegration, j *syncJournal, created, updated int, err error) {
	if j == nil || j.trigger != "manual" || j.actorID == nil || *j.actorID == uuid.Nil {
		return
	}
	// The originating request is long gone; keep the context alive for the write.
	ctx = context.WithoutCancel(ctx)
	label := "GitLab · " + integ.ProjectPath
	took := fmtSyncDuration(time.Since(j.startedAt))
	var text string
	switch {
	case err != nil || j.status == "error":
		reason := j.errText
		if err != nil {
			reason = err.Error()
		}
		if reason == "" {
			reason = "неизвестная ошибка"
		}
		text = fmt.Sprintf("%s: синхронизация не удалась — %s (за %s)", label, reason, took)
	case j.status == "partial":
		text = fmt.Sprintf("%s: +%d новых, ~%d обновлено, за %s (часть действий с ошибками)", label, created, updated, took)
	default:
		text = fmt.Sprintf("%s: +%d новых, ~%d обновлено, за %s", label, created, updated, took)
	}
	h.deliverNotification(ctx, *j.actorID, integ.WorkspaceID, nil, nil, "integration_sync", text)
	payload := gin.H{
		"provider": "gitlab", "integration_id": integ.ID, "status": j.status,
		"created": created, "updated": updated,
	}
	if j.runID != nil {
		payload["run_id"] = *j.runID
	}
	h.broadcast(integ.WorkspaceID, "integration.sync", payload)
}

// fmtSyncDuration renders how long a run took, in short Russian units.
func fmtSyncDuration(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	switch {
	case total <= 0:
		return "меньше секунды"
	case total >= 3600:
		return fmt.Sprintf("%d ч %d м", total/3600, (total%3600)/60)
	case total >= 60:
		return fmt.Sprintf("%d м %d с", total/60, total%60)
	default:
		return fmt.Sprintf("%d с", total)
	}
}

// gitlabSyncKey / gitlabSyncName name a GitLab pull in the jobs registry. The key is
// stable per integration so a manual and an auto run can't overlap (busy-guard).
func gitlabSyncKey(integrationID uuid.UUID) string { return "gitlab_sync:" + integrationID.String() }
func gitlabSyncName(integ db.GitlabIntegration) string {
	name := integ.ProjectPath
	if integ.Name != "" {
		name = integ.Name
	}
	return "Синк GitLab · " + name
}

// syncBoard caches a board's columns + done column during a sync run.
type syncBoard struct {
	id     uuid.UUID
	cols   []db.BoardColumn
	byName map[string]db.BoardColumn
	doneID *uuid.UUID
}

// syncOverlap is how far before last_synced_at an incremental pull sets its
// updatedAfter cutoff — a safety window covering clock skew between Tessera and
// GitLab and issues changed during the previous run's own page fetch. Anything it
// re-delivers unchanged is dropped cheaply by dropUnchangedIssues.
const syncOverlap = 5 * time.Minute

// memberRosterTTL is how long an incremental sync trusts the cached project-member
// roster before refreshing it (a full sweep always refreshes).
const memberRosterTTL = time.Hour

// archiveDeletedLinks archives linked tasks whose GitLab issue was absent from a
// full sweep (deleted or made inaccessible in GitLab). `seen` is the set of every
// issue global id the sweep fetched. Full-sync only — an incremental delta can't
// tell "deleted" from "unchanged, so not re-sent".
func (h *API) archiveDeletedLinks(ctx context.Context, integ db.GitlabIntegration, seen map[string]bool, actorID uuid.UUID, j *syncJournal) {
	links, err := h.q.LinkedTasksForIntegration(ctx, integ.ID)
	if err != nil {
		return
	}
	for _, l := range links {
		if seen[l.GlGlobalID] {
			continue
		}
		if aerr := h.q.ArchiveTaskIfActive(ctx, l.TaskID); aerr != nil {
			continue
		}
		tid := l.TaskID
		h.logEventActor(ctx, tid, actorID, "archived", map[string]any{"source": "gitlab", "reason": "issue_deleted"})
		j.add(journalAction{
			Direction: "pull", EntityType: "task", Op: "delete", TaskID: &tid,
			Summary: "issue удалён в GitLab — задача перенесена в архив",
		})
	}
}

// dropUnchangedIssues removes issues whose GitLab updatedAt AND content hashes match
// what we already stored on their link — the overlap window re-delivers already-synced
// issues, and reconciling them again is wasted work. New issues (no link yet) and
// issues with a changed/unknown updatedAt or a differing title/labels hash are kept.
// The hash check guards the second-precision updatedAt: two edits in the same second
// share a timestamp, so a same-second retitle is still caught.
func (h *API) dropUnchangedIssues(ctx context.Context, integrationID uuid.UUID, issues []gitlab.Issue) []gitlab.Issue {
	keys, err := h.q.LinkedSyncKeysForIntegration(ctx, integrationID)
	if err != nil || len(keys) == 0 {
		return issues
	}
	type syncKey struct {
		updated              *time.Time
		titleHash, labelHash string
	}
	seen := make(map[string]syncKey, len(keys))
	for _, k := range keys {
		seen[k.GlGlobalID] = syncKey{updated: k.GlUpdatedAt, titleHash: k.TitleHash, labelHash: k.LabelsHash}
	}
	kept := issues[:0]
	for _, is := range issues {
		k, linked := seen[is.GlobalID]
		unchanged := linked && k.updated != nil && is.UpdatedAt != nil && is.UpdatedAt.Equal(*k.updated) &&
			k.titleHash == hashStr(is.Title) && k.labelHash == hashStr(labelsKey(is.Labels))
		if unchanged {
			continue // linked and unchanged since last sync
		}
		kept = append(kept, is)
	}
	return kept
}

// runSync is the credential-driven pull engine, decoupled from the HTTP request
// so the background worker can call it too. It mirrors the issues assigned to
// cred's GitLab user onto the integration's board (status→column, priority→
// priority, others→tags), attributing events to actorID. Pull-only. Per-issue
// errors are logged and skipped; only a fetch/board-level failure aborts. mode is
// "full" | "incremental" (how issues are fetched).
func (h *API) runSync(ctx context.Context, integ db.GitlabIntegration, cred db.GitlabCredential, actorID uuid.UUID, trigger, mode string) (created, updated int, err error) {
	aid := actorID
	j := h.newJournal(integ.ID, "pull", trigger, &aid)
	j.mode = mode
	return h.runSyncJournal(ctx, integ, cred, actorID, j)
}

// runSyncJournal is runSync against a caller-supplied journal, so a manual sync
// can open its run row (beginJournal) before detaching into the background and
// still have every action land on that same row.
func (h *API) runSyncJournal(ctx context.Context, integ db.GitlabIntegration, cred db.GitlabCredential, actorID uuid.UUID, j *syncJournal) (created, updated int, err error) {
	defer func() {
		if err != nil {
			j.abort(err)
		}
		// Use a cancellation-detached context so the journal still records even when
		// the originating request was cancelled mid-sync.
		h.flushJournal(context.WithoutCancel(ctx), j)
	}()

	// Credential: prefer the instance-wide service account; fall back to the
	// caller-supplied per-user PAT (legacy). assignUser is the GitLab username used
	// by the "assigned" scope — only meaningful for a personal token.
	baseURL, token, assignUser := cred.BaseUrl, "", cred.GlUsername
	if b, t, ok := h.serviceGitlabConn(ctx); ok {
		baseURL, token, assignUser = b, t, "" // a service token has no "assigned to me"
	} else {
		t, derr := h.sealer.Decrypt(cred.TokenEnc)
		if derr != nil {
			return 0, 0, fmt.Errorf("decrypt stored token: %w", derr)
		}
		token = t
	}
	client := gitlab.New(baseURL, token)
	// Discover issues per the integration scope: "all" pulls the whole project
	// (full import), "assigned" only the credential owner's issues. Under a service
	// token there's no "assigned to me", so we always do a full pull.
	//
	// An incremental pull asks GitLab (server-side) only for issues updated after the
	// last sync minus a 5-minute overlap window — the pull is usually near-empty, so
	// it costs one GraphQL page instead of a few thousand. A full pull leaves the
	// filter off and additionally refetches every already-linked issue (to catch a
	// reassignment away from the owner and to detect deletes — Phase 2).
	incremental := j.mode == "incremental"
	var since *time.Time
	if incremental && integ.LastSyncedAt != nil {
		s := integ.LastSyncedAt.Add(-syncOverlap)
		since = &s
	}
	var assigned []gitlab.Issue
	if integ.Scope == "all" || assignUser == "" {
		assigned, err = client.AllIssuesSince(ctx, integ.ProjectPath, since)
	} else {
		assigned, err = client.AssignedIssuesSince(ctx, integ.ProjectPath, assignUser, since)
	}
	if err != nil {
		return 0, 0, err
	}
	linkedIids, _ := h.q.LinkedIidsForIntegration(ctx, integ.ID)
	var issues []gitlab.Issue
	if incremental {
		// Changed linked issues already surface in the delta (updatedAt > since), so
		// skip the full linked-issue refetch. Then drop issues the overlap window
		// re-delivered but whose GitLab updatedAt is unchanged (nothing to do).
		issues = h.dropUnchangedIssues(ctx, integ.ID, assigned)
	} else {
		iidStrs := make([]string, 0, len(linkedIids))
		for _, id := range linkedIids {
			iidStrs = append(iidStrs, strconv.FormatInt(id, 10))
		}
		linked, lerr := client.IssuesByIIDs(ctx, integ.ProjectPath, iidStrs)
		if lerr != nil {
			return 0, 0, lerr
		}
		issues = mergeIssues(assigned, linked)
	}

	// closed_policy=period: don't import NEW closed issues older than the cutoff
	// (keeps the initial import bounded); already-linked ones keep syncing.
	if integ.ClosedPolicy == "period" && integ.ClosedAfter != nil {
		linkedSet := make(map[int64]bool, len(linkedIids))
		for _, id := range linkedIids {
			linkedSet[id] = true
		}
		kept := issues[:0]
		for _, is := range issues {
			if is.State == "closed" && is.UpdatedAt != nil && is.UpdatedAt.Before(*integ.ClosedAfter) && !linkedSet[is.IID] {
				continue
			}
			kept = append(kept, is)
		}
		issues = kept
	}

	// Refresh the assignable project-member roster (best-effort: a failure here must
	// not abort the issue sync). It rarely changes and its REST paging is costly, so
	// an incremental sync only refreshes it once an hour; a full sweep always does.
	if !incremental || integ.MembersSyncedAt == nil || time.Since(*integ.MembersSyncedAt) >= memberRosterTTL {
		h.syncProjectMembers(ctx, client, integ)
		_ = h.q.MarkGitlabMembersSynced(ctx, integ.ID)
	}

	rules := parseRules(integ.LabelRules)
	wsID := integ.WorkspaceID
	// Time-estimate sync is gated on the board's estimation unit being "time".
	estimateUnit := h.integrationEstimationUnit(ctx, integ)

	// Per-board column cache — a "board" rule can route a task onto a different
	// board (e.g. a Backlog board), so columns are resolved per target board.
	cache := map[uuid.UUID]*syncBoard{}
	loadBoard := func(bid uuid.UUID) (*syncBoard, error) {
		if bc, ok := cache[bid]; ok {
			return bc, nil
		}
		b, err := h.q.GetBoard(ctx, bid)
		if err != nil {
			return nil, err
		}
		cols, err := h.q.ListColumns(ctx, bid)
		if err != nil || len(cols) == 0 {
			return nil, fmt.Errorf("board has no columns")
		}
		byName := make(map[string]db.BoardColumn, len(cols))
		for _, c := range cols {
			byName[c.Name] = c
		}
		bc := &syncBoard{id: bid, cols: cols, byName: byName, doneID: doneColumnID(b)}
		cache[bid] = bc
		return bc, nil
	}
	if _, err := loadBoard(integ.BoardID); err != nil {
		return 0, 0, fmt.Errorf("integration board has no columns")
	}

	// resolveBoardCol picks the target board (a board rule may route elsewhere),
	// the column (status label, or the done column for a closed issue), and the
	// completion timestamp.
	resolveBoardCol := func(issue gitlab.Issue, res gitlab.Resolution) (*syncBoard, db.BoardColumn, *time.Time) {
		targetBoard := integ.BoardID
		if res.BoardID != "" {
			if bid, perr := uuid.Parse(res.BoardID); perr == nil {
				if ws, werr := h.q.WorkspaceIDForBoard(ctx, bid); werr == nil && ws == wsID {
					targetBoard = bid
				}
			}
		}
		bc, berr := loadBoard(targetBoard)
		if berr != nil {
			bc = cache[integ.BoardID] // fall back to the default board
		}
		col, found := bc.byName[res.ColumnName]
		if !found {
			col = bc.cols[0]
		}
		if issue.State == "closed" && bc.doneID != nil {
			for _, c := range bc.cols {
				if c.ID == *bc.doneID {
					col = c
					break
				}
			}
		}
		var completedAt *time.Time
		if bc.doneID != nil && col.ID == *bc.doneID {
			now := time.Now()
			completedAt = &now
		}
		return bc, col, completedAt
	}

	// Pre-pass: for grouped parents, fetch their GitLab children so they sync as
	// Tessera subtasks (and are not duplicated as top-level cards).
	claimed := map[string]bool{}
	childrenOf := map[string][]gitlab.Issue{}
	for _, issue := range issues {
		if !rules.Resolve(issue.Labels).Group {
			continue
		}
		iids, cerr := client.ChildIIDs(ctx, integ.ProjectPath, issue.IID)
		if cerr != nil || len(iids) == 0 {
			continue
		}
		strs := make([]string, 0, len(iids))
		for _, id := range iids {
			strs = append(strs, strconv.FormatInt(id, 10))
		}
		kids, kerr := client.IssuesByIIDs(ctx, integ.ProjectPath, strs)
		if kerr != nil {
			continue
		}
		childrenOf[issue.GlobalID] = kids
		for _, k := range kids {
			claimed[k.GlobalID] = true
		}
	}

	for _, issue := range issues {
		if claimed[issue.GlobalID] {
			continue // synced as a subtask under its parent
		}
		res := rules.Resolve(issue.Labels)
		bc, col, completedAt := resolveBoardCol(issue, res)
		dueDate := effectiveDue(issue, integ.DueSource)
		startDate := effectiveStart(issue, integ.StartSource)
		estimate := effectiveEstimate(issue, estimateUnit)
		taskID, wasCreated, ok := h.syncOneIssue(ctx, integ, issue, res, wsID, bc.id, col.ID, nil, completedAt, dueDate, startDate, estimate, actorID, col.Name, j)
		if !ok {
			continue
		}
		h.applyClosedPolicy(ctx, integ, issue, taskID)
		if wasCreated {
			created++
		} else {
			updated++
		}
		// Grouped parent → mirror its GitLab children as Tessera subtasks (under
		// the parent's board/column; completion follows the child's own state).
		if res.Group {
			for _, kid := range childrenOf[issue.GlobalID] {
				kres := rules.Resolve(kid.Labels)
				var kdone *time.Time
				if kid.State == "closed" {
					now := time.Now()
					kdone = &now
				}
				kdue := effectiveDue(kid, integ.DueSource)
				kstart := effectiveStart(kid, integ.StartSource)
				kest := effectiveEstimate(kid, estimateUnit)
				parentID := taskID
				if ktid, kc, kok := h.syncOneIssue(ctx, integ, kid, kres, wsID, bc.id, col.ID, &parentID, kdone, kdue, kstart, kest, actorID, col.Name, j); kok {
					h.applyClosedPolicy(ctx, integ, kid, ktid)
					if kc {
						created++
					} else {
						updated++
					}
				}
			}
		}
	}

	if incremental {
		_ = h.q.MarkGitlabSynced(ctx, integ.ID)
	} else {
		// A full sweep sees every still-existing issue, so a linked task whose issue is
		// absent was deleted in GitLab → archive it. Guarded on a non-empty fetch so a
		// transient empty response can't mass-archive the board.
		if len(issues) > 0 {
			seen := make(map[string]bool, len(issues))
			for _, is := range issues {
				seen[is.GlobalID] = true
			}
			for _, kids := range childrenOf {
				for _, k := range kids {
					seen[k.GlobalID] = true
				}
			}
			h.archiveDeletedLinks(ctx, integ, seen, actorID, j)
		} else {
			log.Printf("gitlab full-sync ws=%s: fetch returned no issues, skipping delete-detection", integ.WorkspaceID)
		}
		// A full sweep also advances last_full_synced_at so the auto worker knows when
		// the next forced full sweep is due.
		_ = h.q.MarkGitlabFullSynced(ctx, integ.ID)
	}
	// One board-level reload signal (no per-task toasts) so open boards refresh once.
	if created+updated > 0 {
		h.broadcast(wsID, "task.synced", gin.H{"board_id": integ.BoardID, "created": created, "updated": updated})
	}
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
	h.tick(jobGitlabSyncCron, "проверка интеграций к синхронизации")
	h.autoSyncDue(ctx) // catch up at startup, don't wait a tick
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobGitlabSyncCron, "проверка интеграций к синхронизации")
			h.autoSyncDue(ctx)
		}
	}
}

// autoSyncDue syncs every integration whose interval has elapsed (the query
// already filters for enabled + interval>0 + owner set + due).
func (h *API) autoSyncDue(ctx context.Context) {
	// A service token drives every due integration (no owner needed); otherwise only
	// those with an owner credential.
	_, _, hasService := h.serviceGitlabConn(ctx)
	integs, err := h.q.ListDueSyncIntegrations(ctx)
	if err != nil {
		return
	}
	for _, integ := range integs {
		var cred db.GitlabCredential
		actor := uuid.Nil
		if hasService {
			if integ.OwnerUserID != nil {
				actor = *integ.OwnerUserID
			}
		} else {
			if integ.OwnerUserID == nil {
				continue
			}
			c, cerr := h.q.GetGitlabCredential(ctx, *integ.OwnerUserID)
			if cerr != nil {
				continue // owner disconnected — skip until reconfigured
			}
			cred = c
			actor = *integ.OwnerUserID
		}
		// Register a cancelable run; Begin returns false if a manual (or previous auto)
		// run for this integration is already in flight — skip it this tick.
		syncCtx, cancel := context.WithCancel(ctx)
		handle, started := h.jobs.Begin(gitlabSyncKey(integ.ID), gitlabSyncName(integ), jobs.KindSync, integ.WorkspaceID.String(), cancel)
		if !started {
			cancel()
			continue
		}
		// Incremental by default; force a full sweep when one has never run or the
		// configured full-sync interval has elapsed (catches deletes/drift that an
		// incremental pull can't see). full_sync_interval_sec = 0 disables the forced
		// sweep (full then only runs on the very first sync).
		mode := "incremental"
		if fullSyncDue(integ) {
			mode = "full"
		}
		handle.SetOp(mode + " pull")
		created, updated, serr := h.runSync(syncCtx, integ, cred, actor, "auto", mode)
		handle.SetCounts(created, updated)
		handle.Finish(serr)
		cancel()
		if serr != nil {
			log.Printf("gitlab auto-sync ws=%s: %v", integ.WorkspaceID, serr)
			continue
		}
		if created+updated > 0 {
			log.Printf("gitlab auto-sync ws=%s: +%d new, ~%d updated", integ.WorkspaceID, created, updated)
		}
	}
}

// fullSyncDue reports whether the auto worker should force a full sweep for this
// integration instead of an incremental pull: when it has never had one, or its
// configured full-sync interval has elapsed. A zero interval disables the periodic
// sweep (a full pull then runs only on the first-ever sync, via LastFullSyncedAt).
func fullSyncDue(integ db.GitlabIntegration) bool {
	if integ.LastFullSyncedAt == nil {
		return true
	}
	if integ.FullSyncIntervalSec <= 0 {
		return false
	}
	return time.Since(*integ.LastFullSyncedAt) >= time.Duration(integ.FullSyncIntervalSec)*time.Second
}

// ── helpers ────────────────────────────────────────────────

// syncCreateTask creates a mirrored task at the end of its target column (or its
// parent's subtask list when parentID is set). The task has no Tessera creator
// (created_by stays null) — the GitLab author is on the link instead.
func (h *API) syncCreateTask(ctx context.Context, wsID, boardID, columnID uuid.UUID, parentID *uuid.UUID, issue gitlab.Issue, priority int32, completedAt, dueDate, startDate *time.Time, estimate *float64) (db.Task, error) {
	var pos float64
	if parentID != nil {
		subs, err := h.q.ListSubtasks(ctx, parentID)
		if err != nil {
			return db.Task{}, err
		}
		if len(subs) == 0 {
			pos = positionGap
		} else {
			last := subs[len(subs)-1].Position
			pos = positionBetween(&last, nil)
		}
	} else {
		maxPos, err := h.q.MaxTaskPositionInColumn(ctx, columnID)
		if err != nil {
			return db.Task{}, err
		}
		pos = positionBetween(&maxPos, nil)
	}
	num, err := h.q.NextWorkspaceTaskNumber(ctx, wsID)
	if err != nil {
		return db.Task{}, err
	}
	t, err := h.q.CreateTask(ctx, db.CreateTaskParams{
		BoardID: boardID, ColumnID: columnID, ParentID: parentID,
		Title: issue.Title, Description: issue.Description, Priority: priority,
		DueDate: dueDate, StartDate: startDate, Estimate: estimate, Position: pos, CreatedBy: nil, Number: &num,
	})
	if err != nil {
		return db.Task{}, err
	}
	if completedAt != nil {
		if done, derr := h.q.SyncUpdateTask(ctx, db.SyncUpdateTaskParams{
			ID: t.ID, Title: t.Title, Description: t.Description, Priority: t.Priority,
			ColumnID: t.ColumnID, CompletedAt: completedAt, BoardID: t.BoardID,
		}); derr == nil {
			t = done
		}
	}
	return t, nil
}

// sameUUIDPtr reports whether two optional UUIDs are equal.
func sameUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// syncOneIssue creates or updates the Tessera task mirroring one issue on the
// given board/column, optionally as a subtask of parentID, reconciling its link
// and meta. It records a create/update action in the sync journal j (only when
// something actually changed). Returns the task id, whether it was newly created,
// and ok=false on a per-issue failure (logged, caller continues).
func (h *API) syncOneIssue(ctx context.Context, integ db.GitlabIntegration, issue gitlab.Issue, res gitlab.Resolution, wsID, boardID, columnID uuid.UUID, parentID *uuid.UUID, completedAt, dueDate, startDate *time.Time, estimate *float64, actorID uuid.UUID, colName string, j *syncJournal) (uuid.UUID, bool, bool) {
	// Resolve GitLab-relative attachment links to signed proxy URLs.
	issue.Description = h.rewriteAssets(issue.Description, wsID)
	// Synced labels become tags scoped to the integration board's project.
	projectID, perr := h.q.ProjectIDForBoard(ctx, boardID)
	if perr != nil {
		log.Printf("gitlab sync: resolve project for board failed: %v", perr)
		return uuid.Nil, false, false
	}
	entity := "task"
	if parentID != nil {
		entity = "subtask"
	}
	iid := issue.IID
	link, lerr := h.q.GetGitlabLinkByGlobalID(ctx, db.GetGitlabLinkByGlobalIDParams{
		IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
	})
	switch {
	case errors.Is(lerr, pgx.ErrNoRows):
		t, cerr := h.syncCreateTask(ctx, wsID, boardID, columnID, parentID, issue, res.Priority, completedAt, dueDate, startDate, estimate)
		if cerr != nil {
			log.Printf("gitlab sync: create task for issue !%d failed: %v", issue.IID, cerr)
			return uuid.Nil, false, false
		}
		if _, cerr := h.q.CreateGitlabLink(ctx, db.CreateGitlabLinkParams{
			TaskID: t.ID, IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
			GlIid: issue.IID, GlProjectPath: integ.ProjectPath, GlWebUrl: issue.WebURL,
			GlUpdatedAt: issue.UpdatedAt,
			TitleHash:   hashStr(issue.Title), DescHash: hashStr(issue.Description),
			LabelsHash: hashStr(labelsKey(issue.Labels)),
			GlAuthor:   issue.AuthorLogin, GlAuthorName: issue.AuthorName,
			GlAuthorAvatarUrl: h.avatarProxyURL(wsID, issue.AuthorAvatar),
			GlLastState:       issue.State,
		}); cerr != nil {
			log.Printf("gitlab sync: link issue !%d failed: %v", issue.IID, cerr)
			return uuid.Nil, false, false
		}
		_ = h.q.SetGitlabLinkSnapshot(ctx, db.SetGitlabLinkSnapshotParams{TaskID: t.ID, GlSnapshot: buildGlSnapshot(issue, parseRules(integ.LabelRules))})
		h.reconcileTaskMilestone(ctx, integ, projectID, t.ID, issue, false)
		meta := h.reconcileTaskMeta(ctx, t.ID, wsID, projectID, issue, res.Tags)
		h.logEventActor(ctx, t.ID, actorID, "synced", map[string]any{"source": "gitlab", "iid": issue.IID, "url": issue.WebURL})
		// No per-task broadcast during sync: a full import would flood every watcher
		// with thousands of "created" activity toasts. runSync emits one board-level
		// "task.synced" reload event when it finishes instead.

		after := map[string]any{"title": issue.Title, "column": colName, "priority": res.Priority, "completed": completedAt != nil}
		if dueDate != nil {
			after["due"] = dueDate
		}
		if startDate != nil {
			after["start"] = startDate
		}
		detail := map[string]any{"after": after, "url": issue.WebURL}
		meta.into(detail)
		j.add(journalAction{
			Direction: "pull", EntityType: entity, Op: "create", TaskID: &t.ID, GlIid: &iid,
			Summary: journalNoun(entity, true) + " #" + strconv.FormatInt(iid, 10) + " «" + issue.Title + "»",
			Detail:  detail,
		})
		return t.ID, true, true
	case lerr != nil:
		log.Printf("gitlab sync: lookup link for issue !%d failed: %v", issue.IID, lerr)
		return uuid.Nil, false, false
	default:
		// Snapshot the pre-update task so the journal can diff what changed.
		old, _ := h.q.GetTask(ctx, link.TaskID)
		// Fields with an open write-back conflict are frozen: don't overwrite the
		// user's pending value from GitLab until they resolve it. title/description
		// are written in the bulk update below, so resolve the freeze up here.
		frozen := h.conflictFrozenKinds(ctx, link.TaskID)
		title, desc := issue.Title, issue.Description
		if frozen["title_desc"] {
			title, desc = old.Title, old.Description
		}
		prio := res.Priority
		if frozen["priority"] {
			prio = old.Priority
		}
		col, comp := columnID, completedAt
		if frozen["state"] {
			col, comp = old.ColumnID, old.CompletedAt
		}
		t, uerr := h.q.SyncUpdateTask(ctx, db.SyncUpdateTaskParams{
			ID: link.TaskID, Title: title, Description: desc,
			Priority: prio, ColumnID: col, CompletedAt: comp, BoardID: boardID,
		})
		if uerr != nil {
			log.Printf("gitlab sync: update task for issue !%d failed: %v", issue.IID, uerr)
			return uuid.Nil, false, false
		}
		// Re-parent if the grouping changed (e.g. an issue became a child, or a
		// child was detached). SetTaskParent also fixes board/column.
		if !sameUUIDPtr(t.ParentID, parentID) {
			if reparented, perr := h.q.SetTaskParent(ctx, db.SetTaskParentParams{
				ID: link.TaskID, ParentID: parentID, BoardID: boardID, ColumnID: columnID,
			}); perr == nil {
				t = reparented
			}
		}
		if _, uerr := h.q.UpdateGitlabLink(ctx, db.UpdateGitlabLinkParams{
			TaskID: link.TaskID, GlIid: issue.IID, GlWebUrl: issue.WebURL,
			GlUpdatedAt: issue.UpdatedAt,
			TitleHash:   hashStr(issue.Title), DescHash: hashStr(issue.Description),
			LabelsHash: hashStr(labelsKey(issue.Labels)),
			GlAuthor:   issue.AuthorLogin, GlAuthorName: issue.AuthorName,
			GlAuthorAvatarUrl: h.avatarProxyURL(wsID, issue.AuthorAvatar),
			GlLastState:       issue.State,
		}); uerr != nil {
			log.Printf("gitlab sync: update link for issue !%d failed: %v", issue.IID, uerr)
			return uuid.Nil, false, false
		}
		_ = h.q.SetGitlabLinkSnapshot(ctx, db.SetGitlabLinkSnapshotParams{TaskID: link.TaskID, GlSnapshot: buildGlSnapshot(issue, parseRules(integ.LabelRules))})
		h.reconcileTaskMilestone(ctx, integ, projectID, link.TaskID, issue, link.MilestoneOverridden)
		// Sync the due date only when GitLab has one and the user hasn't overridden.
		dueApplied := false
		if !link.DueOverridden && !frozen["due"] && dueDate != nil {
			_ = h.q.UpdateTaskDueDate(ctx, db.UpdateTaskDueDateParams{ID: link.TaskID, DueDate: dueDate})
			t.DueDate = dueDate
			dueApplied = true
		}
		// Likewise the start date: synced unless GitLab gives none or the user overrode.
		startApplied := false
		if !link.StartOverridden && startDate != nil {
			_ = h.q.UpdateTaskStartDate(ctx, db.UpdateTaskStartDateParams{ID: link.TaskID, StartDate: startDate})
			t.StartDate = startDate
			startApplied = true
		}
		// Time estimate (only when the board's unit is time → estimate != nil here):
		// synced unless the user overrode it.
		if !link.EstimateOverridden && !frozen["estimate"] && estimate != nil {
			_ = h.q.UpdateTaskEstimate(ctx, db.UpdateTaskEstimateParams{ID: link.TaskID, Estimate: estimate})
			t.Estimate = estimate
		}
		meta := h.reconcileTaskMeta(ctx, link.TaskID, wsID, projectID, issue, res.Tags)
		// No per-task broadcast during sync (see the create branch) — one reload
		// event is emitted at the end of runSync.

		// Diff the changed task fields for the journal (skip title/description when
		// frozen — they were intentionally not applied, so there's no real change).
		fields := map[string]any{}
		if !frozen["title_desc"] {
			if old.Title != issue.Title {
				fields["title"] = map[string]any{"before": old.Title, "after": issue.Title}
			}
			if old.Description != issue.Description {
				fields["description"] = map[string]any{"before": truncForJournal(old.Description), "after": truncForJournal(issue.Description)}
			}
		}
		if !frozen["priority"] && old.Priority != res.Priority {
			fields["priority"] = map[string]any{"before": old.Priority, "after": res.Priority}
		}
		if !frozen["state"] && old.ColumnID != columnID {
			before := ""
			if oc, oerr := h.q.GetColumn(ctx, old.ColumnID); oerr == nil {
				before = oc.Name
			}
			fields["column"] = map[string]any{"before": before, "after": colName}
		}
		if !frozen["state"] && (old.CompletedAt != nil) != (completedAt != nil) {
			fields["completed"] = map[string]any{"before": old.CompletedAt != nil, "after": completedAt != nil}
		}
		if dueApplied && !timePtrEq(old.DueDate, dueDate) {
			fields["due"] = map[string]any{"before": old.DueDate, "after": dueDate}
		}
		if startApplied && !timePtrEq(old.StartDate, startDate) {
			fields["start"] = map[string]any{"before": old.StartDate, "after": startDate}
		}
		// Only record an action when the sync actually changed something.
		if len(fields) == 0 && !meta.changed() {
			return link.TaskID, false, true
		}
		detail := map[string]any{"url": issue.WebURL}
		if len(fields) > 0 {
			detail["fields"] = fields
		}
		meta.into(detail)
		j.add(journalAction{
			Direction: "pull", EntityType: entity, Op: "update", TaskID: &link.TaskID, GlIid: &iid,
			Summary: journalNoun(entity, false) + " #" + strconv.FormatInt(iid, 10) + " «" + issue.Title + "»: " + meta.summarize(fields),
			Detail:  detail,
		})
		return link.TaskID, false, true
	}
}

// metaDelta captures what reconcileTaskMeta changed, for the journal detail.
type metaDelta struct {
	tagsAdded     []string
	tagsRemoved   []string
	commentsAdded int
	newComments   []string
	assignees     []string // current GitLab assignee display names (informational)
}

// changed reports whether the meta reconcile touched anything.
func (m metaDelta) changed() bool {
	return len(m.tagsAdded) > 0 || len(m.tagsRemoved) > 0 || m.commentsAdded > 0
}

// into folds the meta delta into a journal detail map.
func (m metaDelta) into(detail map[string]any) {
	if len(m.tagsAdded) > 0 || len(m.tagsRemoved) > 0 {
		tags := map[string]any{}
		if len(m.tagsAdded) > 0 {
			tags["added"] = m.tagsAdded
		}
		if len(m.tagsRemoved) > 0 {
			tags["removed"] = m.tagsRemoved
		}
		detail["tags"] = tags
	}
	if m.commentsAdded > 0 {
		detail["comments"] = map[string]any{"added": m.commentsAdded, "new": m.newComments}
	}
	if len(m.assignees) > 0 {
		detail["assignees"] = m.assignees
	}
}

// summarize builds the human-readable change list for an update action's summary.
func (m metaDelta) summarize(fields map[string]any) string {
	parts := make([]string, 0, 6)
	for _, key := range []string{"title", "description", "priority", "column", "completed", "due", "start"} {
		if _, ok := fields[key]; ok {
			parts = append(parts, fieldLabelRU[key])
		}
	}
	if len(m.tagsAdded) > 0 {
		parts = append(parts, fmt.Sprintf("+%d тег.", len(m.tagsAdded)))
	}
	if len(m.tagsRemoved) > 0 {
		parts = append(parts, fmt.Sprintf("−%d тег.", len(m.tagsRemoved)))
	}
	if m.commentsAdded > 0 {
		parts = append(parts, fmt.Sprintf("+%d комм.", m.commentsAdded))
	}
	if len(parts) == 0 {
		return "без изменений"
	}
	return strings.Join(parts, ", ")
}

var fieldLabelRU = map[string]string{
	"title": "заголовок", "description": "описание", "priority": "приоритет",
	"column": "колонка", "completed": "статус", "due": "срок", "start": "начало",
}

// journalNoun returns the Russian "(Created|Updated) (sub)task" phrase for a
// journal summary.
func journalNoun(entity string, created bool) string {
	noun := "задача"
	if entity == "subtask" {
		noun = "подзадача"
	}
	if created {
		return "Создана " + noun
	}
	return "Обновлена " + noun
}

// reconcileTaskMeta applies a synced issue's tags, assignees and comments to its
// Tessera task. Each is "mixed": the sync owns the gitlab-sourced set (added,
// refreshed and pruned to match GitLab) and never touches what the user added
// manually. Returns what changed, for the sync journal.
func (h *API) reconcileTaskMeta(ctx context.Context, taskID, wsID, projectID uuid.UUID, issue gitlab.Issue, tags []gitlab.Tag) metaDelta {
	added, removed := h.reconcileTags(ctx, taskID, wsID, projectID, tags)
	h.reconcileAssignees(ctx, wsID, taskID, issue.Assignees)
	commentsAdded, newBodies := h.syncComments(ctx, taskID, wsID, issue.Notes)
	names := make([]string, 0, len(issue.Assignees))
	for _, p := range issue.Assignees {
		if p.Name != "" {
			names = append(names, p.Name)
		} else {
			names = append(names, p.Login)
		}
	}
	return metaDelta{tagsAdded: added, tagsRemoved: removed, commentsAdded: commentsAdded, newComments: newBodies, assignees: names}
}

// reconcileTags ensures each resolved tag exists (with the GitLab colour or a
// stable auto-colour), attaches it as gitlab-sourced, and prunes gitlab-sourced
// tags GitLab no longer has. Manual ('user') tags are left untouched. Returns the
// names of tags newly attached and pruned, for the journal.
func (h *API) reconcileTags(ctx context.Context, taskID, wsID, projectID uuid.UUID, tags []gitlab.Tag) (added, removed []string) {
	ids := make([]uuid.UUID, 0, len(tags))
	for _, t := range tags {
		color := t.Color
		if color == "" {
			color = autoTagColor(t.Name)
		}
		tag, err := h.q.EnsureTag(ctx, db.EnsureTagParams{WorkspaceID: wsID, ProjectID: projectID, Name: t.Name, Color: color})
		if err != nil {
			continue
		}
		if n, _ := h.q.AddTaskTagSourced(ctx, db.AddTaskTagSourcedParams{TaskID: taskID, TagID: tag.ID, Source: "gitlab"}); n > 0 {
			added = append(added, t.Name)
		}
		ids = append(ids, tag.ID)
	}
	removed, _ = h.q.DeleteStaleGitlabTaskTags(ctx, db.DeleteStaleGitlabTaskTagsParams{TaskID: taskID, Column2: ids})
	return added, removed
}

// syncProjectMembers refreshes the integration's assignable GitLab member roster
// (upsert each + prune those no longer present). Best-effort — logged, never fatal.
func (h *API) syncProjectMembers(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration) {
	members, err := client.ProjectMembers(ctx, integ.ProjectPath)
	if err != nil {
		log.Printf("gitlab sync: list members for %s: %v", integ.ProjectPath, err)
		return
	}
	keep := make([]int64, 0, len(members))
	for _, m := range members {
		_ = h.q.UpsertGitlabProjectMember(ctx, db.UpsertGitlabProjectMemberParams{
			IntegrationID: integ.ID, GlUserID: m.ID, GlUsername: m.Username, GlName: m.Name,
			GlAvatarUrl: h.avatarProxyURL(integ.WorkspaceID, m.AvatarURL), AccessLevel: int32(m.AccessLevel),
		})
		keep = append(keep, m.ID)
	}
	_ = h.q.DeleteStaleGitlabProjectMembers(ctx, db.DeleteStaleGitlabProjectMembersParams{IntegrationID: integ.ID, Column2: keep})
}

// reconcileAssignees rebuilds the GitLab-sourced assignee set: a GitLab assignee
// whose username maps to a Tessera user becomes a real (gitlab-sourced) assignee;
// the rest are stored as external display-only assignees. Manual ('user')
// assignees are left untouched.
// applyClosedPolicy archives (or un-archives) a synced task per the integration's
// closed_policy. For archive_closed_sprints a closed issue in a closed milestone is
// moved to the archive; anything else (open issue, or closed issue in an open/no
// milestone) stays on the board. Idempotent; a no-op for the other policies.
func (h *API) applyClosedPolicy(ctx context.Context, integ db.GitlabIntegration, issue gitlab.Issue, taskID uuid.UUID) {
	if integ.ClosedPolicy != "archive_closed_sprints" {
		return
	}
	if issue.State == "closed" && issue.MilestoneState == "closed" {
		_ = h.q.ArchiveTaskIfActive(ctx, taskID)
	} else {
		_ = h.q.RestoreTaskIfArchived(ctx, taskID)
	}
}

func (h *API) reconcileAssignees(ctx context.Context, wsID, taskID uuid.UUID, people []gitlab.Person) {
	_ = h.q.DeleteGitlabSourcedAssignees(ctx, taskID) // only the sync-made set is rebuilt
	tesseraIDs := make([]uuid.UUID, 0, len(people))
	for _, p := range people {
		// Resolve the GitLab assignee to a Tessera user. Prefer the "Login with
		// GitLab" OAuth identity (the canonical, self-owned link) over a legacy
		// connected-PAT credential — so a person's tasks land on the account they
		// actually sign in with, not on whoever configured the integration token.
		uid, err := h.q.GetUserIDByOAuthUsername(ctx, p.Login)
		if err != nil {
			uid, err = h.q.GetUserIDByGitlabUsername(ctx, p.Login)
		}
		if err == nil {
			_ = h.q.AddTaskAssigneeSourced(ctx, db.AddTaskAssigneeSourcedParams{TaskID: taskID, UserID: uid, Source: "gitlab"})
			tesseraIDs = append(tesseraIDs, uid)
		} else {
			_ = h.q.UpsertGitlabSourcedAssignee(ctx, db.UpsertGitlabSourcedAssigneeParams{
				TaskID: taskID, GlUsername: p.Login, GlName: p.Name,
				GlAvatarUrl: h.avatarProxyURL(wsID, p.AvatarURL),
			})
		}
	}
	_ = h.q.DeleteStaleGitlabAssignees(ctx, db.DeleteStaleGitlabAssigneesParams{TaskID: taskID, Column2: tesseraIDs})
}

// syncComments upserts each GitLab note as a comment, idempotent by note id, with
// the GitLab author denormalised (it may not be a Tessera user). Returns the count
// and truncated bodies of newly inserted comments, for the journal.
func (h *API) syncComments(ctx context.Context, taskID, wsID uuid.UUID, notes []gitlab.Note) (added int, bodies []string) {
	for _, n := range notes {
		noteID := n.GlobalID
		// First, try to link this note back to the user's own comment that produced
		// it (posted from Tessera, gid not yet tagged by the async writeback worker).
		// This avoids re-importing it as a duplicate gitlab-sourced comment when a
		// pull races the push. Strip an optional Tessera marker footer so the stored
		// (unmarked) body still matches.
		claimBody := strings.TrimSuffix(n.Body, tesseraCommentMarker)
		if _, cerr := h.q.ClaimPushedUserComment(ctx, db.ClaimPushedUserCommentParams{
			TaskID: taskID, GlNoteID: &noteID, Body: claimBody,
		}); cerr == nil {
			continue // claimed our own pushed comment — nothing to insert
		}
		body := h.rewriteAssets(n.Body, wsID)
		inserted, err := h.q.UpsertGitlabComment(ctx, db.UpsertGitlabCommentParams{
			TaskID: taskID, Body: body, GlNoteID: &noteID,
			GlAuthorLogin: n.Author.Login, GlAuthorName: n.Author.Name,
			GlAuthorAvatarUrl: h.avatarProxyURL(wsID, n.Author.AvatarURL), CreatedAt: n.CreatedAt,
		})
		if err == nil && inserted {
			added++
			bodies = append(bodies, truncForJournal(body))
		}
	}
	return added, bodies
}

// effectiveEstimate resolves the estimate (canon minutes) the sync should apply:
// only when the board's estimation unit is "time" and GitLab has an estimate.
// GitLab's timeEstimate is in seconds; canon for the "time" unit is minutes.
func effectiveEstimate(issue gitlab.Issue, unit string) *float64 {
	if unit != "time" || issue.TimeEstimate <= 0 {
		return nil
	}
	m := float64(issue.TimeEstimate) / 60.0
	return &m
}

// integrationEstimationUnit resolves the estimation unit for an integration's
// board: project config → workspace config → built-in default ("time").
func (h *API) integrationEstimationUnit(ctx context.Context, integ db.GitlabIntegration) string {
	var proj, ws *json.RawMessage
	if pid, err := h.q.ProjectIDForBoard(ctx, integ.BoardID); err == nil {
		if p, perr := h.q.GetProject(ctx, pid); perr == nil {
			proj = p.Estimation
		}
	}
	if w, werr := h.q.GetWorkspace(ctx, integ.WorkspaceID); werr == nil {
		ws = w.Estimation
	}
	return estimationUnit(proj, ws)
}

// estimationUnit parses the unit from the two-level estimation config (project
// wins over workspace); empty/unset → "time" (the built-in default).
func estimationUnit(proj, ws *json.RawMessage) string {
	parse := func(r *json.RawMessage) string {
		if r == nil || len(*r) == 0 {
			return ""
		}
		var cfg struct {
			Unit string `json:"unit"`
		}
		if json.Unmarshal(*r, &cfg) == nil {
			return cfg.Unit
		}
		return ""
	}
	if u := parse(proj); u != "" {
		return u
	}
	if u := parse(ws); u != "" {
		return u
	}
	return "time"
}

// effectiveDue resolves the due date the sync should apply, per the
// integration's due_source: the issue's own due, the milestone End date, both
// (issue first), or none.
func effectiveDue(issue gitlab.Issue, source string) *time.Time {
	switch source {
	case "issue":
		return issue.DueDate
	case "milestone":
		return issue.MilestoneDue
	case "off":
		return nil
	default: // issue_milestone
		if issue.DueDate != nil {
			return issue.DueDate
		}
		return issue.MilestoneDue
	}
}

// effectiveStart resolves the start date the sync should apply, per the
// integration's start_source: the issue/task creation date, the milestone Start
// date, or none. GitLab issues carry no own start date, so "created" (the default)
// is the reliable source.
func effectiveStart(issue gitlab.Issue, source string) *time.Time {
	switch source {
	case "milestone":
		return issue.MilestoneStart
	case "off":
		return nil
	default: // created
		return issue.CreatedAt
	}
}

// mergeIssues concatenates two issue lists, deduped by global id (first wins).
func mergeIssues(a, b []gitlab.Issue) []gitlab.Issue {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]gitlab.Issue, 0, len(a)+len(b))
	for _, set := range [][]gitlab.Issue{a, b} {
		for _, is := range set {
			if seen[is.GlobalID] {
				continue
			}
			seen[is.GlobalID] = true
			out = append(out, is)
		}
	}
	return out
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
	IID             int64  `json:"iid"`
	WebURL          string `json:"web_url"`
	Author          string `json:"author"`
	AuthorName      string `json:"author_name"`
	AuthorAvatarURL string `json:"author_avatar_url"`
	ProjectPath     string `json:"project_path"`
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
		AuthorName: link.GlAuthorName, AuthorAvatarURL: link.GlAuthorAvatarUrl,
		ProjectPath: link.GlProjectPath,
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
	// Empty or legacy (pre-generic) config → fall back to defaults; the user
	// re-saves through the new rule editor.
	if len(r.Rules) == 0 {
		return gitlab.DefaultRules()
	}
	return r
}

// parseWriteback decodes the JSONB write-back config, defaulting to all-off (no
// write-back) when empty or invalid.
func parseWriteback(raw []byte) gitlab.Writeback {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return gitlab.DefaultWriteback()
	}
	var w gitlab.Writeback
	if err := json.Unmarshal(raw, &w); err != nil {
		return gitlab.DefaultWriteback()
	}
	return w
}

// hashStr is a content snapshot helper (sha256 hex) for the link's *_hash columns:
// the pull records them and the write-back loop-guard compares against them.
func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ListGitlabMembers returns the assignable GitLab project members for a workspace
// (synced from GitLab on each pull). Powers the integration-board assignee picker.
func (h *API) ListGitlabMembers(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListGitlabProjectMembersByWorkspace(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, m := range rows {
		// Prefer the OAuth-identity mapping, else a connected-PAT mapping. Non-nil
		// means this GitLab member already has a Tessera account (for UI dedup).
		mapped := m.TesseraUserID
		if mapped == nil {
			mapped = m.TesseraUserIDPat
		}
		out = append(out, gin.H{
			"gl_user_id": m.GlUserID, "gl_username": m.GlUsername,
			"gl_name": m.GlName, "gl_avatar_url": m.GlAvatarUrl,
			"tessera_user_id": mapped,
		})
	}
	c.JSON(http.StatusOK, out)
}

// PinTaskGitlabAssignee pins a GitLab project member (who may have no Tessera
// account) as an assignee of a task, surviving the sync rebuild (source='user').
func (h *API) PinTaskGitlabAssignee(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		GlUsername  string `json:"gl_username" binding:"required"`
		GlName      string `json:"gl_name"`
		GlAvatarURL string `json:"gl_avatar_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.PinGitlabAssignee(c, db.PinGitlabAssigneeParams{
		TaskID: id, GlUsername: req.GlUsername, GlName: req.GlName, GlAvatarUrl: req.GlAvatarURL,
	}); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.assigned", gin.H{"task_id": id, "gl_username": req.GlUsername})
	h.enqueueWriteback(c, id, middleware.CurrentUser(c), gitlab.TrigAssignees, map[string]any{})
	c.Status(http.StatusNoContent)
}

// RemoveTaskGitlabAssignee unpins a GitLab-member assignee from a task.
func (h *API) RemoveTaskGitlabAssignee(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	if err := h.q.RemoveGitlabAssignee(c, db.RemoveGitlabAssigneeParams{TaskID: id, GlUsername: username}); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "task.unassigned", gin.H{"task_id": id, "gl_username": username})
	h.enqueueWriteback(c, id, middleware.CurrentUser(c), gitlab.TrigAssignees, map[string]any{})
	c.Status(http.StatusNoContent)
}
