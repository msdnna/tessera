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

// BackfillSlugs assigns slugs to projects/boards/notes that lack one (rows that
// predate the slug columns, or boards reset by the per-project re-scope).
// Idempotent: only empty-slug rows are touched. Run once at startup.
func (h *API) BackfillSlugs(ctx context.Context) {
	projects, _ := h.q.ProjectsMissingSlug(ctx)
	for _, p := range projects {
		_ = h.q.SetProjectSlug(ctx, db.SetProjectSlugParams{ID: p.ID, Slug: h.uniqueProjectSlug(ctx, p.Name)})
	}
	boards, _ := h.q.BoardsMissingSlug(ctx)
	for _, b := range boards {
		_ = h.q.SetBoardSlug(ctx, db.SetBoardSlugParams{ID: b.ID, Slug: h.uniqueBoardSlug(ctx, b.ProjectID, b.Name)})
	}
	notes, _ := h.q.NotesMissingSlug(ctx)
	for _, n := range notes {
		_ = h.q.SetNoteSlug(ctx, db.SetNoteSlugParams{ID: n.ID, Slug: h.uniqueNoteSlug(ctx, n.WorkspaceID, n.Title)})
	}
}
