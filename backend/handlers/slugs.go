package handlers

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/slug"
)

// uniqueProjectSlug returns a globally-unique URL slug for a project name.
func (h *API) uniqueProjectSlug(ctx context.Context, name string) string {
	base := slug.Make(name)
	if base == "" {
		base = "project"
	}
	s := base
	for i := 2; ; i++ {
		ex, err := h.q.ProjectSlugExists(ctx, s)
		if err != nil || !ex {
			return s
		}
		s = base + "-" + strconv.Itoa(i)
	}
}

// normalizeProjectSlug turns a user-supplied project address into its canonical
// form. Unlike uniqueProjectSlug it never invents a suffix: when the caller asks
// for a specific address, a collision is an error to report, not to paper over.
// Returns ok=false when nothing usable remains after normalization.
func normalizeProjectSlug(raw string) (string, bool) {
	s := slug.Make(raw)
	return s, s != ""
}

// uniqueBoardSlug returns a slug unique within the project for a board name,
// appending -2, -3, … on collision. Falls back to "board" when empty.
func (h *API) uniqueBoardSlug(ctx context.Context, projectID uuid.UUID, name string) string {
	base := slug.Make(name)
	if base == "" {
		base = "board"
	}
	s := base
	for i := 2; ; i++ {
		ex, err := h.q.BoardSlugExistsInProject(ctx, db.BoardSlugExistsInProjectParams{ProjectID: projectID, Slug: s})
		if err != nil || !ex {
			return s
		}
		s = base + "-" + strconv.Itoa(i)
	}
}

// uniqueMilestoneSlug returns a slug unique within the project for a milestone
// title. "backlog" is treated as taken: ?milestone=backlog is the reserved
// no-milestone scope, so a milestone actually named «Backlog» becomes backlog-2
// rather than shadowing it.
func (h *API) uniqueMilestoneSlug(ctx context.Context, projectID uuid.UUID, title string) string {
	base := slug.Make(title)
	if base == "" {
		base = "milestone"
	}
	s := base
	for i := 2; ; i++ {
		if s != backlogScope {
			ex, err := h.q.MilestoneSlugExistsInProject(ctx, db.MilestoneSlugExistsInProjectParams{ProjectID: projectID, Slug: s})
			if err != nil || !ex {
				return s
			}
		}
		s = base + "-" + strconv.Itoa(i)
	}
}

// uniqueNoteSlug returns a slug unique within the workspace for a note title.
func (h *API) uniqueNoteSlug(ctx context.Context, wsID uuid.UUID, title string) string {
	base := slug.Make(title)
	if base == "" {
		base = "note"
	}
	s := base
	for i := 2; ; i++ {
		ex, err := h.q.NoteSlugExists(ctx, db.NoteSlugExistsParams{WorkspaceID: wsID, Slug: s})
		if err != nil || !ex {
			return s
		}
		s = base + "-" + strconv.Itoa(i)
	}
}

// uniqueDocumentSlug returns a slug unique within the workspace for a document
// title.
func (h *API) uniqueDocumentSlug(ctx context.Context, wsID uuid.UUID, title string) string {
	base := slug.Make(title)
	if base == "" {
		base = "document"
	}
	s := base
	for i := 2; ; i++ {
		ex, err := h.q.DocumentSlugExists(ctx, db.DocumentSlugExistsParams{WorkspaceID: wsID, Slug: s})
		if err != nil || !ex {
			return s
		}
		s = base + "-" + strconv.Itoa(i)
	}
}

// freeProjectSlugs re-addresses the notes and documents of a project that is
// about to be re-stamped into targetWS, so none of them lands on an address the
// target workspace already uses. Runs inside the transfer's transaction (q is
// the tx-bound Queries), before the workspace_id re-stamp — until that happens
// the moving rows are invisible to the target's existence checks, which is
// exactly what makes a row's own address look free.
func (h *API) freeProjectSlugs(ctx context.Context, q *db.Queries, projectID, targetWS uuid.UUID) error {
	notes, err := q.ProjectNoteSlugs(ctx, &projectID)
	if err != nil {
		return err
	}
	noteRows := make([]slugRow, 0, len(notes))
	for _, n := range notes {
		noteRows = append(noteRows, slugRow{id: n.ID, slug: n.Slug})
	}
	err = freeSlugsInto(noteRows,
		func(s string) (bool, error) {
			return q.NoteSlugExists(ctx, db.NoteSlugExistsParams{WorkspaceID: targetWS, Slug: s})
		},
		func(id uuid.UUID, s string) error {
			return q.SetNoteSlug(ctx, db.SetNoteSlugParams{ID: id, Slug: s})
		})
	if err != nil {
		return err
	}

	docs, err := q.ProjectDocumentSlugs(ctx, &projectID)
	if err != nil {
		return err
	}
	docRows := make([]slugRow, 0, len(docs))
	for _, d := range docs {
		docRows = append(docRows, slugRow{id: d.ID, slug: d.Slug})
	}
	return freeSlugsInto(docRows,
		func(s string) (bool, error) {
			return q.DocumentSlugExists(ctx, db.DocumentSlugExistsParams{WorkspaceID: targetWS, Slug: s})
		},
		func(id uuid.UUID, s string) error {
			return q.SetDocumentSlug(ctx, db.SetDocumentSlugParams{ID: id, Slug: s})
		})
}

