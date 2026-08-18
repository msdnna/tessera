package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tessera-mcp/internal/client"
)

// Tag and relation tools.

// registerLinks wires the tag and relation tools onto the server.
func registerLinks(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_set_tags",
		Description: "Add and/or remove tags on a task by name. Tags are project-scoped: a name is resolved inside " +
			"the task's project, and an unknown name is an error unless create_missing=true. Remember the board can be " +
			"grouped by tag, so tagging moves the card.",
	}, setTags(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_link_tasks",
		Description: "Link two tasks: kind 'relates' (default), 'blocks', 'blocked_by' or 'duplicates'. The other task " +
			"is named by its #number. A 'blocks' link is what makes tessera_next_task skip the blocked task while the " +
			"blocker is open.",
	}, linkTasks(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_unlink_tasks",
		Description: "Remove every link between two tasks (both directions — a relation row lives on whichever task " +
			"was linked from). Reports how many rows were removed; 0 means they weren't linked.",
	}, unlinkTasks(c))
}

// ── set_tags ──────────────────────────────────────────────────────────────────

type setTagsInput struct {
	TaskID        string   `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID"`
	WorkspaceID   string   `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number        int64    `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Add           []string `json:"add,omitempty" jsonschema:"tag names to attach"`
	Remove        []string `json:"remove,omitempty" jsonschema:"tag names to detach"`
	CreateMissing bool     `json:"create_missing,omitempty" jsonschema:"create tags in 'add' that don't exist in the project yet; default false so a typo can't litter the project"`
}

type setTagsOut struct {
	TaskID  string   `json:"task_id"`
	Tags    []string `json:"tags"`
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

func setTags(c *client.Client) mcp.ToolHandlerFor[setTagsInput, setTagsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in setTagsInput) (*mcp.CallToolResult, setTagsOut, error) {
		if len(in.Add) == 0 && len(in.Remove) == 0 {
			return nil, setTagsOut{}, fmt.Errorf("provide add and/or remove")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, setTagsOut{}, err
		}
		projectID, err := projectIDForBoard(ctx, c, d.BoardID)
		if err != nil {
			return nil, setTagsOut{}, err
		}

		out := setTagsOut{TaskID: d.ID}
		if len(in.Add) > 0 {
			ids, rErr := resolveTags(ctx, c, projectID, in.Add, in.CreateMissing)
			if rErr != nil {
				return nil, setTagsOut{}, rErr
			}
			for _, id := range ids {
				if err := c.AddTaskTag(ctx, d.ID, id); err != nil {
					return nil, setTagsOut{}, err
				}
			}
			out.Added = in.Add
		}
		if len(in.Remove) > 0 {
			// Removing only makes sense for tags that exist; never create here.
			ids, rErr := resolveTags(ctx, c, projectID, in.Remove, false)
			if rErr != nil {
				return nil, setTagsOut{}, rErr
			}
			for _, id := range ids {
				if err := c.RemoveTaskTag(ctx, d.ID, id); err != nil {
					return nil, setTagsOut{}, err
				}
			}
			out.Removed = in.Remove
		}

		updated, err := c.GetTask(ctx, d.ID)
		if err != nil {
			return nil, setTagsOut{}, err
		}
		for _, tg := range updated.Tags {
			out.Tags = append(out.Tags, tg.Name)
		}
		return nil, out, nil
	}
}

// ── link_tasks ────────────────────────────────────────────────────────────────

// relationKinds are the link kinds the schema allows. The backend stores kind as
// free text, so validating here is what keeps a typo from becoming a relation
// nothing can interpret.
var relationKinds = map[string]bool{
	"relates": true, "blocks": true, "blocked_by": true, "duplicates": true,
}

