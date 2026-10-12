package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// ── Команды-макросы палитры ⌘K и шаблоны задач (#3069) ───────
//
// A macro is a named action the palette runs by its trigger (>bug) or a key:
// create a task (from a template, with tags, priority, «me», the current stage),
// open a link, create a document from a template, run a CI pipeline. Shared
// macros (owner_id NULL) are the workspace's and are edited by its owner and
// admins; personal ones are their owner's alone and invisible to everybody
// else. The server stores and checks; running is the client's — every action is
// a call to an endpoint that exists anyway, so its invariants hold. The «/»
// dictionary of comment commands (workspace_commands, 0050) is a separate thing.
//
// A task template is the workspace's ready-made task: a title, a Markdown
// description (checklists included), tags by name (matched to the project's tags
// when a task is made from it), a priority and subtasks.

const (
	maxMacros          = 100
	maxMacroParamBytes = 4096
	maxTemplates       = 100
)

var (
	macroActions = map[string]bool{
		"create_task": true, "open_link": true, "create_document": true, "run_pipeline": true,
	}
	// The palette call, stored without the «>»: lowercase letters (Latin or
	// Cyrillic), digits, - and _.
	macroTriggerRe = regexp.MustCompile(`^[a-z0-9а-яё][a-z0-9а-яё_-]{0,23}$`)
)

