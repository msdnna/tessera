package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// The tag table of the «Проект» window (#2990): one prefix's friendly name edited
// in its group header, merging duplicates into one tag, and deleting several
// tags at once with an undo.

// maxBulkTags bounds one merge / delete / restore request.
const maxBulkTags = 500

// projectForTags resolves the project of a tag-table request and checks the
// caller's membership. ok=false once the answer has been written.
func (h *API) projectForTags(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForProject(c, projectID)
	if notFound(c, err) {
		return uuid.Nil, uuid.Nil, false
	}
	if err != nil {
		fail(c, err)
		return uuid.Nil, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, wsID, true
}

// projectTags loads the named tags and refuses any that is not this project's.
func (h *API) projectTags(c *gin.Context, projectID uuid.UUID, ids []uuid.UUID) ([]db.Tag, bool) {
	if len(ids) == 0 || len(ids) > maxBulkTags {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag list"})
		return nil, false
	}
	out := make([]db.Tag, 0, len(ids))
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		tag, err := h.q.GetTag(c, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && tag.ProjectID != projectID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return nil, false
		}
		if err != nil {
			fail(c, err)
			return nil, false
		}
		out = append(out, tag)
	}
	return out, true
}

// SetTagPrefix sets or clears ONE prefix's friendly name (PATCH
// /projects/:id/tag-prefixes). The group header of the tag table renames one
// prefix; going through SetTagPrefixes would have the client resend the whole
// set it last saw and overwrite a neighbour's edit made meanwhile. An empty
// label clears the name.
func (h *API) SetTagPrefix(c *gin.Context) {
	projectID, wsID, ok := h.projectForTags(c)
	if !ok {
		return
	}
	var req struct {
		Prefix string `json:"prefix"`
		Label  string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := canonPrefix(req.Prefix)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prefix is required"})
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		if err := h.q.DeleteTagPrefix(c, db.DeleteTagPrefixParams{ProjectID: projectID, Prefix: key}); err != nil {
			fail(c, err)
			return
		}
	} else if _, err := h.q.UpsertTagPrefix(c, db.UpsertTagPrefixParams{
		ProjectID: projectID, WorkspaceID: wsID, Prefix: key, Label: label,
	}); err != nil {
		fail(c, err)
		return
	}
	rows, err := h.q.ListTagPrefixes(c, projectID)
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "tag_prefixes.updated", gin.H{"project_id": projectID, "prefixes": rows})
	c.JSON(http.StatusOK, rows)
}