type linkTasksInput struct {
	TaskID        string `json:"task_id,omitempty" jsonschema:"the task UUID to link from"`
	WorkspaceID   string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number        int64  `json:"number,omitempty" jsonschema:"the per-workspace task number to link from, e.g. 252"`
	RelatedNumber int64  `json:"related_number" jsonschema:"the #number of the task to link to"`
	Kind          string `json:"kind,omitempty" jsonschema:"relates (default) | blocks | blocked_by | duplicates"`
}

type linkTasksOut struct {
	TaskID        string `json:"task_id"`
	RelatedNumber int64  `json:"related_number"`
	Kind          string `json:"kind"`
}

func linkTasks(c *client.Client) mcp.ToolHandlerFor[linkTasksInput, linkTasksOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in linkTasksInput) (*mcp.CallToolResult, linkTasksOut, error) {
		if in.RelatedNumber == 0 {
			return nil, linkTasksOut{}, fmt.Errorf("related_number is required")
		}
		kind := strings.ToLower(strings.TrimSpace(in.Kind))
		if kind == "" {
			kind = "relates"
		}
		if !relationKinds[kind] {
			return nil, linkTasksOut{}, fmt.Errorf("kind must be one of relates, blocks, blocked_by, duplicates")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, linkTasksOut{}, err
		}
		if d.Number != nil && *d.Number == in.RelatedNumber {
			return nil, linkTasksOut{}, fmt.Errorf("cannot link a task to itself")
		}
		if err := c.AddRelation(ctx, d.ID, in.RelatedNumber, kind); err != nil {
			return nil, linkTasksOut{}, err
		}
		return nil, linkTasksOut{TaskID: d.ID, RelatedNumber: in.RelatedNumber, Kind: kind}, nil
	}
}

// ── unlink_tasks ──────────────────────────────────────────────────────────────

type unlinkTasksInput struct {
	TaskID        string `json:"task_id,omitempty" jsonschema:"one side of the link, by task UUID"`
	WorkspaceID   string `json:"workspace_id,omitempty" jsonschema:"workspace UUID, needed with number/related_number"`
	Number        int64  `json:"number,omitempty" jsonschema:"one side of the link, by #number"`
	RelatedNumber int64  `json:"related_number" jsonschema:"the other side of the link, by #number"`
}

type unlinkTasksOut struct {
	TaskID  string `json:"task_id"`
	Removed int    `json:"removed"`
	Note    string `json:"note,omitempty"`
}

func unlinkTasks(c *client.Client) mcp.ToolHandlerFor[unlinkTasksInput, unlinkTasksOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in unlinkTasksInput) (*mcp.CallToolResult, unlinkTasksOut, error) {
		if in.RelatedNumber == 0 {
			return nil, unlinkTasksOut{}, fmt.Errorf("related_number is required")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, unlinkTasksOut{}, err
		}
		wsID := in.WorkspaceID
		if wsID == "" {
			if wsID, err = workspaceIDForTask(ctx, c, d); err != nil {
				return nil, unlinkTasksOut{}, err
			}
		}
		other, err := c.GetTaskByNumber(ctx, wsID, in.RelatedNumber)
		if err != nil {
			return nil, unlinkTasksOut{}, fmt.Errorf("resolve #%d: %w", in.RelatedNumber, err)
		}

		// A relation row lives on whoever linked first, so look from both ends.
		removed := 0
		for _, side := range [2][2]string{{d.ID, other.ID}, {other.ID, d.ID}} {
			rels, lErr := c.ListRelations(ctx, side[0])
			if lErr != nil {
				return nil, unlinkTasksOut{}, lErr
			}
			for _, r := range rels {
				if r.RelatedTaskID != side[1] {
					continue
				}
				if dErr := c.DeleteRelation(ctx, r.ID); dErr != nil {
					return nil, unlinkTasksOut{}, dErr
				}
				removed++
			}
		}
		out := unlinkTasksOut{TaskID: d.ID, Removed: removed}
		if removed == 0 {
			out.Note = fmt.Sprintf("no link between this task and #%d", in.RelatedNumber)
		}
		return nil, out, nil
	}
}