type macroView struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Trigger   string          `json:"trigger"`
	Action    string          `json:"action"`
	Params    json.RawMessage `json:"params"`
	Keys      string          `json:"keys"`
	Category  string          `json:"category"`
	Enabled   bool            `json:"enabled"`
	Shared    bool            `json:"shared"`
	Mine      bool            `json:"mine"`
	CanEdit   bool            `json:"can_edit"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func (h *API) macroViewOf(m db.WorkspaceMacro, me uuid.UUID, manager bool) macroView {
	params := json.RawMessage(m.Params)
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	mine := m.OwnerID != nil && *m.OwnerID == me
	return macroView{
		ID: m.ID, Name: m.Name, Trigger: m.Trigger, Action: m.Action, Params: params,
		Keys: m.Keys, Category: m.Category, Enabled: m.Enabled,
		Shared: m.OwnerID == nil, Mine: mine, CanEdit: mine || (m.OwnerID == nil && manager),
		UpdatedAt: m.UpdatedAt,
	}
}

type macroInput struct {
	Name     string          `json:"name"`
	Trigger  string          `json:"trigger"`
	Action   string          `json:"action"`
	Params   json.RawMessage `json:"params"`
	Keys     string          `json:"keys"`
	Category string          `json:"category"`
	Enabled  *bool           `json:"enabled"`
	Shared   bool            `json:"shared"`
}

// check normalizes the input and returns the refusal, if any.
func (in *macroInput) check() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Trigger = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Trigger), ">"))
	in.Category = strings.TrimSpace(in.Category)
	in.Keys = strings.TrimSpace(in.Keys)
	switch {
	case in.Name == "" || len([]rune(in.Name)) > 80:
		return "name: 1–80 characters"
	case !macroTriggerRe.MatchString(in.Trigger):
		return "trigger: letters, digits, - and _, up to 24"
	case !macroActions[in.Action]:
		return "unknown action"
	case len(in.Keys) > 32 || len([]rune(in.Category)) > 40:
		return "keys or category too long"
	}
	if len(in.Params) == 0 || string(in.Params) == "null" {
		in.Params = json.RawMessage(`{}`)
	}
	if len(in.Params) > maxMacroParamBytes {
		return "params too large"
	}
	var params map[string]any
	if json.Unmarshal(in.Params, &params) != nil {
		return "params must be an object"
	}
	if in.Action == "open_link" {
		raw, _ := params["url"].(string)
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "open_link needs an http(s) url"
		}
	}
	return ""
}

// ListMacros — GET /workspaces/:id/macros: the shared macros and the caller's own.
func (h *API) ListMacros(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	me := middleware.CurrentUser(c)
	rows, err := h.q.ListMacros(c, db.ListMacrosParams{WorkspaceID: wsID, Viewer: me})
	if err != nil {
		fail(c, err)
		return
	}
	manager := h.isManager(c, wsID)
	out := make([]macroView, 0, len(rows))
	for _, m := range rows {
		out = append(out, h.macroViewOf(m, me, manager))
	}
	c.JSON(http.StatusOK, gin.H{"macros": out, "can_manage": manager})
}

// triggerTaken: another macro the caller sees already answers to this trigger.
func (h *API) triggerTaken(c *gin.Context, wsID uuid.UUID, trigger string, except uuid.UUID) (bool, error) {
	rows, err := h.q.ListMacros(c, db.ListMacrosParams{WorkspaceID: wsID, Viewer: middleware.CurrentUser(c)})
	if err != nil {
		return false, err
	}
	for _, m := range rows {
		if m.ID != except && m.Trigger == trigger {
			return true, nil
		}
	}
	return false, nil
}

// CreateMacro — POST /workspaces/:id/macros. A shared one needs owner/admin.
func (h *API) CreateMacro(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var in macroInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if msg := in.check(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	manager := h.isManager(c, wsID)
	if in.Shared && !manager {
		c.JSON(http.StatusForbidden, gin.H{"error": "shared commands are edited by the owner and admins"})
		return
	}
	me := middleware.CurrentUser(c)
	existing, err := h.q.ListMacros(c, db.ListMacrosParams{WorkspaceID: wsID, Viewer: me})
	if err != nil {
		fail(c, err)
		return
	}
	if len(existing) >= maxMacros {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many commands"})
		return
	}
	if taken, err := h.triggerTaken(c, wsID, in.Trigger, uuid.Nil); err != nil {
		fail(c, err)
		return
	} else if taken {
		c.JSON(http.StatusConflict, gin.H{"error": "trigger taken", "trigger": in.Trigger})
		return
	}
	var owner *uuid.UUID
	if !in.Shared {
		owner = &me
	}
	enabled := in.Enabled == nil || *in.Enabled
	m, err := h.q.CreateMacro(c, db.CreateMacroParams{
		WorkspaceID: wsID, OwnerID: owner, Name: in.Name, Trigger: in.Trigger, Action: in.Action,
		Params: in.Params, Keys: in.Keys, Category: in.Category, Enabled: enabled, CreatedBy: &me,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "macros.changed", gin.H{})
	c.JSON(http.StatusCreated, h.macroViewOf(m, me, manager))
}

// loadMacro finds a macro the caller may edit; writes the refusal itself.
func (h *API) loadMacro(c *gin.Context) (db.WorkspaceMacro, bool, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.WorkspaceMacro{}, false, false
	}
	m, err := h.q.GetMacro(c, id)
	if notFound(c, err) {
		return db.WorkspaceMacro{}, false, false
	}
	if err != nil {
		fail(c, err)
		return db.WorkspaceMacro{}, false, false
	}
	me := middleware.CurrentUser(c)
	// Someone else's personal macro does not exist for the caller.
	if m.OwnerID != nil && *m.OwnerID != me {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return db.WorkspaceMacro{}, false, false
	}
	if !h.requireMember(c, m.WorkspaceID) {
		return db.WorkspaceMacro{}, false, false
	}
	manager := h.isManager(c, m.WorkspaceID)
	if m.OwnerID == nil && !manager {
		c.JSON(http.StatusForbidden, gin.H{"error": "shared commands are edited by the owner and admins"})
		return db.WorkspaceMacro{}, false, false
	}
	return m, manager, true
}

// UpdateMacro — PATCH /macros/:id with the whole command.
func (h *API) UpdateMacro(c *gin.Context) {
	m, manager, ok := h.loadMacro(c)
	if !ok {
		return
	}
	var in macroInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if msg := in.check(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if in.Shared && !manager {
		c.JSON(http.StatusForbidden, gin.H{"error": "shared commands are edited by the owner and admins"})
		return
	}
	if taken, err := h.triggerTaken(c, m.WorkspaceID, in.Trigger, m.ID); err != nil {
		fail(c, err)
		return
	} else if taken {
		c.JSON(http.StatusConflict, gin.H{"error": "trigger taken", "trigger": in.Trigger})
		return
	}
	me := middleware.CurrentUser(c)
	var owner *uuid.UUID
	if !in.Shared {
		owner = &me
	}
	enabled := in.Enabled == nil || *in.Enabled
	upd, err := h.q.UpdateMacro(c, db.UpdateMacroParams{
		ID: m.ID, OwnerID: owner, Name: in.Name, Trigger: in.Trigger, Action: in.Action,
		Params: in.Params, Keys: in.Keys, Category: in.Category, Enabled: enabled,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, m.WorkspaceID, "macros.changed", gin.H{})
	c.JSON(http.StatusOK, h.macroViewOf(upd, me, manager))
}

// DeleteMacro — DELETE /macros/:id.
func (h *API) DeleteMacro(c *gin.Context) {
	m, _, ok := h.loadMacro(c)
	if !ok {
		return
	}
	if err := h.q.DeleteMacro(c, m.ID); err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, m.WorkspaceID, "macros.changed", gin.H{})
	c.Status(http.StatusNoContent)
}

// ── task templates ──

type templateInput struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Priority    int32    `json:"priority"`
	Subtasks    []string `json:"subtasks"`
}

func cleanList(in []string, maxN, maxLen int) ([]string, bool) {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len([]rune(s)) > maxLen {
			return nil, false
		}
		out = append(out, s)
	}
	return out, len(out) <= maxN
}

func (in *templateInput) check() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Title = strings.TrimSpace(in.Title)
	var ok bool
	switch {
	case in.Name == "" || len([]rune(in.Name)) > 80:
		return "name: 1–80 characters"
	case len([]rune(in.Title)) > 300 || len(in.Description) > 20000:
		return "title or description too long"
	case in.Priority < 0 || in.Priority > 4:
		return "priority: 0–4"
	}
	if in.Tags, ok = cleanList(in.Tags, 20, 64); !ok {
		return "tags: up to 20, 64 characters each"
	}
	if in.Subtasks, ok = cleanList(in.Subtasks, 50, 300); !ok {
		return "subtasks: up to 50, 300 characters each"
	}
	return ""
}

// ListTaskTemplates — GET /workspaces/:id/task-templates.
func (h *API) ListTaskTemplates(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListTaskTemplates(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// CreateTaskTemplate — POST /workspaces/:id/task-templates (any member).
func (h *API) CreateTaskTemplate(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var in templateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if msg := in.check(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if rows, err := h.q.ListTaskTemplates(c, wsID); err == nil && len(rows) >= maxTemplates {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many templates"})
		return
	}
	me := middleware.CurrentUser(c)
	tpl, err := h.q.CreateTaskTemplate(c, db.CreateTaskTemplateParams{
		WorkspaceID: wsID, Name: in.Name, Title: in.Title, Description: in.Description,
		Priority: in.Priority, CreatedBy: &me, Tags: in.Tags, Subtasks: in.Subtasks,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "task_templates.changed", gin.H{})
	c.JSON(http.StatusCreated, tpl)
}

// loadTemplate: the template, editable by its author or the workspace's owner/admin.
func (h *API) loadTemplate(c *gin.Context) (db.TaskTemplate, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.TaskTemplate{}, false
	}
	tpl, err := h.q.GetTaskTemplate(c, id)
	if notFound(c, err) {
		return db.TaskTemplate{}, false
	}
	if err != nil {
		fail(c, err)
		return db.TaskTemplate{}, false
	}
	if !h.requireMember(c, tpl.WorkspaceID) {
		return db.TaskTemplate{}, false
	}
	mine := tpl.CreatedBy != nil && *tpl.CreatedBy == middleware.CurrentUser(c)
	if !mine && !h.isManager(c, tpl.WorkspaceID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the author or a workspace admin edits a template"})
		return db.TaskTemplate{}, false
	}
	return tpl, true
}

// UpdateTaskTemplate — PATCH /task-templates/:id with the whole template.
func (h *API) UpdateTaskTemplate(c *gin.Context) {
	tpl, ok := h.loadTemplate(c)
	if !ok {
		return
	}
	var in templateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if msg := in.check(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	upd, err := h.q.UpdateTaskTemplate(c, db.UpdateTaskTemplateParams{
		ID: tpl.ID, Name: in.Name, Title: in.Title, Description: in.Description,
		Priority: in.Priority, Tags: in.Tags, Subtasks: in.Subtasks,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, tpl.WorkspaceID, "task_templates.changed", gin.H{})
	c.JSON(http.StatusOK, upd)
}

// DeleteTaskTemplate — DELETE /task-templates/:id.
func (h *API) DeleteTaskTemplate(c *gin.Context) {
	tpl, ok := h.loadTemplate(c)
	if !ok {
		return
	}
	if err := h.q.DeleteTaskTemplate(c, tpl.ID); err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, tpl.WorkspaceID, "task_templates.changed", gin.H{})
	c.Status(http.StatusNoContent)
}
