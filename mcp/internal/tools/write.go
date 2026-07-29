package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// registerWrite wires the write tools (comment, description, move) plus the
// image-reading tool and column listing onto the server.
func registerWrite(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_add_comment",
		Description: "Post a Markdown comment on a task or subtask. Optionally embed local image files " +
			"(e.g. CDP test screenshots) via image_paths — they're uploaded and appended inline. Use this " +
			"to report results, attach test evidence, or ask the user a clarifying question.",
	}, addComment(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_update_description",
		Description: "Attach Markdown to a task's description. mode 'append' (default) adds it under the " +
			"existing text (optionally under a heading); 'replace' overwrites. Good for filing a plan or a " +
			"results write-up. Other task fields (priority, dates, recurrence) are preserved.",
	}, updateDescription(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_move_task",
		Description: "Move a task to a column by name (case-insensitive, e.g. 'В процессе' to start work, " +
			"'На рассмотрении' to send it for review) or by column UUID. Moving into the board's done column " +
			"marks the task complete.",
	}, moveTask(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_assign_task",
		Description: "Set a task's assignees. Each entry is a user email, name, UUID, 'author' (the task's " +
			"creator) or 'me' (the token owner). Use replace=true to hand a task back — e.g. assignees=['author'] " +
			"reassigns it to whoever created it and drops the agent. Pair with tessera_move_task to 'На рассмотрении' " +
			"when a task needs the user's testing or review.",
	}, assignTask(c))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "tessera_list_columns",
		Description: "List a board's columns (id, name, position, whether it's the done column). Handy to see valid targets for tessera_move_task.",
	}, listColumns(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_view_image",
		Description: "Fetch and view image(s) referenced by a task — inline images in its description/comments " +
			"and image attachments. Pass a specific ref from tessera_get_task's 'images', or omit ref to view all. " +
			"Returns the images so you can actually see screenshots and diagrams.",
	}, viewImage(c))
}

// ── shared helpers ────────────────────────────────────────────────────────────

// resolveTaskID turns a (task_id | workspace_id+number) selector into a task id.
func resolveTaskID(ctx context.Context, c *client.Client, taskID, workspaceID string, number int64) (string, error) {
	if taskID != "" {
		return taskID, nil
	}
	if workspaceID != "" && number != 0 {
		d, err := c.GetTaskByNumber(ctx, workspaceID, number)
		if err != nil {
			return "", err
		}
		return d.ID, nil
	}
	return "", fmt.Errorf("provide task_id, or workspace_id + number")
}

// resolveTaskDetail is resolveTaskID plus the full task detail.
func resolveTaskDetail(ctx context.Context, c *client.Client, taskID, workspaceID string, number int64) (model.TaskDetail, error) {
	switch {
	case taskID != "":
		return c.GetTask(ctx, taskID)
	case workspaceID != "" && number != 0:
		return c.GetTaskByNumber(ctx, workspaceID, number)
	default:
		return model.TaskDetail{}, fmt.Errorf("provide task_id, or workspace_id + number")
	}
}

// cleanRecurrence normalises a stored recurrence blob: an empty or JSON-null
// value becomes nil so it's omitted from a full-replace update (which the
// backend reads as "no recurrence") instead of being sent as a literal null.
func cleanRecurrence(r json.RawMessage) json.RawMessage {
	s := strings.TrimSpace(string(r))
	if s == "" || s == "null" {
		return nil
	}
	return r
}

// commentAuthor is the best display name for a comment's author: the native
// user name, else the mirrored GitLab author, else "".
func commentAuthor(cm model.Comment) string {
	if cm.AuthorName != nil && strings.TrimSpace(*cm.AuthorName) != "" {
		return *cm.AuthorName
	}
	if cm.GlAuthorName != "" {
		return cm.GlAuthorName
	}
	return cm.GlAuthorLogin
}

// imageMDRe matches a Markdown image: ![alt](url).
var imageMDRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)

// imageRefOut is a viewable image discovered on a task. Ref is what to pass to
// tessera_view_image.
type imageRefOut struct {
	Ref    string `json:"ref"`
	Source string `json:"source"` // description | comment | attachment
	Label  string `json:"label,omitempty"`
}

// imageExts are the attachment extensions treated as viewable images.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".bmp": true,
}

// collectImages gathers viewable image refs from a task's description, comments
// and attachments, de-duplicated by ref (first source wins).
func collectImages(description string, comments []model.Comment, attachments []model.Attachment) []imageRefOut {
	var out []imageRefOut
	seen := map[string]bool{}
	add := func(ref, source, label string) {
		if ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		out = append(out, imageRefOut{Ref: ref, Source: source, Label: label})
	}
	for _, m := range imageMDRe.FindAllStringSubmatch(description, -1) {
		add(m[2], "description", m[1])
	}
	for _, cm := range comments {
		for _, m := range imageMDRe.FindAllStringSubmatch(cm.Body, -1) {
			add(m[2], "comment", m[1])
		}
	}
	for _, a := range attachments {
		if strings.HasPrefix(a.ContentType, "image/") || imageExts[strings.ToLower(filepath.Ext(a.Filename))] {
			add("attachment:"+a.ID, "attachment", a.Filename)
		}
	}
	return out
}

