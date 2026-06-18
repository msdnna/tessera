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
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListTagPrefixes(c, projectID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, rows)
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
		fail(c)
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
	if err := h.q.DeleteTagPrefixesForProject(c, projectID); err != nil {
		fail(c)
		return
	}
	// Dedup by canonical key (first label wins), skip blanks.
	seen := make(map[string]bool)
	out := make([]db.TagPrefix, 0, len(req.Prefixes))
	for _, p := range req.Prefixes {
		key := canonPrefix(p.Prefix)
		label := strings.TrimSpace(p.Label)
		if key == "" || label == "" || seen[key] {
			continue
		}
		seen[key] = true
		row, err := h.q.UpsertTagPrefix(c, db.UpsertTagPrefixParams{
			ProjectID: projectID, WorkspaceID: wsID, Prefix: key, Label: label,
		})
		if err != nil {
			fail(c)
			return
		}
		out = append(out, row)
	}
	h.broadcast(wsID, "tag_prefixes.updated", gin.H{"project_id": projectID, "prefixes": out})
	c.JSON(http.StatusOK, out)
}
