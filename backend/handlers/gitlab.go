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
	DueSource       string       `json:"due_source"`
	LastSyncedAt    *time.Time   `json:"last_synced_at"`
	LabelRules      gitlab.Rules `json:"label_rules"`
}

// integrationView projects a stored integration row into its JSON view.
func integrationView(integ db.GitlabIntegration) gitlabIntegrationView {
	bid := integ.BoardID
	return gitlabIntegrationView{
		Configured: true, ProjectPath: integ.ProjectPath, BoardID: &bid,
		Enabled: integ.Enabled, SyncIntervalSec: integ.SyncIntervalSec,
		DueSource: integ.DueSource, LastSyncedAt: integ.LastSyncedAt,
		LabelRules: parseRules(integ.LabelRules),
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
		DueSource       string          `json:"due_source"`
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
	switch req.DueSource {
	case "issue", "milestone", "off", "issue_milestone":
	default:
		req.DueSource = "issue_milestone"
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
		DueSource:       req.DueSource,
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

// syncBoard caches a board's columns + done column during a sync run.
type syncBoard struct {
	id     uuid.UUID
	cols   []db.BoardColumn
	byName map[string]db.BoardColumn
	doneID *uuid.UUID
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
	// Issues currently assigned to me (discovers new) plus every issue we already
	// linked (keeps them fresh and reflects a reassignment away from me — the
	// task stays, the assignee changes). Merge, deduped by global id.
	assigned, err := client.AssignedIssues(ctx, integ.ProjectPath, cred.GlUsername)
	if err != nil {
		return 0, 0, err
	}
	linkedIids, _ := h.q.LinkedIidsForIntegration(ctx, integ.ID)
	iidStrs := make([]string, 0, len(linkedIids))
	for _, id := range linkedIids {
		iidStrs = append(iidStrs, strconv.FormatInt(id, 10))
	}
	linked, err := client.IssuesByIIDs(ctx, integ.ProjectPath, iidStrs)
	if err != nil {
		return 0, 0, err
	}
	issues := mergeIssues(assigned, linked)

	rules := parseRules(integ.LabelRules)
	wsID := integ.WorkspaceID

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
		bc := &syncBoard{id: bid, cols: cols, byName: byName, doneID: h.resolveDoneColumn(ctx, b)}
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
		taskID, wasCreated, ok := h.syncOneIssue(ctx, integ, issue, res, wsID, bc.id, col.ID, nil, completedAt, dueDate, actorID)
		if !ok {
			continue
		}
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
				parentID := taskID
				if _, kc, kok := h.syncOneIssue(ctx, integ, kid, kres, wsID, bc.id, col.ID, &parentID, kdone, kdue, actorID); kok {
					if kc {
						created++
					} else {
						updated++
					}
				}
			}
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

// syncCreateTask creates a mirrored task at the end of its target column (or its
// parent's subtask list when parentID is set). The task has no Tessera creator
// (created_by stays null) — the GitLab author is on the link instead.
func (h *API) syncCreateTask(ctx context.Context, wsID, boardID, columnID uuid.UUID, parentID *uuid.UUID, issue gitlab.Issue, priority int32, completedAt, dueDate *time.Time) (db.Task, error) {
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
		DueDate: dueDate, Position: pos, CreatedBy: nil, Number: &num,
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
// and meta. Returns the task id, whether it was newly created, and ok=false on a
// per-issue failure (logged, caller continues).
func (h *API) syncOneIssue(ctx context.Context, integ db.GitlabIntegration, issue gitlab.Issue, res gitlab.Resolution, wsID, boardID, columnID uuid.UUID, parentID *uuid.UUID, completedAt, dueDate *time.Time, actorID uuid.UUID) (uuid.UUID, bool, bool) {
	// Resolve GitLab-relative attachment links to signed proxy URLs.
	issue.Description = h.rewriteAssets(issue.Description, wsID)
	link, lerr := h.q.GetGitlabLinkByGlobalID(ctx, db.GetGitlabLinkByGlobalIDParams{
		IntegrationID: integ.ID, GlGlobalID: issue.GlobalID,
	})
	switch {
	case errors.Is(lerr, pgx.ErrNoRows):
		t, cerr := h.syncCreateTask(ctx, wsID, boardID, columnID, parentID, issue, res.Priority, completedAt, dueDate)
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
		}); cerr != nil {
			log.Printf("gitlab sync: link issue !%d failed: %v", issue.IID, cerr)
			return uuid.Nil, false, false
		}
		h.reconcileTaskMeta(ctx, t.ID, wsID, issue, res.Tags)
		h.logEventActor(ctx, t.ID, actorID, "synced", map[string]any{"source": "gitlab", "iid": issue.IID, "url": issue.WebURL})
		h.broadcast(wsID, "task.created", t)
		return t.ID, true, true
	case lerr != nil:
		log.Printf("gitlab sync: lookup link for issue !%d failed: %v", issue.IID, lerr)
		return uuid.Nil, false, false
	default:
		t, uerr := h.q.SyncUpdateTask(ctx, db.SyncUpdateTaskParams{
			ID: link.TaskID, Title: issue.Title, Description: issue.Description,
			Priority: res.Priority, ColumnID: columnID, CompletedAt: completedAt, BoardID: boardID,
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
		}); uerr != nil {
			log.Printf("gitlab sync: update link for issue !%d failed: %v", issue.IID, uerr)
			return uuid.Nil, false, false
		}
		// Sync the due date only when GitLab has one and the user hasn't overridden.
		if !link.DueOverridden && dueDate != nil {
			_ = h.q.UpdateTaskDueDate(ctx, db.UpdateTaskDueDateParams{ID: link.TaskID, DueDate: dueDate})
			t.DueDate = dueDate
		}
		h.reconcileTaskMeta(ctx, link.TaskID, wsID, issue, res.Tags)
		h.broadcast(wsID, "task.updated", t)
		return link.TaskID, false, true
	}
}

// reconcileTaskMeta applies a synced issue's tags, assignees and comments to its
// Tessera task. Each is "mixed": the sync owns the gitlab-sourced set (added,
// refreshed and pruned to match GitLab) and never touches what the user added
// manually.
func (h *API) reconcileTaskMeta(ctx context.Context, taskID, wsID uuid.UUID, issue gitlab.Issue, tags []gitlab.Tag) {
	h.reconcileTags(ctx, taskID, wsID, tags)
	h.reconcileAssignees(ctx, taskID, issue.Assignees)
	h.syncComments(ctx, taskID, wsID, issue.Notes)
}

// reconcileTags ensures each resolved tag exists (with the GitLab colour or a
// stable auto-colour), attaches it as gitlab-sourced, and prunes gitlab-sourced
// tags GitLab no longer has. Manual ('user') tags are left untouched.
func (h *API) reconcileTags(ctx context.Context, taskID, wsID uuid.UUID, tags []gitlab.Tag) {
	ids := make([]uuid.UUID, 0, len(tags))
	for _, t := range tags {
		color := t.Color
		if color == "" {
			color = autoTagColor(t.Name)
		}
		tag, err := h.q.EnsureTag(ctx, db.EnsureTagParams{WorkspaceID: wsID, Name: t.Name, Color: color})
		if err != nil {
			continue
		}
		_ = h.q.AddTaskTagSourced(ctx, db.AddTaskTagSourcedParams{TaskID: taskID, TagID: tag.ID, Source: "gitlab"})
		ids = append(ids, tag.ID)
	}
	_ = h.q.DeleteStaleGitlabTaskTags(ctx, db.DeleteStaleGitlabTaskTagsParams{TaskID: taskID, Column2: ids})
}

// reconcileAssignees rebuilds the GitLab-sourced assignee set: a GitLab assignee
// whose username maps to a Tessera user becomes a real (gitlab-sourced) assignee;
// the rest are stored as external display-only assignees. Manual ('user')
// assignees are left untouched.
func (h *API) reconcileAssignees(ctx context.Context, taskID uuid.UUID, people []gitlab.Person) {
	_ = h.q.DeleteTaskGitlabAssignees(ctx, taskID) // external set is fully rebuilt
	tesseraIDs := make([]uuid.UUID, 0, len(people))
	for _, p := range people {
		if uid, err := h.q.GetUserIDByGitlabUsername(ctx, p.Login); err == nil {
			_ = h.q.AddTaskAssigneeSourced(ctx, db.AddTaskAssigneeSourcedParams{TaskID: taskID, UserID: uid, Source: "gitlab"})
			tesseraIDs = append(tesseraIDs, uid)
		} else {
			_ = h.q.AddTaskGitlabAssignee(ctx, db.AddTaskGitlabAssigneeParams{TaskID: taskID, GlUsername: p.Login, GlName: p.Name})
		}
	}
	_ = h.q.DeleteStaleGitlabAssignees(ctx, db.DeleteStaleGitlabAssigneesParams{TaskID: taskID, Column2: tesseraIDs})
}

// syncComments upserts each GitLab note as a comment, idempotent by note id, with
// the GitLab author denormalised (it may not be a Tessera user).
func (h *API) syncComments(ctx context.Context, taskID, wsID uuid.UUID, notes []gitlab.Note) {
	for _, n := range notes {
		noteID := n.GlobalID
		_ = h.q.UpsertGitlabComment(ctx, db.UpsertGitlabCommentParams{
			TaskID: taskID, Body: h.rewriteAssets(n.Body, wsID), GlNoteID: &noteID,
			GlAuthorLogin: n.Author.Login, GlAuthorName: n.Author.Name, CreatedAt: n.CreatedAt,
		})
	}
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
	// Empty or legacy (pre-generic) config → fall back to defaults; the user
	// re-saves through the new rule editor.
	if len(r.Rules) == 0 {
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
