package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// Task authoring tools: create a task or a batch of subtasks, patch fields,
// re-parent, and move a description between tasks.

// registerTasksWrite wires the task authoring/editing tools onto the server.
func registerTasksWrite(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_create_task",
		Description: "Create a task on a board (or a subtask, via parent_number/parent_id). Give it a title plus " +
			"any of description, priority, due_date, start_date, estimate_hours, assignees and tags. Column defaults " +
			"to the board's leftmost. Dates are ISO ('2026-08-21') or RFC3339 — relative wording like 'завтра' is not " +
			"parsed here, resolve it to a date first. When a parent is given, creating a subtask whose title already " +
			"matches a live subtask of that parent is rejected (pass allow_duplicate_titles to override) — this keeps " +
			"a repeated decomposition from spawning duplicates.",
	}, createTask(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_create_subtasks",
		Description: "Create several subtasks under one parent in a single call — the way to decompose a big task. " +
			"Each item takes a title plus optional description, priority, due_date, estimate_hours, assignees and tags. " +
			"A failing item doesn't roll back the rest: the result lists created and failed separately. Re-running is " +
			"safe: an item whose title already matches a live subtask of the parent is skipped (reported under " +
			"skipped_existing), not duplicated — pass allow_duplicate_titles to force creation.",
	}, createSubtasks(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_update_task",
		Description: "Update a task's fields — title, description, priority, dates, estimate, completed. Only the " +
			"fields you pass change; everything else is left alone. Use clear=['due_date'] to blank a date or the " +
			"estimate (pass description='' to empty a description). To change tags use tessera_set_tags, for the " +
			"column tessera_move_task, for assignees tessera_assign_task.",
	}, updateTask(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_set_parent",
		Description: "Make a task a subtask of another one (parent_number/parent_id), or detach=true to promote it " +
			"back to a top-level card. A task attached to a parent inherits the parent's board and column.",
	}, setParent(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_move_description",
		Description: "Move or copy a description from one task to another (e.g. when splitting a task into subtasks). " +
			"mode 'append' (default) adds it under the target's text, 'replace' overwrites; cut=true empties the source " +
			"afterwards — the write to the target happens first, so nothing is lost if the second call fails.",
	}, moveDescription(c))
}

// ── create_task ───────────────────────────────────────────────────────────────

// taskFieldsInput is the set of task fields shared by create_task and each item
// of create_subtasks.
type taskFieldsInput struct {
	Title         string   `json:"title" jsonschema:"the task title"`
	Description   string   `json:"description,omitempty" jsonschema:"Markdown description"`
	Priority      int      `json:"priority,omitempty" jsonschema:"0 none, 1 low, 2 normal, 3 high, 4 urgent"`
	DueDate       string   `json:"due_date,omitempty" jsonschema:"due date, ISO '2026-08-21' or RFC3339"`
	StartDate     string   `json:"start_date,omitempty" jsonschema:"start date, ISO '2026-08-21' or RFC3339"`
	EstimateHours float64  `json:"estimate_hours,omitempty" jsonschema:"estimate in hours (must be > 0)"`
	Assignees     []string `json:"assignees,omitempty" jsonschema:"who to assign: user emails, names, UUIDs or 'me' (the token owner)"`
	Tags          []string `json:"tags,omitempty" jsonschema:"tag names (project-scoped); unknown names fail unless create_missing_tags is true"`
}

type createTaskInput struct {
	BoardID              string `json:"board_id,omitempty" jsonschema:"the board UUID to create on (not needed when a parent is given)"`
	Column               string `json:"column,omitempty" jsonschema:"target column name or UUID; defaults to the board's leftmost column"`
	WorkspaceID          string `json:"workspace_id,omitempty" jsonschema:"workspace UUID — needed with parent_number, and to resolve assignee names/emails"`
	ParentID             string `json:"parent_id,omitempty" jsonschema:"create this as a subtask of the given task UUID"`
	ParentNumber         int64  `json:"parent_number,omitempty" jsonschema:"create this as a subtask of the given task #number (with workspace_id)"`
	CreateMissingTags    bool   `json:"create_missing_tags,omitempty" jsonschema:"create tags that don't exist in the project yet; default false so a typo can't litter the project"`
	AllowDuplicateTitles bool   `json:"allow_duplicate_titles,omitempty" jsonschema:"with a parent, allow creating a subtask whose title already matches a live subtask; default false rejects the duplicate"`
	taskFieldsInput
}

