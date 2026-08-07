package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/quickact"
)

// Limits on the custom command dictionary. They exist to keep the autocomplete
// popup usable and the payload small, not because storage cares.
const (
	maxWorkspaceCommands = 50
	maxCommandKeyLen     = 32
	maxCommandDescLen    = 200
)

// commandKeyRe is the storage form of a custom command key: no leading slash,
// lowercase. Mirror it in the frontend (utils/commands.js).
var commandKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// commandView is one custom dictionary entry as clients see it.
type commandView struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

func commandViews(rows []db.WorkspaceCommand) []commandView {
	out := make([]commandView, 0, len(rows))
	for _, r := range rows {
		out = append(out, commandView{Key: r.Key, Description: r.Description})
	}
	return out
}

// ListWorkspaceCommands returns the command registry for the editor popup: the
// built-in quick actions (from quickact.Registry, so adding a command in Go is
// enough for clients to learn about it) plus the workspace's custom dictionary.
//
// can_manage rides along because the frontend has nowhere else to learn its own
// workspace role — GET /workspaces does not return it — and gating the settings
// entry behind a second /members request would be silly.
func (h *API) ListWorkspaceCommands(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListWorkspaceCommands(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"builtin":    quickact.Registry,
		"custom":     commandViews(rows),
		"can_manage": h.isManager(c, wsID),
	})
}

// SetWorkspaceCommands replaces the workspace's whole custom dictionary. The
// payload is the complete desired state (like SetTagPrefixes): anything absent
// is removed, and array order becomes popup order.
//
// Unlike tag prefixes, an invalid key is a 400 rather than a silent skip — the
// user typed it by hand in the dictionary editor and deserves to know why it
// did not stick.
func (h *API) SetWorkspaceCommands(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireManager(c, wsID) {
		return
	}
	var req struct {
		Commands []commandView `json:"commands"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Commands) > maxWorkspaceCommands {
		c.JSON(http.StatusBadRequest, gin.H{"error": "не больше 50 команд"})
		return
	}

	// Validate everything before touching storage: a half-applied dictionary
	// would be worse than a rejected one.
	type entry struct{ key, description string }
	entries := make([]entry, 0, len(req.Commands))
	seen := make(map[string]bool, len(req.Commands))
	for _, cmd := range req.Commands {
		key := quickact.CanonKey(cmd.Key)
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "пустой ключ команды"})
			return
		}
		if len(key) > maxCommandKeyLen || !commandKeyRe.MatchString(key) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "недопустимый ключ команды: " + cmd.Key + " (латиница, цифры, _ и -)",
			})
			return
		}
		if quickact.IsBuiltin(key) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": key + " — встроенная команда, выберите другой ключ",
			})
			return
		}
		desc := strings.TrimSpace(cmd.Description)
		if len([]rune(desc)) > maxCommandDescLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "описание команды " + key + " длиннее 200 символов"})
			return
		}
		if seen[key] {
			continue // duplicate after canonicalisation — first wins
		}
		seen[key] = true
		entries = append(entries, entry{key: key, description: desc})
	}

	if err := h.q.DeleteWorkspaceCommands(c, wsID); err != nil {
		fail(c)
		return
	}
	out := make([]db.WorkspaceCommand, 0, len(entries))
	for i, e := range entries {
		row, err := h.q.UpsertWorkspaceCommand(c, db.UpsertWorkspaceCommandParams{
			WorkspaceID: wsID, Key: e.key, Description: e.description, Position: int32(i),
		})
		if err != nil {
			fail(c)
			return
		}
		out = append(out, row)
	}
	views := commandViews(out)
	h.broadcast(wsID, "workspace_commands.updated", gin.H{"commands": views})
	c.JSON(http.StatusOK, views)
}

// isManager reports the caller's ability to edit workspace settings without
// writing a 403 — requireManager is the gate, this is the flag.
func (h *API) isManager(c *gin.Context, wsID uuid.UUID) bool {
	switch h.memberRole(c, wsID) {
	case "owner", "admin":
		return true
	}
	return false
}

// customCommandKeys returns the workspace's custom command keys for the parser.
// A storage error is not fatal: the dictionary only decides whether a "/word"
// line is *reported* as a recognised custom command, never whether it executes,
// so an empty list degrades the preview and nothing else.
func (h *API) customCommandKeys(c *gin.Context, wsID uuid.UUID) []string {
	rows, err := h.q.ListWorkspaceCommands(c, wsID)
	if err != nil {
		return nil
	}
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	return keys
}
