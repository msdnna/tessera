package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
)

// projectSlugAliasTTL is how long a project's former address keeps leading to
// it after the address changed (#3012). The v2 forms promise «30 дней» in so
// many words — change both together.
const projectSlugAliasTTL = 30 * 24 * time.Hour

// projectBySlug resolves a project address: the live slug first, then a former
// address that hasn't expired. aliased tells the caller the address it was
// handed is an old one, so it can steer the reader to the current slug.
//
// The order is the rule, not an optimisation: a slug another project took after
// this one left it belongs to the new holder, alias or not.
func (h *API) projectBySlug(ctx context.Context, s string) (p db.Project, aliased bool, err error) {
	p, err = h.q.GetProjectBySlug(ctx, s)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) || s == "" {
		return p, false, err
	}
	p, err = h.q.GetProjectByAlias(ctx, s)
	return p, err == nil, err
}

// ResolveProjectBySlug answers GET /project-by-slug?slug= — what a page that
// carries only the project's address (the stage page, #2993) needs to find the
// project, and to learn its current address when the one in the URL is old.
func (h *API) ResolveProjectBySlug(c *gin.Context) {
	p, _, err := h.projectBySlug(c, c.Query("slug"))
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// slugTakenBy names the project holding an address in a 409, as far as the
// caller may know about it.
type slugTakenBy struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	GroupID     *uuid.UUID `json:"group_id"`
	GroupName   string     `json:"group_name,omitempty"`
}

// respondSlugTaken writes the 409 for a project address another project
// already holds (#3013): the English sentinel clients already match, the first
// free variant of the address the user typed — what ⌥↵ puts into the field —
// and the holder.
//
// Project addresses are unique across the whole instance, so the holder may sit
// in a workspace the caller can't see. Its name is told only to members of its
// workspace; anyone else learns that the address is taken, which the 409 says
// anyway, and nothing about whose it is.
func (h *API) respondSlugTaken(c *gin.Context, s string) {
	body := gin.H{"error": "slug already taken", "suggestion": h.uniqueProjectSlug(c, s)}
	holder, err := h.q.GetProjectBySlug(c, s)
	if err == nil && h.lookupMembership(c, holder.WorkspaceID).ok {
		tb := slugTakenBy{
			ID: holder.ID, Name: holder.Name, Slug: holder.Slug,
			WorkspaceID: holder.WorkspaceID, GroupID: holder.GroupID,
		}
		if holder.GroupID != nil {
			if g, gerr := h.q.GetProjectGroup(c, *holder.GroupID); gerr == nil {
				tb.GroupName = g.Name
			}
		}
		body["taken_by"] = tb
	}
	c.JSON(http.StatusConflict, body)
}

// ProjectCounts answers GET /projects/:id/counts — what deleting the project
// takes with it, for the two-step confirmation («3 доски и 148 задач», #3014).
// Any member may ask: the numbers are what the tree already shows them.
func (h *API) ProjectCounts(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	fp, err := h.q.ProjectFootprint(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, fp)
}

// WorkspaceCounts answers GET /workspaces/:id/counts — projects, boards and
// tasks a workspace delete takes with it («4 проекта, 23 доски и 612 задач»).
func (h *API) WorkspaceCounts(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	fp, err := h.q.WorkspaceFootprint(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, fp)
}

// PurgeExpiredSlugAliases sweeps former project addresses past their 30 days.
// Readers already ignore them (GetProjectByAlias checks expires_at), so this is
// housekeeping; it rides the trash_purge pass.
func (h *API) PurgeExpiredSlugAliases(ctx context.Context) {
	soft(ctx, "PurgeExpiredProjectSlugAliases", func() error {
		_, err := h.q.PurgeExpiredProjectSlugAliases(ctx)
		return err
	}())
}