type createdTaskOut struct {
	ID       string `json:"id"`
	Number   *int64 `json:"number,omitempty"`
	Title    string `json:"title"`
	Column   string `json:"column,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	URL      string `json:"url,omitempty"`
	// Warnings carry post-create steps that failed (a tag or assignee that
	// wouldn't attach) — the task exists either way, so this must not be an error.
	Warnings []string `json:"warnings,omitempty"`
}

func createTask(c *client.Client) mcp.ToolHandlerFor[createTaskInput, createdTaskOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createTaskInput) (*mcp.CallToolResult, createdTaskOut, error) {
		if strings.TrimSpace(in.Title) == "" {
			return nil, createdTaskOut{}, fmt.Errorf("title is required")
		}

		boardID, columnID, columnLabel := in.BoardID, "", ""
		var parentID *string
		if in.ParentID != "" || in.ParentNumber != 0 {
			parent, err := resolveTaskDetail(ctx, c, in.ParentID, in.WorkspaceID, in.ParentNumber)
			if err != nil {
				return nil, createdTaskOut{}, fmt.Errorf("resolve parent: %w", err)
			}
			// Idempotency backstop: a stateless re-run that re-reads a task it
			// already decomposed must not spawn a second child with the same title.
			if !in.AllowDuplicateTitles {
				if ex, dup := liveChildTitles(parent)[normTitle(in.Title)]; dup {
					return nil, createdTaskOut{}, fmt.Errorf(
						"subtask %q already exists under the parent (%s) — pass allow_duplicate_titles:true to create another",
						strings.TrimSpace(in.Title), childRef(ex))
				}
			}
			// A subtask lives on its parent's board and column.
			boardID, columnID = parent.BoardID, parent.ColumnID
			parentID = &parent.ID
		}
		if boardID == "" {
			return nil, createdTaskOut{}, fmt.Errorf("provide board_id, or a parent (parent_id / workspace_id + parent_number)")
		}
		if columnID == "" || in.Column != "" {
			col, err := resolveColumn(ctx, c, boardID, in.Column)
			if err != nil {
				return nil, createdTaskOut{}, err
			}
			columnID, columnLabel = col.ID, col.Name
		}

		spec, err := buildTaskSpec(ctx, c, boardID, in.WorkspaceID, in.taskFieldsInput, in.CreateMissingTags)
		if err != nil {
			return nil, createdTaskOut{}, err
		}

		created, err := applyTaskSpec(ctx, c, boardID, columnID, parentID, spec)
		if err != nil {
			return nil, createdTaskOut{}, err
		}
		out := createdTaskOut{
			ID: created.task.ID, Number: created.task.Number, Title: created.task.Title,
			Column: columnLabel, Warnings: created.warnings,
			URL: taskURL(ctx, c, boardID, created.task),
		}
		if parentID != nil {
			out.ParentID = *parentID
		}
		return nil, out, nil
	}
}

// taskSpec is a create request with every human reference already resolved to
// ids. Resolving before the POST means an unknown tag or assignee fails loudly
// instead of leaving a half-configured task behind.
type taskSpec struct {
	params      client.NewTaskParams
	assigneeIDs []string
	tagIDs      []string
}

// buildTaskSpec validates and resolves one task's fields against a board.
func buildTaskSpec(ctx context.Context, c *client.Client, boardID, workspaceID string, f taskFieldsInput, createMissingTags bool) (taskSpec, error) {
	var spec taskSpec
	if strings.TrimSpace(f.Title) == "" {
		return spec, fmt.Errorf("title is required")
	}
	if f.Priority < 0 || f.Priority > 4 {
		return spec, fmt.Errorf("priority must be 0..4 (0 none … 4 urgent)")
	}
	due, err := parseDate(f.DueDate)
	if err != nil {
		return spec, err
	}
	start, err := parseDate(f.StartDate)
	if err != nil {
		return spec, err
	}
	spec.params = client.NewTaskParams{
		Title:       strings.TrimSpace(f.Title),
		Description: f.Description,
		Priority:    f.Priority,
		DueDate:     due,
		StartDate:   start,
	}
	if f.EstimateHours > 0 {
		est := f.EstimateHours
		spec.params.Estimate = &est
	}
	if len(f.Assignees) > 0 {
		// A brand-new task has no author yet, so 'author' has nothing to resolve
		// against — resolveAssignees reports that itself.
		if spec.assigneeIDs, err = resolveAssignees(ctx, c, f.Assignees, model.TaskDetail{}, workspaceID); err != nil {
			return spec, err
		}
	}
	if len(f.Tags) > 0 {
		projectID, pErr := projectIDForBoard(ctx, c, boardID)
		if pErr != nil {
			return spec, pErr
		}
		if spec.tagIDs, err = resolveTags(ctx, c, projectID, f.Tags, createMissingTags); err != nil {
			return spec, err
		}
	}
	return spec, nil
}

// createdTask is a created task plus non-fatal problems attaching its tags and
// assignees.
type createdTask struct {
	task     model.Task
	warnings []string
}

// applyTaskSpec creates the task and then attaches its tags and assignees, which
// the create endpoint does not accept inline.
func applyTaskSpec(ctx context.Context, c *client.Client, boardID, columnID string, parentID *string, spec taskSpec) (createdTask, error) {
	params := spec.params
	params.ColumnID = columnID
	params.ParentID = parentID
	t, err := c.CreateTask(ctx, boardID, params)
	if err != nil {
		return createdTask{}, err
	}
	out := createdTask{task: t}
	for _, id := range spec.tagIDs {
		if err := c.AddTaskTag(ctx, t.ID, id); err != nil {
			out.warnings = append(out.warnings, fmt.Sprintf("tag %s not attached: %v", id, err))
		}
	}
	for _, id := range spec.assigneeIDs {
		if err := c.AddAssignee(ctx, t.ID, id); err != nil {
			out.warnings = append(out.warnings, fmt.Sprintf("assignee %s not attached: %v", id, err))
		}
	}
	return out, nil
}

// taskURL builds the board deep link for a freshly created task (best-effort:
// "" when the board's project or the task number is unavailable).
func taskURL(ctx context.Context, c *client.Client, boardID string, t model.Task) string {
	enr, err := boardEnrichment(ctx, c, boardID)
	if err != nil {
		return ""
	}
	return enr.taskURL(t)
}

// ── create_subtasks ───────────────────────────────────────────────────────────

type createSubtasksInput struct {
	TaskID               string            `json:"task_id,omitempty" jsonschema:"the parent task UUID"`
	WorkspaceID          string            `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the parent, and to resolve assignee names/emails"`
	Number               int64             `json:"number,omitempty" jsonschema:"the parent's per-workspace task number, e.g. 252"`
	Items                []taskFieldsInput `json:"items" jsonschema:"the subtasks to create, in order"`
	CreateMissingTags    bool              `json:"create_missing_tags,omitempty" jsonschema:"create tags that don't exist in the project yet; default false"`
	AllowDuplicateTitles bool              `json:"allow_duplicate_titles,omitempty" jsonschema:"create items even when a live subtask of the parent already has the same title; default false skips them into skipped_existing"`
}

