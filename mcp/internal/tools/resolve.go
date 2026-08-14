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

// resolveTags maps tag names (or UUIDs) to tag ids within a project. Tags are
// project-scoped, so the same name in another project is a different row.
// createMissing defaults to off at the call sites: silently creating a tag turns
// a typo into permanent project clutter.
func resolveTags(ctx context.Context, c *client.Client, projectID string, names []string, createMissing bool) ([]string, error) {
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
	var ids []string
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		var id string
		switch tg, ok := byName[strings.ToLower(name)]; {
		case ok:
			id = tg.ID
		case byID[name].ID != "":
			id = name
		case createMissing:
			created, cErr := c.CreateTag(ctx, projectID, name, "")
			if cErr != nil {
				return nil, fmt.Errorf("create tag %q: %w", name, cErr)
			}
			byName[strings.ToLower(name)] = created
			id = created.ID
		default:
			sort.Strings(known)
			return nil, fmt.Errorf("no tag %q in this project (pass create_missing=true to add it); existing: %s",
				name, strings.Join(known, ", "))
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// parseDate accepts an ISO date ("2026-08-21") or a full RFC3339 timestamp.
// Relative wording ("завтра") is deliberately not parsed here — that is the
// backend quick-action engine's job, and a second parser would drift from it.
func parseDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("cannot parse date %q: use 2006-01-02 or RFC3339", s)
}
