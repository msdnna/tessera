package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

const (
	// trashRetention is how long a soft-deleted board or column (0078) can still
	// be restored. The v2 client offers the undo for seven seconds; the rest is a
	// margin for a slow restore request and for a support-desk rescue, not a
	// recycle bin anyone browses.
	trashRetention = 24 * time.Hour

	// taskTrashRetention is how long a task or a document stays in its trash
	// (0086). Unlike a board, these are browsed: the board's «Корзина» and the
	// documents tree list them with «Вернуть» for the whole period.
	taskTrashRetention = 30 * 24 * time.Hour

	// trashPurgeEvery matches the interval published in backgroundWorkers.
	trashPurgeEvery = 10 * time.Minute

	// trashPurgeOpTimeout bounds one pass.
	trashPurgeOpTimeout = 2 * time.Minute
)

// RunTrashWorker finishes the deletes the soft-delete left pending: boards and
// columns that stayed deleted past trashRetention, tasks and documents past
// taskTrashRetention, are removed for good.
func (h *API) RunTrashWorker(ctx context.Context) {
	ticker := time.NewTicker(trashPurgeEvery)
	defer ticker.Stop()
	h.tick(jobTrashPurge, opTrashPurge)
	h.withAdvisoryLock(ctx, trashPurgeLock, func() { h.PurgeTrash(ctx) })
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobTrashPurge, opTrashPurge)
			h.withAdvisoryLock(ctx, trashPurgeLock, func() { h.PurgeTrash(ctx) })
		}
	}
}

// PurgeTrash runs one pass. Exported for the jobs screen's "run now" and for
// the integration tests, which call it with the clock already past the cutoff.
func (h *API) PurgeTrash(ctx context.Context) {
	now := time.Now()
	h.purgeTrashBefore(ctx, now.Add(-trashRetention), now.Add(-taskTrashRetention))
}

// purgeTrashBefore takes two cutoffs: containers (boards, columns) are kept a
// day, browsable items (tasks, documents) a month.
func (h *API) purgeTrashBefore(ctx context.Context, cutoff, itemCutoff time.Time) {
	ctx, cancel := context.WithTimeout(ctx, trashPurgeOpTimeout)
	defer cancel()
	// A board takes its cards through the cascade, so their upload folders are
	// read first and swept once the delete is committed — what DeleteBoard did
	// before the soft delete.
	var gone []uuid.UUID
	if err := h.inTx(ctx, func(q *db.Queries) error {
		var err error
		if gone, err = q.DeletedBoardTaskIDs(ctx, cutoff); err != nil {
			return err
		}
		_, err = q.PurgeDeletedBoards(ctx, cutoff)
		return err
	}); err != nil {
		slog.Warn("trash purge: boards", "err", err)
	} else {
		h.purgeTaskUploads(gone)
	}
	// A deleted column holds no cards (they moved out on delete), so purging it
	// takes only the column row and its move log.
	if _, err := h.q.PurgeDeletedColumns(ctx, cutoff); err != nil {
		slog.Warn("trash purge: columns", "err", err)
	}
	// Former project addresses past their 30 days (#3012) — on their own clock,
	// not the trash cutoff: expires_at is stamped when the address is left.
	h.PurgeExpiredSlugAliases(ctx)
	// Tasks in the trash (0086) go with their subtasks and their upload folders,
	// the way «Удалить навсегда» (PurgeDeletedTask) takes them.
	if goneTasks, err := h.q.PurgeDeletedTasks(ctx, itemCutoff); err != nil {
		slog.Warn("trash purge: tasks", "err", err)
	} else {
		h.purgeTaskUploads(goneTasks)
	}
	h.purgeDeletedDocuments(ctx, itemCutoff)
}