type failedItemOut struct {
	Title string `json:"title"`
	Error string `json:"error"`
}

// skippedItemOut is an item that was not created because a live subtask of the
// parent already carried the same title — the anti-duplicate guard.
type skippedItemOut struct {
	Title          string `json:"title"`
	ExistingID     string `json:"existing_id"`
	ExistingNumber *int64 `json:"existing_number,omitempty"`
}

type createSubtasksOut struct {
	ParentID string           `json:"parent_id"`
	Created  []createdTaskOut `json:"created,omitempty"`
	Skipped  []skippedItemOut `json:"skipped_existing,omitempty"`
	Failed   []failedItemOut  `json:"failed,omitempty"`
}

func createSubtasks(c *client.Client) mcp.ToolHandlerFor[createSubtasksInput, createSubtasksOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createSubtasksInput) (*mcp.CallToolResult, createSubtasksOut, error) {
		if len(in.Items) == 0 {
			return nil, createSubtasksOut{}, fmt.Errorf("items is required")
		}
		parent, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, createSubtasksOut{}, err
		}

		// Index the parent's existing live children so a re-run — the night
		// agent's stateless passes re-read the task and re-decompose it — skips
		// what it already made instead of duplicating it. Newly created items are
		// folded in too, so identical titles within one batch collapse as well.
		var seen map[string]model.Task
		if !in.AllowDuplicateTitles {
			seen = liveChildTitles(parent)
		}

		out := createSubtasksOut{ParentID: parent.ID}
		for _, item := range in.Items {
			if seen != nil {
				if ex, dup := seen[normTitle(item.Title)]; dup {
					out.Skipped = append(out.Skipped, skippedItemOut{
						Title: item.Title, ExistingID: ex.ID, ExistingNumber: ex.Number,
					})
					continue
				}
			}
			spec, sErr := buildTaskSpec(ctx, c, parent.BoardID, in.WorkspaceID, item, in.CreateMissingTags)
			if sErr != nil {
				out.Failed = append(out.Failed, failedItemOut{Title: item.Title, Error: sErr.Error()})
				continue
			}
			created, cErr := applyTaskSpec(ctx, c, parent.BoardID, parent.ColumnID, &parent.ID, spec)
			if cErr != nil {
				out.Failed = append(out.Failed, failedItemOut{Title: item.Title, Error: cErr.Error()})
				continue
			}
			out.Created = append(out.Created, createdTaskOut{
				ID: created.task.ID, Number: created.task.Number, Title: created.task.Title,
				ParentID: parent.ID, Warnings: created.warnings,
			})
			if seen != nil {
				seen[normTitle(created.task.Title)] = model.Task{
					ID: created.task.ID, Title: created.task.Title, Number: created.task.Number,
				}
			}
		}
		return nil, out, nil
	}
}