// freeSlugsInto frees a workspace-scoped address for every row that would land
// on one already in use. Notes and documents carry a slug unique per workspace
// (notes_ws_slug_key, idx_documents_ws_slug) while a project transfer moves them
// between workspaces, so a title shared by both sides — «Регламент» is not a rare
// one — would abort the whole transfer on the unique index with nothing in the
// error naming the row that caused it.
//
// Rows keep their address whenever it is free in the target and take the same
// -2, -3 … variant a new title would otherwise get. The taken set carries the
// moving rows' own addresses, so a rename can't collide with a sibling that
// already sits on the variant (two «Регламент» in the source: reglament and
// reglament-2, the first of which must skip past its sibling to reglament-3).
func freeSlugsInto(rows []slugRow, exists func(string) (bool, error), set func(uuid.UUID, string) error) error {
	taken := map[string]bool{}
	for _, r := range rows {
		taken[r.slug] = true
	}
	for _, r := range rows {
		clash, err := exists(r.slug)
		if err != nil {
			return err
		}
		if !clash {
			continue
		}
		free, err := firstFreeVariant(r.slug, taken, exists)
		if err != nil {
			return err
		}
		if err := set(r.id, free); err != nil {
			return err
		}
		taken[free] = true
	}
	return nil
}

// slugRow is one addressable row being moved: its id and current slug.
type slugRow struct {
	id   uuid.UUID
	slug string
}

// firstFreeVariant returns the first of base-2, base-3, … that is neither taken
// by a sibling nor already present in the target.
func firstFreeVariant(base string, taken map[string]bool, exists func(string) (bool, error)) (string, error) {
	for i := 2; ; i++ {
		cand := base + "-" + strconv.Itoa(i)
		if taken[cand] {
			continue
		}
		clash, err := exists(cand)
		if err != nil {
			return "", err
		}
		if !clash {
			return cand, nil
		}
	}
}

// BackfillSlugs assigns slugs to projects/boards/notes that lack one (rows that
// predate the slug columns, or boards reset by the per-project re-scope).
// Idempotent: only empty-slug rows are touched. Run once at startup.
func (h *API) BackfillSlugs(ctx context.Context) {
	projects, _ := h.q.ProjectsMissingSlug(ctx)
	for _, p := range projects {
		soft(ctx, "SetProjectSlug", h.q.SetProjectSlug(ctx, db.SetProjectSlugParams{ID: p.ID, Slug: h.uniqueProjectSlug(ctx, p.Name)}))
	}
	boards, _ := h.q.BoardsMissingSlug(ctx)
	for _, b := range boards {
		soft(ctx, "SetBoardSlug", h.q.SetBoardSlug(ctx, db.SetBoardSlugParams{ID: b.ID, Slug: h.uniqueBoardSlug(ctx, b.ProjectID, b.Name)}))
	}
	notes, _ := h.q.NotesMissingSlug(ctx)
	for _, n := range notes {
		soft(ctx, "SetNoteSlug", h.q.SetNoteSlug(ctx, db.SetNoteSlugParams{ID: n.ID, Slug: h.uniqueNoteSlug(ctx, n.WorkspaceID, n.Title)}))
	}
	documents, _ := h.q.DocumentsMissingSlug(ctx)
	for _, d := range documents {
		soft(ctx, "SetDocumentSlug", h.q.SetDocumentSlug(ctx, db.SetDocumentSlugParams{ID: d.ID, Slug: h.uniqueDocumentSlug(ctx, d.WorkspaceID, d.Title)}))
	}
	milestones, _ := h.q.MilestonesMissingSlug(ctx)
	for _, m := range milestones {
		soft(ctx, "SetMilestoneSlug", h.q.SetMilestoneSlug(ctx, db.SetMilestoneSlugParams{ID: m.ID, Slug: h.uniqueMilestoneSlug(ctx, m.ProjectID, m.Title)}))
	}
}
