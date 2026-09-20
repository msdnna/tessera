package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// Shared resolvers: the write tools all speak in human terms (a column name, a
// tag name, "me"/"author", a #number) and have to land on UUIDs before touching
// the API.

// resolveColumn finds a board column by UUID or case-insensitive name. An empty
// want selects the leftmost column, which is where a new task belongs by default.
func resolveColumn(ctx context.Context, c *client.Client, boardID, want string) (model.Column, error) {
	cols, err := c.ListColumns(ctx, boardID)
	if err != nil {
		return model.Column{}, err
	}
	if len(cols) == 0 {
		return model.Column{}, fmt.Errorf("board %s has no columns", boardID)
	}
	if strings.TrimSpace(want) == "" {
		leftmost := cols[0]
		for _, col := range cols[1:] {
			if col.Position < leftmost.Position {
				leftmost = col
			}
		}
		return leftmost, nil
	}
	lower := strings.ToLower(strings.TrimSpace(want))
	var names []string
	for _, col := range cols {
		names = append(names, col.Name)
		if col.ID == want || strings.ToLower(strings.TrimSpace(col.Name)) == lower {
			return col, nil
		}
	}
	return model.Column{}, fmt.Errorf("no column matching %q on this board; available: %s", want, strings.Join(names, ", "))
}

// resolveAssignees maps assignee refs — a UUID, an email, a display name, "me"
// (token owner) or "author" (task creator) — to user ids. task supplies the
// author; workspaceID may be "" to derive it from the task.
func resolveAssignees(ctx context.Context, c *client.Client, refs []string, task model.TaskDetail, workspaceID string) ([]string, error) {
	var members []model.Member
	loaded := false
	loadMembers := func() error {
		if loaded {
			return nil
		}
		wsID := workspaceID
		if wsID == "" {
			var err error
			if wsID, err = workspaceIDForTask(ctx, c, task); err != nil {
				return err
			}
		}
		var err error
		if members, err = c.ListMembers(ctx, wsID); err != nil {
			return err
		}
		loaded = true
		return nil
	}

	seen := map[string]bool{}
	var ids []string
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		var id string
		switch {
		case strings.EqualFold(ref, "me"):
			if id = c.SelfID(ctx); id == "" {
				return nil, fmt.Errorf("cannot resolve 'me': /auth/me lookup failed")
			}
		case strings.EqualFold(ref, "author"):
			if task.CreatedBy == nil || *task.CreatedBy == "" {
				return nil, fmt.Errorf("task has no recorded author to assign")
			}
			id = *task.CreatedBy
		case uuidRe.MatchString(ref):
			id = ref
		default:
			if err := loadMembers(); err != nil {
				return nil, err
			}
			var err error
			if id, err = matchMember(ref, members); err != nil {
				return nil, err
			}
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// projectIDForBoard returns the project a board belongs to — the scope tags
// live in.
func projectIDForBoard(ctx context.Context, c *client.Client, boardID string) (string, error) {
	b, err := c.GetBoard(ctx, boardID)
	if err != nil {
		return "", err
	}
	return b.ProjectID, nil
}

// resolveTags maps tag names (or UUIDs) to the project's tag rows. Tags are
// project-scoped, so the same name in another project is a different row.
// createMissing defaults to off at the call sites: silently creating a tag turns
// a typo into permanent project clutter. Rows rather than bare ids, so a caller
// reporting what it did can name the tag the way the project spells it instead
// of echoing the caller's own casing back at them.
func resolveTags(ctx context.Context, c *client.Client, projectID string, names []string, createMissing bool) ([]model.Tag, error) {
	if len(names) == 0 {
		return nil, nil
	}
	existing, err := c.ListProjectTags(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]model.Tag, len(existing))
	byID := make(map[string]model.Tag, len(existing))
	var known []string
	for _, tg := range existing {
		byName[strings.ToLower(strings.TrimSpace(tg.Name))] = tg
		byID[tg.ID] = tg
		known = append(known, tg.Name)
	}

	seen := map[string]bool{}
	var out []model.Tag
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		var match model.Tag
		switch tg, ok := byName[strings.ToLower(name)]; {
		case ok:
			match = tg
		case byID[name].ID != "":
			match = byID[name]
		case createMissing:
			created, cErr := c.CreateTag(ctx, projectID, name, "")
			if cErr != nil {
				return nil, fmt.Errorf("create tag %q: %w", name, cErr)
			}
			byName[strings.ToLower(name)] = created
			match = created
		default:
			sort.Strings(known)
			return nil, fmt.Errorf("no tag %q in this project (pass create_missing=true to add it); existing: %s",
				name, strings.Join(known, ", "))
		}
		if !seen[match.ID] {
			seen[match.ID] = true
			out = append(out, match)
		}
	}
	return out, nil
}