// normTitle collapses a title to a comparison key so a re-run doesn't create a
// second child that only differs by case or surrounding whitespace.
func normTitle(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// liveChildTitles indexes a parent's non-archived subtasks by normalized title.
// It backs the create-time anti-duplicate guard; archived children are ignored so
// re-creating a title that was deliberately archived still works.
func liveChildTitles(parent model.TaskDetail) map[string]model.Task {
	m := make(map[string]model.Task, len(parent.Subtasks))
	for _, st := range parent.Subtasks {
		if st.ArchivedAt != nil {
			continue
		}
		m[normTitle(st.Title)] = st
	}
	return m
}

// childRef renders an existing child as #number when known, else its id — for
// the "already exists" message.
func childRef(t model.Task) string {
	if t.Number != nil {
		return fmt.Sprintf("#%d", *t.Number)
	}
	return t.ID
}

// ── update_task ───────────────────────────────────────────────────────────────

type updateTaskInput struct {
	TaskID        string   `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID"`
	WorkspaceID   string   `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number        int64    `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Title         *string  `json:"title,omitempty" jsonschema:"new title (must not be empty)"`
	Description   *string  `json:"description,omitempty" jsonschema:"new Markdown description; pass '' to empty it (this replaces the text — use tessera_update_description to append)"`
	Priority      *int     `json:"priority,omitempty" jsonschema:"0 none, 1 low, 2 normal, 3 high, 4 urgent"`
	DueDate       *string  `json:"due_date,omitempty" jsonschema:"new due date, ISO '2026-08-21' or RFC3339"`
	StartDate     *string  `json:"start_date,omitempty" jsonschema:"new start date, ISO '2026-08-21' or RFC3339"`
	EstimateHours *float64 `json:"estimate_hours,omitempty" jsonschema:"estimate in hours; must be > 0 (use clear to remove it)"`
	Completed     *bool    `json:"completed,omitempty" jsonschema:"mark done/undone — note the board moves the card to/from the done column"`
	Clear         []string `json:"clear,omitempty" jsonschema:"fields to blank out: due_date, start_date, estimate_hours"`
}

type updateTaskOut struct {
	TaskID  string   `json:"task_id"`
	Number  *int64   `json:"number,omitempty"`
	Changed []string `json:"changed"`
}

// clearableFields are the task fields the backend's tri-state PATCH actually
// nulls out. Description/title/priority read a JSON null as "absent", so
// clearing them this way would be a silent no-op.
var clearableFields = map[string]string{
	"due_date":       "due_date",
	"start_date":     "start_date",
	"estimate":       "estimate",
	"estimate_hours": "estimate",
}

func updateTask(c *client.Client) mcp.ToolHandlerFor[updateTaskInput, updateTaskOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateTaskInput) (*mcp.CallToolResult, updateTaskOut, error) {
		patch := client.TaskPatch{}
		var changed []string

		if in.Title != nil {
			if strings.TrimSpace(*in.Title) == "" {
				return nil, updateTaskOut{}, fmt.Errorf("title must not be empty")
			}
			patch.Set("title", *in.Title)
			changed = append(changed, "title")
		}
		if in.Description != nil {
			patch.Set("description", *in.Description)
			changed = append(changed, "description")
		}
		if in.Priority != nil {
			if *in.Priority < 0 || *in.Priority > 4 {
				return nil, updateTaskOut{}, fmt.Errorf("priority must be 0..4 (0 none … 4 urgent)")
			}
			patch.Set("priority", *in.Priority)
			changed = append(changed, "priority")
		}
		for _, f := range []struct {
			name string
			val  *string
		}{{"due_date", in.DueDate}, {"start_date", in.StartDate}} {
			if f.val == nil {
				continue
			}
			t, err := parseDate(*f.val)
			if err != nil {
				return nil, updateTaskOut{}, err
			}
			if t == nil {
				return nil, updateTaskOut{}, fmt.Errorf("%s is empty; use clear=[%q] to remove it", f.name, f.name)
			}
			patch.Set(f.name, t.Format(timeRFC3339))
			changed = append(changed, f.name)
		}
		if in.EstimateHours != nil {
			// The backend turns a non-positive estimate into NULL, so "0" would
			// quietly mean "clear" — make the caller say so explicitly.
			if *in.EstimateHours <= 0 {
				return nil, updateTaskOut{}, fmt.Errorf("estimate_hours must be > 0; use clear=[\"estimate_hours\"] to remove it")
			}
			patch.Set("estimate", *in.EstimateHours)
			changed = append(changed, "estimate")
		}
		if in.Completed != nil {
			patch.Set("completed", *in.Completed)
			changed = append(changed, "completed")
		}
		for _, raw := range in.Clear {
			field, ok := clearableFields[strings.ToLower(strings.TrimSpace(raw))]
			if !ok {
				return nil, updateTaskOut{}, fmt.Errorf("cannot clear %q; clearable: due_date, start_date, estimate_hours "+
					"(to empty a description pass description='')", raw)
			}
			patch.Clear(field)
			changed = append(changed, "clear "+field)
		}
		if len(patch) == 0 {
			return nil, updateTaskOut{}, fmt.Errorf("nothing to update: pass at least one field or clear")
		}

		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, updateTaskOut{}, err
		}
		if err := c.UpdateTask(ctx, d.ID, patch); err != nil {
			return nil, updateTaskOut{}, err
		}
		return nil, updateTaskOut{TaskID: d.ID, Number: d.Number, Changed: changed}, nil
	}
}