// ── add_comment ───────────────────────────────────────────────────────────────

type addCommentInput struct {
	TaskID      string   `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID to comment on"`
	WorkspaceID string   `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number      int64    `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Body        string   `json:"body" jsonschema:"the comment text (Markdown)"`
	ImagePaths  []string `json:"image_paths,omitempty" jsonschema:"optional local image file paths to upload and embed inline (the MCP server process must be able to read them)"`
}

type addCommentOut struct {
	CommentID      string `json:"comment_id"`
	TaskID         string `json:"task_id"`
	EmbeddedImages int    `json:"embedded_images,omitempty"`
}

func addComment(c *client.Client) mcp.ToolHandlerFor[addCommentInput, addCommentOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in addCommentInput) (*mcp.CallToolResult, addCommentOut, error) {
		if strings.TrimSpace(in.Body) == "" && len(in.ImagePaths) == 0 {
			return nil, addCommentOut{}, fmt.Errorf("body or image_paths is required")
		}
		taskID, err := resolveTaskID(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, addCommentOut{}, err
		}

		body := in.Body
		embedded := 0
		for _, p := range in.ImagePaths {
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil, addCommentOut{}, fmt.Errorf("read image %q: %w", p, rerr)
			}
			name := filepath.Base(p)
			url, uerr := c.UploadMedia(ctx, name, "", data)
			if uerr != nil {
				return nil, addCommentOut{}, fmt.Errorf("upload image %q: %w", p, uerr)
			}
			body = strings.TrimRight(body, "\n") + fmt.Sprintf("\n\n![%s](%s)", name, url)
			embedded++
		}

		cm, err := c.CreateComment(ctx, taskID, body)
		if err != nil {
			return nil, addCommentOut{}, err
		}
		return nil, addCommentOut{CommentID: cm.ID, TaskID: taskID, EmbeddedImages: embedded}, nil
	}
}

// ── update_description ────────────────────────────────────────────────────────

type updateDescriptionInput struct {
	TaskID      string `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID"`
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number      int64  `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Markdown    string `json:"markdown" jsonschema:"the Markdown to attach to the description"`
	Mode        string `json:"mode,omitempty" jsonschema:"'append' (default) adds under the existing description; 'replace' overwrites it"`
	Heading     string `json:"heading,omitempty" jsonschema:"optional section heading for append mode, e.g. 'План' or 'Результаты тестов'"`
}

type updateDescriptionOut struct {
	TaskID     string `json:"task_id"`
	Mode       string `json:"mode"`
	DescLength int    `json:"description_length"`
}

func updateDescription(c *client.Client) mcp.ToolHandlerFor[updateDescriptionInput, updateDescriptionOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateDescriptionInput) (*mcp.CallToolResult, updateDescriptionOut, error) {
		if strings.TrimSpace(in.Markdown) == "" {
			return nil, updateDescriptionOut{}, fmt.Errorf("markdown is required")
		}
		mode := in.Mode
		if mode == "" {
			mode = "append"
		}
		if mode != "append" && mode != "replace" {
			return nil, updateDescriptionOut{}, fmt.Errorf("mode must be 'append' or 'replace'")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, updateDescriptionOut{}, err
		}

		block := in.Markdown
		if in.Heading != "" {
			block = "## " + in.Heading + "\n\n" + block
		}
		desc := block
		if mode == "append" && strings.TrimSpace(d.Description) != "" {
			desc = strings.TrimRight(d.Description, "\n") + "\n\n---\n\n" + block
		}

		if err := c.UpdateTask(ctx, d.ID, client.TaskUpdate{
			Title:       d.Title,
			Description: desc,
			Priority:    d.Priority,
			DueDate:     d.DueDate,
			StartDate:   d.StartDate,
			Estimate:    d.Estimate,
			Completed:   d.CompletedAt != nil,
			Recurrence:  cleanRecurrence(d.Recurrence),
		}); err != nil {
			return nil, updateDescriptionOut{}, err
		}
		return nil, updateDescriptionOut{TaskID: d.ID, Mode: mode, DescLength: len(desc)}, nil
	}
}

// ── move_task ─────────────────────────────────────────────────────────────────

type moveTaskInput struct {
	TaskID      string `json:"task_id,omitempty" jsonschema:"the task UUID to move"`
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number      int64  `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Column      string `json:"column" jsonschema:"target column: a name (case-insensitive, e.g. 'В процессе') or a column UUID"`
}