// tagIDs flattens resolved tag rows to the ids the write endpoints take.
func tagIDs(tags []model.Tag) []string {
	if len(tags) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tags))
	for _, tg := range tags {
		ids = append(ids, tg.ID)
	}
	return ids
}

// matchProjectTags is resolveTags' read-only half, for detaching: it returns the
// rows it recognises and *skips* the rest instead of failing. A name the project
// does not carry is not an error when the goal is "the task must not have this
// tag" — that already holds. Attaching keeps the strict resolveTags: there a typo
// would either do nothing or, with create_missing, litter the project, so it has
// to be said out loud.
func matchProjectTags(ctx context.Context, c *client.Client, projectID string, names []string) ([]model.Tag, error) {
	if len(names) == 0 {
		return nil, nil
	}
	existing, err := c.ListProjectTags(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]model.Tag, len(existing))
	byID := make(map[string]model.Tag, len(existing))
	for _, tg := range existing {
		byName[strings.ToLower(strings.TrimSpace(tg.Name))] = tg
		byID[tg.ID] = tg
	}
	seen := map[string]bool{}
	var out []model.Tag
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		match, ok := byName[strings.ToLower(name)]
		if !ok {
			if match, ok = byID[name]; !ok {
				continue
			}
		}
		if seen[match.ID] {
			continue
		}
		seen[match.ID] = true
		out = append(out, match)
	}
	return out, nil
}

// tagIDSet indexes a task's tags by id — what a tag write has to be checked
// against to tell "attached it" from "it was already there".
func tagIDSet(tags []model.Tag) map[string]bool {
	out := make(map[string]bool, len(tags))
	for _, tg := range tags {
		out[tg.ID] = true
	}
	return out
}

// parseDateIn accepts an ISO date ("2026-08-21") or a full RFC3339 timestamp,
// reading anything that carries no offset of its own in loc.
//
// loc is the *profile's* zone, not the process's, and that is the whole point.
// A due date is a calendar day stored as midnight in the writer's own calendar,
// and every reader turns it back into a day by re-expressing that instant in the
// zone it is reading from (dueDayOf in frontend/src/utils/taskFilter.js,
// callerLocation in handlers/api.go, rank.IsOverdue here). Midnight taken in a
// zone *ahead* of the reader's is therefore an instant that still belongs to the
// previous day over there: written from a host on MSK for a profile on New York
// time, "2026-09-21" arrived as the 20th — a day early on the board, and overdue
// in my_tasks on the very day it was asked for. The reading half already asks
// /auth/me for the zone (Client.Location); this is the same rule on the way out.
//
// A full RFC3339 timestamp names its own offset and is left alone — the zone
// only fills in what the caller did not say.
//
// Relative wording ("завтра") is deliberately not parsed here — that is the
// backend quick-action engine's job, and a second parser would drift from it.
func parseDateIn(s string, loc *time.Location) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if loc == nil {
		loc = time.Local
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("cannot parse date %q: use 2006-01-02 or RFC3339", s)
}
