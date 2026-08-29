// Package tools registers the Tessera MCP tools on a server: read-only access
// to workspaces, projects, boards and a priority-ranked task queue.
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/rank"
)

// Register wires every tool onto the server, backed by the given API client.
func Register(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "tessera_list_workspaces",
		Description: "List the Tessera workspaces the connected account belongs to.",
	}, listWorkspaces(c))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "tessera_list_projects",
		Description: "List the projects inside a workspace.",
	}, listProjects(c))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "tessera_list_boards",
		Description: "List the kanban boards inside a project.",
	}, listBoards(c))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "tessera_resolve_board",
		Description: "Resolve a human-readable /project/<slug>/board/<slug> pair to its board.",
	}, resolveBoard(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_list_tasks",
		Description: "List a board's tasks as a priority-ranked work queue (incomplete first, " +
			"then overdue, then by priority and due date). Optionally scope to a milestone.",
	}, listTasks(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_my_tasks",
		Description: "List tasks assigned to the connected account across a whole workspace, " +
			"priority-ranked. A good 'what should I work on' entry point.",
	}, myTasks(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_next_task",
		Description: "Return the single highest-priority actionable task — the next thing to do. " +
			"Scope to a board (board_id) or a whole workspace (workspace_id). On a board, tasks " +
			"with an open blocker are skipped.",
	}, nextTask(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_get_task",
		Description: "Fetch a task's full detail (description markdown, tags, assignees, subtasks, " +
			"comments, image refs, GitLab link). Identify it by task_id, or by workspace_id + number (#252). " +
			"Comments carry the author and an is_agent flag so you can tell your own posts from the user's replies.",
	}, getTask(c))

	registerWrite(s, c)
}

// ── inputs ──────────────────────────────────────────────────────────────────

type workspaceInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace UUID"`
}
type projectInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project UUID"`
}
type resolveBoardInput struct {
	ProjectSlug string `json:"project_slug" jsonschema:"the project slug from the URL"`
	BoardSlug   string `json:"board_slug" jsonschema:"the board slug from the URL"`
}
type listTasksInput struct {
	BoardID          string `json:"board_id" jsonschema:"the board UUID"`
	Milestone        string `json:"milestone,omitempty" jsonschema:"optional: a milestone UUID to scope to, or 'backlog' for tasks without a milestone"`
	IncludeCompleted bool   `json:"include_completed,omitempty" jsonschema:"include completed tasks (ranked last); default false"`
}
type nextTaskInput struct {
	BoardID     string `json:"board_id,omitempty" jsonschema:"a board UUID to pick the next task from"`
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"a workspace UUID to pick the next assigned task from (used if board_id is empty)"`
}
type getTaskInput struct {
	TaskID      string `json:"task_id,omitempty" jsonschema:"the task UUID"`
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to look up a task by its #number"`
	Number      int64  `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
}

// ── outputs ─────────────────────────────────────────────────────────────────

type workspaceOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type projectOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
type boardOut struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
}

// The MCP spec requires a tool's output schema to be a JSON object, so list
// results are wrapped rather than returned as a bare array (some clients, e.g.
// Claude Code, reject a top-level "array" output schema).
type workspaceListOut struct {
	Workspaces []workspaceOut `json:"workspaces"`
}
type projectListOut struct {
	Projects []projectOut `json:"projects"`
}
type boardListOut struct {
	Boards []boardOut `json:"boards"`
}

// taskSummary is the compact, agent-friendly view of a task in a list.
type taskSummary struct {
	ID            string   `json:"id"`
	Number        *int64   `json:"number,omitempty"`
	Title         string   `json:"title"`
	Priority      int      `json:"priority"`
	PriorityLabel string   `json:"priority_label"`
	Project       string   `json:"project,omitempty"`
	Column        string   `json:"column,omitempty"`
	Due           string   `json:"due,omitempty"`
	Overdue       bool     `json:"overdue,omitempty"`
	Completed     bool     `json:"completed,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	AssigneeCount int      `json:"assignee_count,omitempty"`
	EstimateHours *float64 `json:"estimate_hours,omitempty"`
	GitlabURL     string   `json:"gitlab_url,omitempty"`
	URL           string   `json:"url,omitempty"`
}
type taskListOut struct {
	Count int           `json:"count"`
	Tasks []taskSummary `json:"tasks"`
}
type nextTaskOut struct {
	Found bool         `json:"found"`
	Task  *taskSummary `json:"task,omitempty"`
	Note  string       `json:"note,omitempty"`
}