// timeRFC3339 is the wire format the backend parses dates in.
const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

// ── set_parent ────────────────────────────────────────────────────────────────

type setParentInput struct {
	TaskID       string `json:"task_id,omitempty" jsonschema:"the task UUID to re-parent"`
	WorkspaceID  string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task and the parent"`
	Number       int64  `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	ParentID     string `json:"parent_id,omitempty" jsonschema:"the new parent's task UUID"`
	ParentNumber int64  `json:"parent_number,omitempty" jsonschema:"the new parent's #number (with workspace_id)"`
	Detach       bool   `json:"detach,omitempty" jsonschema:"detach from the current parent instead, making it a top-level task"`
}

type setParentOut struct {
	TaskID   string `json:"task_id"`
	ParentID string `json:"parent_id,omitempty"`
	Detached bool   `json:"detached,omitempty"`
}

func setParent(c *client.Client) mcp.ToolHandlerFor[setParentInput, setParentOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in setParentInput) (*mcp.CallToolResult, setParentOut, error) {
		hasParent := in.ParentID != "" || in.ParentNumber != 0
		if in.Detach == hasParent {
			return nil, setParentOut{}, fmt.Errorf("provide either a parent (parent_id / parent_number) or detach=true")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, setParentOut{}, err
		}
		var parentID *string
		if hasParent {
			parent, pErr := resolveTaskDetail(ctx, c, in.ParentID, in.WorkspaceID, in.ParentNumber)
			if pErr != nil {
				return nil, setParentOut{}, fmt.Errorf("resolve parent: %w", pErr)
			}
			parentID = &parent.ID
		}
		if err := c.SetParent(ctx, d.ID, parentID); err != nil {
			return nil, setParentOut{}, err
		}
		out := setParentOut{TaskID: d.ID, Detached: parentID == nil}
		if parentID != nil {
			out.ParentID = *parentID
		}
		return nil, out, nil
	}
}

// ── move_description ──────────────────────────────────────────────────────────

type moveDescriptionInput struct {
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"workspace UUID, used with from_number/to_number"`
	FromTaskID  string `json:"from_task_id,omitempty" jsonschema:"source task UUID"`
	FromNumber  int64  `json:"from_number,omitempty" jsonschema:"source task #number (with workspace_id)"`
	ToTaskID    string `json:"to_task_id,omitempty" jsonschema:"target task UUID"`
	ToNumber    int64  `json:"to_number,omitempty" jsonschema:"target task #number (with workspace_id)"`
	Mode        string `json:"mode,omitempty" jsonschema:"'append' (default) adds under the target's description; 'replace' overwrites it"`
	Cut         bool   `json:"cut,omitempty" jsonschema:"empty the source description afterwards; default false = copy"`
	Heading     string `json:"heading,omitempty" jsonschema:"optional heading to put above the moved text in append mode"`
}

