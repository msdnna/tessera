package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// canonPrefix normalises a tag-prefix key so the namespace derived from a tag
// name ("S: " / "effort::") lines up with what the GitLab rule editor stores
// ("S:"). Mirror this in the frontend (utils/tagGroups.js canonPrefix).
func canonPrefix(p string) string {
	return strings.ToLower(strings.TrimSpace(p))
}

// ListTagPrefixes returns a project's prefix→label display names.
func (h *API) ListTagPrefixes(c *gin.Context) {
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
	rows, err := h.q.ListTagPrefixes(c, projectID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}

// ListWorkspaceTagPrefixes returns prefix→label display names across every
// project in a workspace — for cross-project views (Home, workspace task lists)
// that render scoped tag pills but have no single project to scope to. Prefixes
// are already canonical in storage; the same prefix may be labelled differently
// per project, so rows are deduped by prefix with the first non-empty label
// winning (query order is by prefix, so the result is stable).
func (h *API) ListWorkspaceTagPrefixes(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListWorkspaceTagPrefixes(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	seen := make(map[string]bool, len(rows))
	out := make([]db.TagPrefix, 0, len(rows))
	for _, r := range rows {
		key := canonPrefix(r.Prefix)
		if key == "" || strings.TrimSpace(r.Label) == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	c.JSON(http.StatusOK, out)
}

// SetTagPrefixes replaces the project's full set of prefix display names. The
// payload is the complete desired state: prefixes are canonicalised, blank
// labels are dropped, and anything not listed is removed. Callers that only
// manage a subset (e.g. the GitLab modal) must send the merged full set.
func (h *API) SetTagPrefixes(c *gin.Context) {
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
	var req struct {
		Prefixes []struct {
			Prefix string `json:"prefix"`
			Label  string `json:"label"`
		} `json:"prefixes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Transactional: this is DELETE-then-UPSERT. A crash between the two would
	// otherwise commit the delete and lose every prefix the project had.
	out := make([]db.TagPrefix, 0, len(req.Prefixes))
	if err := h.inTx(c, func(q *db.Queries) error {
		if err := q.DeleteTagPrefixesForProject(c, projectID); err != nil {
			return err
		}
		// Dedup by canonical key (first label wins), skip blanks.
		seen := make(map[string]bool)
		for _, p := range req.Prefixes {
			key := canonPrefix(p.Prefix)
			label := strings.TrimSpace(p.Label)
			if key == "" || label == "" || seen[key] {
				continue
			}
			seen[key] = true
			row, err := q.UpsertTagPrefix(c, db.UpsertTagPrefixParams{
				ProjectID: projectID, WorkspaceID: wsID, Prefix: key, Label: label,
			})
			if err != nil {
				return err
			}
			out = append(out, row)
		}
		return nil
	}); err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "tag_prefixes.updated", gin.H{"project_id": projectID, "prefixes": out})
	c.JSON(http.StatusOK, out)
}