type assigneeOut struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
type subtaskOut struct {
	ID        string `json:"id"`
	Number    *int64 `json:"number,omitempty"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}
type commentOut struct {
	Body      string `json:"body"`
	Author    string `json:"author,omitempty"`
	AuthorID  string `json:"author_id,omitempty"`
	IsAgent   bool   `json:"is_agent,omitempty"`
	CreatedAt string `json:"created_at"`
}
type relationOut struct {
	Kind      string `json:"kind"`
	Number    *int64 `json:"number,omitempty"`
	Title     string `json:"title,omitempty"`
	Completed bool   `json:"completed,omitempty"`
}
type attachmentRefOut struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}
type taskDetailOut struct {
	ID            string        `json:"id"`
	Number        *int64        `json:"number,omitempty"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Priority      int           `json:"priority"`
	PriorityLabel string        `json:"priority_label"`
	Column        string        `json:"column,omitempty"`
	Due           string        `json:"due,omitempty"`
	Overdue       bool          `json:"overdue,omitempty"`
	Completed     bool          `json:"completed"`
	EstimateHours *float64      `json:"estimate_hours,omitempty"`
	Tags          []string      `json:"tags,omitempty"`
	Assignees     []assigneeOut `json:"assignees,omitempty"`
	Subtasks      []subtaskOut  `json:"subtasks,omitempty"`
	Comments      []commentOut  `json:"comments,omitempty"`
	Images        []imageRefOut `json:"images,omitempty"`
	GitlabURL     string        `json:"gitlab_url,omitempty"`

	Relations   []relationOut      `json:"relations,omitempty"`
	Attachments []attachmentRefOut `json:"attachments,omitempty"`
}

// ── handlers ────────────────────────────────────────────────────────────────

func listWorkspaces(c *client.Client) mcp.ToolHandlerFor[struct{}, workspaceListOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, workspaceListOut, error) {
		ws, err := c.ListWorkspaces(ctx)
		if err != nil {
			return nil, workspaceListOut{}, err
		}
		out := make([]workspaceOut, 0, len(ws))
		for _, w := range ws {
			out = append(out, workspaceOut{ID: w.ID, Name: w.Name})
		}
		return nil, workspaceListOut{Workspaces: out}, nil
	}
}

func listProjects(c *client.Client) mcp.ToolHandlerFor[workspaceInput, projectListOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in workspaceInput) (*mcp.CallToolResult, projectListOut, error) {
		if in.WorkspaceID == "" {
			return nil, projectListOut{}, fmt.Errorf("workspace_id is required")
		}
		ps, err := c.ListProjects(ctx, in.WorkspaceID)
		if err != nil {
			return nil, projectListOut{}, err
		}
		out := make([]projectOut, 0, len(ps))
		for _, p := range ps {
			out = append(out, projectOut{ID: p.ID, Name: p.Name, Slug: p.Slug})
		}
		return nil, projectListOut{Projects: out}, nil
	}
}

func listBoards(c *client.Client) mcp.ToolHandlerFor[projectInput, boardListOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, boardListOut, error) {
		if in.ProjectID == "" {
			return nil, boardListOut{}, fmt.Errorf("project_id is required")
		}
		bs, err := c.ListBoards(ctx, in.ProjectID)
		if err != nil {
			return nil, boardListOut{}, err
		}
		out := make([]boardOut, 0, len(bs))
		for _, b := range bs {
			out = append(out, boardOut{ID: b.ID, ProjectID: b.ProjectID, Name: b.Name, Slug: b.Slug})
		}
		return nil, boardListOut{Boards: out}, nil
	}
}

func resolveBoard(c *client.Client) mcp.ToolHandlerFor[resolveBoardInput, boardOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in resolveBoardInput) (*mcp.CallToolResult, boardOut, error) {
		if in.ProjectSlug == "" || in.BoardSlug == "" {
			return nil, boardOut{}, fmt.Errorf("project_slug and board_slug are required")
		}
		b, err := c.ResolveBoardBySlug(ctx, in.ProjectSlug, in.BoardSlug)
		if err != nil {
			return nil, boardOut{}, err
		}
		return nil, boardOut{ID: b.ID, ProjectID: b.ProjectID, Name: b.Name, Slug: b.Slug}, nil
	}
}

func listTasks(c *client.Client) mcp.ToolHandlerFor[listTasksInput, taskListOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listTasksInput) (*mcp.CallToolResult, taskListOut, error) {
		if in.BoardID == "" {
			return nil, taskListOut{}, fmt.Errorf("board_id is required")
		}
		enr, err := boardEnrichment(ctx, c, in.BoardID)
		if err != nil {
			return nil, taskListOut{}, err
		}
		tasks, err := c.ListBoardTasks(ctx, in.BoardID, in.Milestone)
		if err != nil {
			return nil, taskListOut{}, err
		}
		now := time.Now()
		ranked := rank.Tasks(tasks, rank.Options{IncludeCompleted: in.IncludeCompleted, Now: now})
		out := taskListOut{Count: len(ranked)}
		for _, t := range ranked {
			out.Tasks = append(out.Tasks, enr.summarize(t, now))
		}
		return nil, out, nil
	}
}

