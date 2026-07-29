package tools

import (
	"context"
	"strconv"
	"time"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// priorityLabel maps the numeric priority (0..4) to a word. Matches the web
// PRIORITY_LABELS scale where higher = more urgent.
func priorityLabel(p int) string {
	switch p {
	case 4:
		return "urgent"
	case 3:
		return "high"
	case 2:
		return "normal"
	case 1:
		return "low"
	default:
		return "none"
	}
}

// dueString formats a due date as RFC3339, or "" when unset.
func dueString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// isOverdue reports whether an incomplete task's due date is in the past.
func isOverdue(t model.Task, now time.Time) bool {
	return t.CompletedAt == nil && t.DueDate != nil && t.DueDate.Before(now)
}

// summarize builds the compact task view shared by all list tools. columnName
// and url may be "" when unavailable.
func summarize(t model.Task, columnName string, tagNames map[string]string, url string, now time.Time) taskSummary {
	s := taskSummary{
		ID: t.ID, Number: t.Number, Title: t.Title,
		Priority: t.Priority, PriorityLabel: priorityLabel(t.Priority),
		Project: t.ProjectName, // populated only on workspace listings (board tasks leave it empty)
		Column:  columnName, Due: dueString(t.DueDate), Overdue: isOverdue(t, now),
		Completed: t.CompletedAt != nil, EstimateHours: t.Estimate,
		AssigneeCount: len(t.AssigneeIDs), URL: url,
	}
	for _, id := range t.TagIDs {
		if name, ok := tagNames[id]; ok {
			s.Tags = append(s.Tags, name)
		}
	}
	if t.GitlabURL != nil {
		s.GitlabURL = *t.GitlabURL
	}
	return s
}

// enrichment holds the board context needed to turn raw tasks into summaries:
// column names, tag names, and the pieces of a human-readable deep link.
type enrichment struct {
	c           *client.Client
	columns     map[string]string // column id → name
	tags        map[string]string // tag id → name
	projectSlug string
	boardSlug   string
}

// boardEnrichment loads a board's project, columns and tags in preparation for
// summarising its tasks.
func boardEnrichment(ctx context.Context, c *client.Client, boardID string) (enrichment, error) {
	e := enrichment{c: c, columns: map[string]string{}, tags: map[string]string{}}

	board, err := c.GetBoard(ctx, boardID)
	if err != nil {
		return e, err
	}
	e.boardSlug = board.Slug

	cols, err := c.ListColumns(ctx, boardID)
	if err != nil {
		return e, err
	}
	for _, col := range cols {
		e.columns[col.ID] = col.Name
	}

	// Project slug + tag names are best-effort enrichment; a failure here should
	// not fail the whole listing.
	if project, err := c.GetProject(ctx, board.ProjectID); err == nil {
		e.projectSlug = project.Slug
	}
	if tags, err := c.ListProjectTags(ctx, board.ProjectID); err == nil {
		for _, tg := range tags {
			e.tags[tg.ID] = tg.Name
		}
	}
	return e, nil
}

// summarize turns a board task into its compact view, including a deep link
// when the project/board slugs and task number are all known.
func (e enrichment) summarize(t model.Task, now time.Time) taskSummary {
	return summarize(t, e.columns[t.ColumnID], e.tags, e.taskURL(t), now)
}

// taskURL builds a /project/<slug>/board/<slug>?task=<number> deep link, or ""
// when any piece is missing.
func (e enrichment) taskURL(t model.Task) string {
	if e.projectSlug == "" || e.boardSlug == "" || t.Number == nil {
		return ""
	}
	return e.c.WebBaseURL() + "/project/" + e.projectSlug + "/board/" + e.boardSlug +
		"?task=" + strconv.FormatInt(*t.Number, 10)
}

// workspaceTagNames returns a tag id → name map spanning all of a workspace's
// projects, for summarising cross-project (my-tasks) listings.
func workspaceTagNames(ctx context.Context, c *client.Client, workspaceID string) (map[string]string, error) {
	tags, err := c.ListWorkspaceTags(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(tags))
	for _, tg := range tags {
		m[tg.ID] = tg.Name
	}
	return m, nil
}

// columnName resolves a single column id to its name (best-effort; "" on error).
func columnName(ctx context.Context, c *client.Client, boardID, columnID string) string {
	cols, err := c.ListColumns(ctx, boardID)
	if err != nil {
		return ""
	}
	for _, col := range cols {
		if col.ID == columnID {
			return col.Name
		}
	}
	return ""
}

// blockedTaskIDs returns the set of task ids that have an open (incomplete)
// blocker on the board, so next_task can skip them. tasks supplies completion
// status; the dependency graph supplies the blocks/blocked_by edges.
func blockedTaskIDs(ctx context.Context, c *client.Client, boardID string, tasks []model.Task) (map[string]bool, error) {
	deps, err := c.ListBoardDependencies(ctx, boardID)
	if err != nil {
		return nil, err
	}
	completed := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		completed[t.ID] = t.CompletedAt != nil
	}
	blocked := map[string]bool{}
	for _, d := range deps {
		// Normalise each edge to "blockedTask is blocked by blockerTask".
		var blockedTask, blockerTask string
		switch d.Kind {
		case "blocked_by":
			blockedTask, blockerTask = d.TaskID, d.RelatedTaskID
		case "blocks":
			blockedTask, blockerTask = d.RelatedTaskID, d.TaskID
		default:
			continue // relates/duplicates don't gate work
		}
		if !completed[blockerTask] {
			blocked[blockedTask] = true
		}
	}
	return blocked, nil
}
