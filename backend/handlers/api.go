package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/realtime"
	"tessera/middleware"
)

// API holds the dependencies shared by all resource handlers.
type API struct {
	q         *db.Queries
	hub       *realtime.Hub
	uploadDir string
}

func NewAPI(q *db.Queries, hub *realtime.Hub, uploadDir string) *API {
	return &API{q: q, hub: hub, uploadDir: uploadDir}
}

// positionGap is the spacing used when appending to the end of a list.
const positionGap = 65536.0

// positionBetween computes a float position for an item dropped between two
// neighbours (classic kanban-style midpoint). nil prev = top of list, nil next = end.
func positionBetween(prev, next *float64) float64 {
	switch {
	case prev != nil && next != nil:
		return (*prev + *next) / 2
	case prev != nil:
		return *prev + positionGap
	case next != nil:
		return *next / 2
	default:
		return positionGap
	}
}

// requireMember authorizes the current user against a workspace. It writes a
// 403 and returns false when the user is not a member.
func (h *API) requireMember(c *gin.Context, workspaceID uuid.UUID) bool {
	_, err := h.q.GetMembership(c, db.GetMembershipParams{
		WorkspaceID: workspaceID,
		UserID:      middleware.CurrentUser(c),
	})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this workspace"})
		return false
	}
	return true
}

// broadcast fans a domain event out to connected clients scoped to a workspace.
func (h *API) broadcast(workspaceID uuid.UUID, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.hub.Broadcast(realtime.Event{Scope: workspaceID.String(), Type: eventType, Data: data})
}

// parseID reads a uuid path param, writing 400 on failure.
func parseID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return uuid.Nil, false
	}
	return id, true
}

// notFound reports whether err is pgx.ErrNoRows and, if so, writes 404.
func notFound(c *gin.Context, err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return true
	}
	return false
}

// fail writes a generic 500.
func fail(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