// MergeTags folds the source tags into the target (POST
// /projects/:id/tags/merge): every task that wore a source wears the target, the
// sources are deleted. Journalled per task like a hand edit — tagged for the
// target where it is new, untagged for each source — and the label write-back is
// queued for linked issues.
func (h *API) MergeTags(c *gin.Context) {
	projectID, wsID, ok := h.projectForTags(c)
	if !ok {
		return
	}
	var req struct {
		TargetID  uuid.UUID   `json:"target_id" binding:"required"`
		SourceIDs []uuid.UUID `json:"source_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	all := append([]uuid.UUID{req.TargetID}, req.SourceIDs...)
	tags, ok := h.projectTags(c, projectID, all)
	if !ok {
		return
	}
	target := tags[0]
	sources := make([]db.Tag, 0, len(tags)-1)
	for _, tg := range tags[1:] {
		if tg.ID != target.ID {
			sources = append(sources, tg)
		}
	}
	if len(sources) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to merge"})
		return
	}
	type untag struct {
		task uuid.UUID
		name string
	}
	var untagged []untag
	gained := map[uuid.UUID]bool{}
	if err := h.inTx(c, func(q *db.Queries) error {
		for _, src := range sources {
			links, err := q.TagTaskLinks(c, src.ID)
			if err != nil {
				return err
			}
			for _, l := range links {
				untagged = append(untagged, untag{task: l.TaskID, name: src.Name})
			}
			added, err := q.MergeTagLinks(c, db.MergeTagLinksParams{TargetID: target.ID, SourceID: src.ID})
			if err != nil {
				return err
			}
			for _, id := range added {
				gained[id] = true
			}
			if err := q.DeleteTag(c, src.ID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		fail(c, err)
		return
	}
	actor := middleware.CurrentUser(c)
	touched := map[uuid.UUID]bool{}
	for _, u := range untagged {
		h.logEvent(c, u.task, "untagged", map[string]any{"tag": u.name})
		touched[u.task] = true
	}
	for id := range gained {
		h.logEvent(c, id, "tagged", map[string]any{"tag": target.Name})
	}
	for id := range touched {
		h.enqueueWriteback(c, id, actor, gitlab.TrigLabels, map[string]any{})
	}
	for _, src := range sources {
		h.broadcast(wsID, "tag.deleted", gin.H{"id": src.ID})
	}
	h.broadcast(wsID, "tag.updated", target)
	c.JSON(http.StatusOK, gin.H{"target": target, "merged": len(sources), "tasks": len(touched)})
}

// deletedTag is one tag of a bulk delete as handed back for the undo: the row
// and who wore it.
type deletedTag struct {
	db.Tag
	Links []db.TagTaskLinksRow `json:"links"`
}

// DeleteTags removes several tags at once (POST /projects/:id/tags/delete) and
// answers with what they were — rows and task links — so the client's undo
// toast can put them back through RestoreTags.
func (h *API) DeleteTags(c *gin.Context) {
	projectID, wsID, ok := h.projectForTags(c)
	if !ok {
		return
	}
	var req struct {
		IDs []uuid.UUID `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tags, ok := h.projectTags(c, projectID, req.IDs)
	if !ok {
		return
	}
	out := make([]deletedTag, 0, len(tags))
	if err := h.inTx(c, func(q *db.Queries) error {
		for _, tg := range tags {
			links, err := q.TagTaskLinks(c, tg.ID)
			if err != nil {
				return err
			}
			if links == nil {
				links = []db.TagTaskLinksRow{}
			}
			if err := q.DeleteTag(c, tg.ID); err != nil {
				return err
			}
			out = append(out, deletedTag{Tag: tg, Links: links})
		}
		return nil
	}); err != nil {
		fail(c, err)
		return
	}
	for _, d := range out {
		h.broadcast(wsID, "tag.deleted", gin.H{"id": d.ID})
	}
	c.JSON(http.StatusOK, gin.H{"tags": out})
}

// RestoreTags is the undo of DeleteTags (POST /projects/:id/tags/restore): each
// tag comes back under its old id and is re-attached to the tasks that still
// exist in the project's workspace. A tag whose name was taken in the meantime
// is skipped and reported.
func (h *API) RestoreTags(c *gin.Context) {
	projectID, wsID, ok := h.projectForTags(c)
	if !ok {
		return
	}
	var req struct {
		Tags []struct {
			ID     uuid.UUID `json:"id"`
			Name   string    `json:"name"`
			Color  string    `json:"color"`
			Source string    `json:"source"`
			Links  []struct {
				TaskID uuid.UUID `json:"task_id"`
				Source string    `json:"source"`
			} `json:"links"`
		} `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Tags) == 0 || len(req.Tags) > maxBulkTags {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag list"})
		return
	}
	restored := make([]db.Tag, 0, len(req.Tags))
	skipped := []string{}
	for _, in := range req.Tags {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			continue
		}
		free, err := h.tagNameFree(c, projectID, name, in.ID)
		if err != nil {
			fail(c, err)
			return
		}
		if !free {
			skipped = append(skipped, name)
			continue
		}
		source := "user"
		if in.Source == "gitlab" {
			source = "gitlab"
		}
		tag, err := h.q.RestoreTag(c, db.RestoreTagParams{
			ID: in.ID, WorkspaceID: wsID, ProjectID: projectID, Name: name, Color: in.Color, Source: source,
		})
		if errors.Is(err, pgx.ErrNoRows) || isUniqueViolation(err) {
			// The id is alive again (a second undo) or the name got taken.
			skipped = append(skipped, name)
			continue
		}
		if err != nil {
			fail(c, err)
			return
		}
		for _, l := range in.Links {
			ls := "user"
			if l.Source == "gitlab" {
				ls = "gitlab"
			}
			_, err := h.q.RestoreTaskTag(c, db.RestoreTaskTagParams{
				TagID: tag.ID, Source: ls, TaskID: l.TaskID, WorkspaceID: wsID,
			})
			soft(c, "RestoreTaskTag", err)
		}
		restored = append(restored, tag)
		h.broadcast(wsID, "tag.created", tag)
	}
	c.JSON(http.StatusOK, gin.H{"tags": restored, "skipped": skipped})
}