type moveDescriptionOut struct {
	FromTaskID    string `json:"from_task_id"`
	ToTaskID      string `json:"to_task_id"`
	Mode          string `json:"mode"`
	MovedChars    int    `json:"moved_chars"`
	SourceEmptied bool   `json:"source_emptied,omitempty"`
}

func moveDescription(c *client.Client) mcp.ToolHandlerFor[moveDescriptionInput, moveDescriptionOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in moveDescriptionInput) (*mcp.CallToolResult, moveDescriptionOut, error) {
		mode := in.Mode
		if mode == "" {
			mode = "append"
		}
		if mode != "append" && mode != "replace" {
			return nil, moveDescriptionOut{}, fmt.Errorf("mode must be 'append' or 'replace'")
		}
		src, err := resolveTaskDetail(ctx, c, in.FromTaskID, in.WorkspaceID, in.FromNumber)
		if err != nil {
			return nil, moveDescriptionOut{}, fmt.Errorf("resolve source: %w", err)
		}
		dst, err := resolveTaskDetail(ctx, c, in.ToTaskID, in.WorkspaceID, in.ToNumber)
		if err != nil {
			return nil, moveDescriptionOut{}, fmt.Errorf("resolve target: %w", err)
		}
		if src.ID == dst.ID {
			return nil, moveDescriptionOut{}, fmt.Errorf("source and target are the same task")
		}
		block := strings.TrimSpace(src.Description)
		if block == "" {
			return nil, moveDescriptionOut{}, fmt.Errorf("source task has an empty description")
		}
		if in.Heading != "" {
			block = "## " + in.Heading + "\n\n" + block
		}
		desc := block
		if mode == "append" && strings.TrimSpace(dst.Description) != "" {
			desc = strings.TrimRight(dst.Description, "\n") + "\n\n---\n\n" + block
		}

		// Write the target first: if the source were cleared first and this call
		// failed, the text would be gone for good.
		if err := c.UpdateTask(ctx, dst.ID, client.TaskPatch{}.Set("description", desc)); err != nil {
			return nil, moveDescriptionOut{}, err
		}
		out := moveDescriptionOut{FromTaskID: src.ID, ToTaskID: dst.ID, Mode: mode, MovedChars: len(block)}
		if in.Cut {
			if err := c.UpdateTask(ctx, src.ID, client.TaskPatch{}.Set("description", "")); err != nil {
				return nil, out, fmt.Errorf("target updated, but clearing the source failed: %w", err)
			}
			out.SourceEmptied = true
		}
		return nil, out, nil
	}
}
