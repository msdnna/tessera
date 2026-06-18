package handlers

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/slug"
)

// uniqueBoardSlug returns a globally-unique URL slug for a board name, appending
// -2, -3, … on collision. Falls back to "board" when the name yields nothing.
func (h *API) uniqueBoardSlug(ctx context.Context, name string) string {
	base := slug.Make(name)
	if base == "" {
		base = "board"
	}
	s := base
	for i := 2; ; i++ {
		ex, err := h.q.BoardSlugExists(ctx, s)
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

// BackfillSlugs assigns slugs to boards/notes that predate the slug column.
// Idempotent: only rows with an empty slug are touched. Run once at startup.
func (h *API) BackfillSlugs(ctx context.Context) {
	boards, _ := h.q.BoardsMissingSlug(ctx)
	for _, b := range boards {
		_ = h.q.SetBoardSlug(ctx, db.SetBoardSlugParams{ID: b.ID, Slug: h.uniqueBoardSlug(ctx, b.Name)})
	}
	notes, _ := h.q.NotesMissingSlug(ctx)
	for _, n := range notes {
		_ = h.q.SetNoteSlug(ctx, db.SetNoteSlugParams{ID: n.ID, Slug: h.uniqueNoteSlug(ctx, n.WorkspaceID, n.Title)})
	}
}