type moveTaskOut struct {
	TaskID   string `json:"task_id"`
	Column   string `json:"column"`
	ColumnID string `json:"column_id"`
}

func moveTask(c *client.Client) mcp.ToolHandlerFor[moveTaskInput, moveTaskOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in moveTaskInput) (*mcp.CallToolResult, moveTaskOut, error) {
		if strings.TrimSpace(in.Column) == "" {
			return nil, moveTaskOut{}, fmt.Errorf("column is required")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, moveTaskOut{}, err
		}
		cols, err := c.ListColumns(ctx, d.BoardID)
		if err != nil {
			return nil, moveTaskOut{}, err
		}
		want := strings.ToLower(strings.TrimSpace(in.Column))
		var target *model.Column
		var names []string
		for i := range cols {
			names = append(names, cols[i].Name)
			if cols[i].ID == in.Column || strings.ToLower(strings.TrimSpace(cols[i].Name)) == want {
				target = &cols[i]
				break
			}
		}
		if target == nil {
			return nil, moveTaskOut{}, fmt.Errorf("no column matching %q on this board; available: %s", in.Column, strings.Join(names, ", "))
		}
		if err := c.MoveTask(ctx, d.ID, target.ID); err != nil {
			return nil, moveTaskOut{}, err
		}
		return nil, moveTaskOut{TaskID: d.ID, Column: target.Name, ColumnID: target.ID}, nil
	}
}

// ── assign_task ───────────────────────────────────────────────────────────────

