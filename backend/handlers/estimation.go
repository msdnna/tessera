package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// estimationConfig is the two-level (workspace-default / project-override) unit
// setting for task estimates. It is provider-neutral and deliberately small;
// clients resolve project → workspace → built-in default and do all the input
// parsing / output formatting (the backend only validates and stores minutes).
type estimationConfig struct {
	Unit        string  `json:"unit"`                    // "time" | "points" | "custom"
	HoursPerDay float64 `json:"hours_per_day,omitempty"` // time only
	DaysPerWeek float64 `json:"days_per_week,omitempty"` // time only
	PointsScale string  `json:"points_scale,omitempty"`  // points only: fibonacci | tshirt | linear
	CustomLabel string  `json:"custom_label,omitempty"`  // custom only
}

// normalizeEstimationConfig validates a client-supplied config and returns its
// canonical JSON form, or nil to clear it (an absent / null / unit-less payload
// means "inherit": project falls back to workspace, workspace to the built-in
// default time/8h/5d). Returns an error string for an invalid unit.
func normalizeEstimationConfig(raw *json.RawMessage) (*json.RawMessage, string) {
	if raw == nil {
		return nil, ""
	}
	trimmed := strings.TrimSpace(string(*raw))
	if trimmed == "" || trimmed == "null" {
		return nil, ""
	}
	var cfg estimationConfig
	if err := json.Unmarshal(*raw, &cfg); err != nil {
		return nil, "invalid estimation config"
	}
	cfg.Unit = strings.TrimSpace(strings.ToLower(cfg.Unit))
	switch cfg.Unit {
	case "":
		return nil, "" // unit-less → clear (inherit)
	case "time":
		cfg.HoursPerDay = clampFloat(cfg.HoursPerDay, 1, 24, 8)
		cfg.DaysPerWeek = clampFloat(cfg.DaysPerWeek, 1, 7, 5)
		cfg.PointsScale, cfg.CustomLabel = "", ""
	case "points":
		cfg.PointsScale = strings.TrimSpace(strings.ToLower(cfg.PointsScale))
		switch cfg.PointsScale {
		case "fibonacci", "tshirt", "linear":
		default:
			cfg.PointsScale = "fibonacci"
		}
		cfg.HoursPerDay, cfg.DaysPerWeek, cfg.CustomLabel = 0, 0, ""
	case "custom":
		cfg.CustomLabel = strings.TrimSpace(cfg.CustomLabel)
		cfg.HoursPerDay, cfg.DaysPerWeek, cfg.PointsScale = 0, 0, ""
	default:
		return nil, "unit must be time, points or custom"
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, "invalid estimation config"
	}
	msg := json.RawMessage(out)
	return &msg, ""
}

func clampFloat(v, lo, hi, def float64) float64 {
	if v <= 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// SetWorkspaceEstimation sets the workspace-wide default estimation config. The
// body is the config object directly; `null` / `{}` clears it back to the
// built-in default.
func (h *API) SetWorkspaceEstimation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, id) {
		return
	}
	canon, errMsg := h.bindEstimation(c)
	if errMsg == "skip" {
		return
	}
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	ws, err := h.q.SetWorkspaceEstimation(c, db.SetWorkspaceEstimationParams{ID: id, Estimation: canon})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(id, "workspace.estimation", gin.H{"workspace_id": id, "estimation": canon})
	c.JSON(http.StatusOK, ws)
}

// SetProjectEstimation sets a project's estimation-config override (or clears it
// to inherit the workspace default).
func (h *API) SetProjectEstimation(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForProject(c, projectID)
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
	canon, errMsg := h.bindEstimation(c)
	if errMsg == "skip" {
		return
	}
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	p, err := h.q.SetProjectEstimation(c, db.SetProjectEstimationParams{ID: projectID, Estimation: canon})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "project.estimation", gin.H{"project_id": projectID, "estimation": canon})
	c.JSON(http.StatusOK, p)
}

// bindEstimation reads the request body as a raw estimation config and returns
// its canonical form. The sentinel "skip" means a response has already been
// written (bad JSON); a non-empty message is a validation error to report.
func (h *API) bindEstimation(c *gin.Context) (*json.RawMessage, string) {
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		// An empty body means "inherit" (the UI sends null when «Наследовать» is on;
		// axios omits a null body entirely → EOF here). Treat it as clear, not error.
		if errors.Is(err, io.EOF) {
			return nil, ""
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, "skip"
	}
	canon, errMsg := normalizeEstimationConfig(&raw)
	return canon, errMsg
}
