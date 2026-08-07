package handlers

import "tessera/internal/db"

// The board lists (top-level cards, subtasks, archive) ship 2500+ rows on a
// large board. The task description — free-form markdown that can run to
// kilobytes each — dominates that payload and is never rendered on a card, so
// we drop it from the wire here and let the modal / hover fetch it on demand
// (GET /tasks/:id/description). has_description preserves the card's "this task
// has a description" affordance without carrying the text itself.
//
// The zero-churn way to omit an embedded field is to re-declare it on the outer
// struct with the same json name + omitempty and leave it zero: the shallower
// field wins the name conflict and, being empty, is dropped — so the row's
// Description never reaches the encoder. (A bare json:"-" would NOT work: an
// ignored field doesn't compete for the name, so the promoted one still emits.)
// Everything else the frontend already consumes is promoted through unchanged.

type slimBoardTask struct {
	db.ListBoardTasksWithMetaRow
	Description    string `json:"description,omitempty"`
	HasDescription bool   `json:"has_description"`
}

func slimBoardTasks(rows []db.ListBoardTasksWithMetaRow) []slimBoardTask {
	out := make([]slimBoardTask, len(rows))
	for i, r := range rows {
		out[i] = slimBoardTask{ListBoardTasksWithMetaRow: r, HasDescription: r.Description != ""}
	}
	return out
}

type slimBoardSubtask struct {
	db.ListBoardSubtasksWithMetaRow
	Description    string `json:"description,omitempty"`
	HasDescription bool   `json:"has_description"`
}

func slimBoardSubtasks(rows []db.ListBoardSubtasksWithMetaRow) []slimBoardSubtask {
	out := make([]slimBoardSubtask, len(rows))
	for i, r := range rows {
		out[i] = slimBoardSubtask{ListBoardSubtasksWithMetaRow: r, HasDescription: r.Description != ""}
	}
	return out
}

type slimBoardArchived struct {
	db.ListBoardArchivedWithMetaRow
	Description    string `json:"description,omitempty"`
	HasDescription bool   `json:"has_description"`
}

func slimBoardArchivedTasks(rows []db.ListBoardArchivedWithMetaRow) []slimBoardArchived {
	out := make([]slimBoardArchived, len(rows))
	for i, r := range rows {
		out[i] = slimBoardArchived{ListBoardArchivedWithMetaRow: r, HasDescription: r.Description != ""}
	}
	return out
}