// uuidRe recognises a bare UUID so an assignee ref can be used as a user id directly.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type assignTaskInput struct {
	TaskID      string   `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID"`
	WorkspaceID string   `json:"workspace_id,omitempty" jsonschema:"workspace UUID — to resolve the task by number and/or assignee names/emails (otherwise derived from the task)"`
	Number      int64    `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Assignees   []string `json:"assignees" jsonschema:"who to assign: user emails, names, UUIDs, 'author' (task creator) or 'me' (token owner)"`
	Replace     bool     `json:"replace,omitempty" jsonschema:"replace current assignees with this set (removes the rest — e.g. to hand a task back); default false = add"`
}

type assignTaskOut struct {
	TaskID    string        `json:"task_id"`
	Assignees []assigneeOut `json:"assignees"`
}

func assignTask(c *client.Client) mcp.ToolHandlerFor[assignTaskInput, assignTaskOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in assignTaskInput) (*mcp.CallToolResult, assignTaskOut, error) {
		if len(in.Assignees) == 0 {
			return nil, assignTaskOut{}, fmt.Errorf("assignees is required")
		}
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, assignTaskOut{}, err
		}

		var members []model.Member // loaded lazily, only when a name/email must be resolved
		membersLoaded := false
		loadMembers := func() error {
			if membersLoaded {
				return nil
			}
			wsID := in.WorkspaceID
			if wsID == "" {
				if wsID, err = workspaceIDForTask(ctx, c, d); err != nil {
					return err
				}
			}
			if members, err = c.ListMembers(ctx, wsID); err != nil {
				return err
			}
			membersLoaded = true
			return nil
		}

		targets := map[string]bool{}
		for _, raw := range in.Assignees {
			ref := strings.TrimSpace(raw)
			if ref == "" {
				continue
			}
			switch {
			case strings.EqualFold(ref, "me"):
				self := c.SelfID(ctx)
				if self == "" {
					return nil, assignTaskOut{}, fmt.Errorf("cannot resolve 'me': /auth/me lookup failed")
				}
				targets[self] = true
			case strings.EqualFold(ref, "author"):
				if d.CreatedBy == nil || *d.CreatedBy == "" {
					return nil, assignTaskOut{}, fmt.Errorf("task has no recorded author to assign")
				}
				targets[*d.CreatedBy] = true
			case uuidRe.MatchString(ref):
				targets[ref] = true
			default:
				if err := loadMembers(); err != nil {
					return nil, assignTaskOut{}, err
				}
				id, mErr := matchMember(ref, members)
				if mErr != nil {
					return nil, assignTaskOut{}, mErr
				}
				targets[id] = true
			}
		}

		current := map[string]bool{}
		for _, a := range d.Assignees {
			current[a.ID] = true
		}
		for id := range targets {
			if !current[id] {
				if err := c.AddAssignee(ctx, d.ID, id); err != nil {
					return nil, assignTaskOut{}, err
				}
			}
		}
		if in.Replace {
			for id := range current {
				if !targets[id] {
					if err := c.RemoveAssignee(ctx, d.ID, id); err != nil {
						return nil, assignTaskOut{}, err
					}
				}
			}
		}

		updated, err := c.GetTask(ctx, d.ID)
		if err != nil {
			return nil, assignTaskOut{}, err
		}
		out := assignTaskOut{TaskID: d.ID}
		for _, a := range updated.Assignees {
			out.Assignees = append(out.Assignees, assigneeOut{Name: a.Name, Email: a.Email})
		}
		return nil, out, nil
	}
}

// workspaceIDForTask derives a task's workspace via its board → project.
func workspaceIDForTask(ctx context.Context, c *client.Client, d model.TaskDetail) (string, error) {
	b, err := c.GetBoard(ctx, d.BoardID)
	if err != nil {
		return "", err
	}
	p, err := c.GetProject(ctx, b.ProjectID)
	if err != nil {
		return "", err
	}
	if p.WorkspaceID == "" {
		return "", fmt.Errorf("could not determine workspace for task; pass workspace_id")
	}
	return p.WorkspaceID, nil
}

// matchMember resolves an email or name (case-insensitive) to a member's user id.
func matchMember(ref string, members []model.Member) (string, error) {
	want := strings.ToLower(strings.TrimSpace(ref))
	var names []string
	for _, m := range members {
		names = append(names, m.Name)
		if strings.ToLower(m.Email) == want || strings.ToLower(strings.TrimSpace(m.Name)) == want {
			return m.UserID, nil
		}
	}
	return "", fmt.Errorf("no workspace member matching %q; members: %s", ref, strings.Join(names, ", "))
}

// ── list_columns ──────────────────────────────────────────────────────────────

type listColumnsInput struct {
	BoardID string `json:"board_id" jsonschema:"the board UUID"`
}

type columnOut struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Position float64 `json:"position"`
	IsDone   bool    `json:"is_done,omitempty"`
}
type columnListOut struct {
	Columns []columnOut `json:"columns"`
}

func listColumns(c *client.Client) mcp.ToolHandlerFor[listColumnsInput, columnListOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listColumnsInput) (*mcp.CallToolResult, columnListOut, error) {
		if in.BoardID == "" {
			return nil, columnListOut{}, fmt.Errorf("board_id is required")
		}
		cols, err := c.ListColumns(ctx, in.BoardID)
		if err != nil {
			return nil, columnListOut{}, err
		}
		doneID := ""
		if b, err := c.GetBoard(ctx, in.BoardID); err == nil && b.DoneColumnID != nil {
			doneID = *b.DoneColumnID
		}
		out := columnListOut{}
		for _, col := range cols {
			out.Columns = append(out.Columns, columnOut{
				ID: col.ID, Name: col.Name, Position: col.Position, IsDone: col.ID == doneID,
			})
		}
		return nil, out, nil
	}
}

// ── view_image ────────────────────────────────────────────────────────────────

type viewImageInput struct {
	TaskID      string `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID whose images to view"`
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number      int64  `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	Ref         string `json:"ref,omitempty" jsonschema:"a specific image ref from tessera_get_task's 'images' (a URL or 'attachment:<id>'); omit to view every image on the task"`
}

type viewImageOut struct {
	Count int      `json:"count"`
	Refs  []string `json:"refs,omitempty"`
	Note  string   `json:"note,omitempty"`
}

// maxViewImages caps how many images a single view_image (ref omitted) returns,
// so a task with many screenshots doesn't flood the context.
const maxViewImages = 8

func viewImage(c *client.Client) mcp.ToolHandlerFor[viewImageInput, viewImageOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in viewImageInput) (*mcp.CallToolResult, viewImageOut, error) {
		var refs []string
		if in.Ref != "" {
			refs = []string{in.Ref}
		} else {
			d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
			if err != nil {
				return nil, viewImageOut{}, err
			}
			comments, _ := c.ListComments(ctx, d.ID)
			attachments, _ := c.ListAttachments(ctx, d.ID)
			for _, img := range collectImages(d.Description, comments, attachments) {
				refs = append(refs, img.Ref)
			}
			if len(refs) == 0 {
				return nil, viewImageOut{Count: 0, Note: "no images found on this task"}, nil
			}
		}

		out := viewImageOut{}
		note := ""
		if len(refs) > maxViewImages {
			note = fmt.Sprintf("showing first %d of %d images; request a specific ref for the rest", maxViewImages, len(refs))
			refs = refs[:maxViewImages]
		}
		res := &mcp.CallToolResult{}
		for _, ref := range refs {
			data, mime, err := c.FetchImage(ctx, ref)
			if err != nil {
				res.Content = append(res.Content, &mcp.TextContent{Text: fmt.Sprintf("failed to load %s: %v", ref, err)})
				continue
			}
			if mime == "" {
				mime = "image/png"
			}
			res.Content = append(res.Content, &mcp.TextContent{Text: ref})
			res.Content = append(res.Content, &mcp.ImageContent{Data: data, MIMEType: mime})
			out.Refs = append(out.Refs, ref)
		}
		out.Count = len(out.Refs)
		out.Note = note
		return res, out, nil
	}
}