func myTasks(c *client.Client) mcp.ToolHandlerFor[workspaceInput, taskListOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in workspaceInput) (*mcp.CallToolResult, taskListOut, error) {
		if in.WorkspaceID == "" {
			return nil, taskListOut{}, fmt.Errorf("workspace_id is required")
		}
		tagNames, err := workspaceTagNames(ctx, c, in.WorkspaceID)
		if err != nil {
			return nil, taskListOut{}, err
		}
		tasks, err := c.ListWorkspaceTasks(ctx, in.WorkspaceID, true)
		if err != nil {
			return nil, taskListOut{}, err
		}
		now := time.Now()
		ranked := rank.Tasks(tasks, rank.Options{Now: now})
		out := taskListOut{Count: len(ranked)}
		for _, t := range ranked {
			out.Tasks = append(out.Tasks, summarize(t, t.ColumnName, tagNames, "", now))
		}
		return nil, out, nil
	}
}

func nextTask(c *client.Client) mcp.ToolHandlerFor[nextTaskInput, nextTaskOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in nextTaskInput) (*mcp.CallToolResult, nextTaskOut, error) {
		now := time.Now()
		switch {
		case in.BoardID != "":
			enr, err := boardEnrichment(ctx, c, in.BoardID)
			if err != nil {
				return nil, nextTaskOut{}, err
			}
			tasks, err := c.ListBoardTasks(ctx, in.BoardID, "")
			if err != nil {
				return nil, nextTaskOut{}, err
			}
			blocked, err := blockedTaskIDs(ctx, c, in.BoardID, tasks)
			if err != nil {
				return nil, nextTaskOut{}, err
			}
			for _, t := range rank.Tasks(tasks, rank.Options{Now: now}) {
				if blocked[t.ID] {
					continue
				}
				s := enr.summarize(t, now)
				return nil, nextTaskOut{Found: true, Task: &s}, nil
			}
			return nil, nextTaskOut{Found: false, Note: "no actionable tasks (all done or blocked)"}, nil

		case in.WorkspaceID != "":
			tagNames, err := workspaceTagNames(ctx, c, in.WorkspaceID)
			if err != nil {
				return nil, nextTaskOut{}, err
			}
			tasks, err := c.ListWorkspaceTasks(ctx, in.WorkspaceID, true)
			if err != nil {
				return nil, nextTaskOut{}, err
			}
			ranked := rank.Tasks(tasks, rank.Options{Now: now})
			if len(ranked) == 0 {
				return nil, nextTaskOut{Found: false, Note: "no open tasks assigned to you"}, nil
			}
			s := summarize(ranked[0], ranked[0].ColumnName, tagNames, "", now)
			return nil, nextTaskOut{Found: true, Task: &s}, nil

		default:
			return nil, nextTaskOut{}, fmt.Errorf("provide either board_id or workspace_id")
		}
	}
}

func getTask(c *client.Client) mcp.ToolHandlerFor[getTaskInput, taskDetailOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getTaskInput) (*mcp.CallToolResult, taskDetailOut, error) {
		// resolveTaskDetail loads full detail (subtasks/tags/assignees) for both the
		// by-id and by-number paths — the raw by-number endpoint returns a stub.
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, taskDetailOut{}, err
		}

		now := time.Now()
		out := taskDetailOut{
			ID: d.ID, Number: d.Number, Title: d.Title, Description: d.Description,
			Priority: d.Priority, PriorityLabel: priorityLabel(d.Priority),
			Due: dueString(d.DueDate), Overdue: isOverdue(d.Task, now),
			Completed: d.CompletedAt != nil, EstimateHours: d.Estimate,
			Column: columnName(ctx, c, d.BoardID, d.ColumnID),
		}
		for _, t := range d.Tags {
			out.Tags = append(out.Tags, t.Name)
		}
		for _, a := range d.Assignees {
			out.Assignees = append(out.Assignees, assigneeOut{Name: a.Name, Email: a.Email})
		}
		for _, st := range d.Subtasks {
			out.Subtasks = append(out.Subtasks, subtaskOut{
				ID: st.ID, Number: st.Number, Title: st.Title, Completed: st.CompletedAt != nil,
			})
		}
		if d.GitLab != nil {
			out.GitlabURL = d.GitLab.WebURL
		}

		self := c.SelfID(ctx) // "" when unknown; never falsely matches
		comments, _ := c.ListComments(ctx, d.ID)
		for _, cm := range comments {
			out.Comments = append(out.Comments, commentOut{
				Body: cm.Body, Author: commentAuthor(cm), AuthorID: cm.AuthorID,
				IsAgent:   self != "" && cm.AuthorID == self,
				CreatedAt: cm.CreatedAt.Format(time.RFC3339),
			})
		}
		attachments, _ := c.ListAttachments(ctx, d.ID)
		out.Images = collectImages(d.Description, comments, attachments)
		for _, a := range attachments {
			out.Attachments = append(out.Attachments, attachmentRefOut{ID: a.ID, Filename: a.Filename, Size: a.Size})
		}
		// Relations and attachments are best-effort enrichment, like comments:
		// a failure here shouldn't cost the caller the whole task detail.
		relations, _ := c.ListRelations(ctx, d.ID)
		for _, r := range relations {
			out.Relations = append(out.Relations, relationOut{
				Kind: r.Kind, Number: r.RelatedNumber, Title: r.RelatedTitle,
				Completed: r.RelatedCompletedAt != nil,
			})
		}
		return nil, out, nil
	}
}
