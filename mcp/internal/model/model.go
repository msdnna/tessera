// Package model holds minimal DTOs matching the JSON returned by the Tessera
// REST API. Only the fields the MCP server needs are declared; unknown fields
// are ignored on decode. Kept deliberately independent of the backend Go types
// (loose coupling — the API contract is the seam), mirroring how the web and
// Android clients don't share Go structs either.
package model

import "time"

// Workspace is a top-level container the user is a member of.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Project lives in a workspace (optionally inside a project group).
type Project struct {
	ID      string  `json:"id"`
	GroupID *string `json:"group_id"`
	Name    string  `json:"name"`
	Color   string  `json:"color"`
	Slug    string  `json:"slug"`
}

// Board belongs to a project and holds columns of tasks.
type Board struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"project_id"`
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	DoneColumnID *string `json:"done_column_id"`
}

// Column is a kanban column on a board.
type Column struct {
	ID       string  `json:"id"`
	BoardID  string  `json:"board_id"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	Position float64 `json:"position"`
}

// Tag is a project-scoped label.
type Tag struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
}

// Task is the board-tasks-with-meta shape, a superset covering both the board
// listing (GET /boards/:id/tasks) and the workspace listing
// (GET /workspaces/:id/tasks). Absent fields decode to their zero value.
type Task struct {
	ID          string     `json:"id"`
	BoardID     string     `json:"board_id"`
	ColumnID    string     `json:"column_id"`
	ParentID    *string    `json:"parent_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    int        `json:"priority"`
	DueDate     *time.Time `json:"due_date"`
	StartDate   *time.Time `json:"start_date"`
	Position    float64    `json:"position"`
	CompletedAt *time.Time `json:"completed_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
	Number      *int64     `json:"number"`
	Estimate    *float64   `json:"estimate"`
	MilestoneID *string    `json:"milestone_id"`
	TagIDs      []string   `json:"tag_ids"`
	AssigneeIDs []string   `json:"assignee_ids"`
	GitlabIID   *int64     `json:"gitlab_iid"`
	GitlabURL   *string    `json:"gitlab_url"`

	// Present only on the workspace listing (joined names).
	BoardName   string `json:"board_name"`
	ProjectName string `json:"project_name"`
	ColumnName  string `json:"column_name"`
}

// Assignee is a user linked to a task.
type Assignee struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// GitlabLink is the linked GitLab issue on a task detail.
type GitlabLink struct {
	IID         int64  `json:"iid"`
	WebURL      string `json:"web_url"`
	ProjectPath string `json:"project_path"`
}

// TaskDetail is GET /tasks/:id — the task plus its tags, assignees, subtasks
// and GitLab link.
type TaskDetail struct {
	Task
	Tags      []Tag      `json:"tags"`
	Assignees []Assignee `json:"assignees"`
	Subtasks  []Task     `json:"subtasks"`
	GitLab    *GitlabLink `json:"gitlab"`
}

// Dependency is a directed link between two tasks on a board. Kind is "blocks"
// (TaskID blocks RelatedTaskID) or "blocked_by" (TaskID is blocked by RelatedTaskID).
type Dependency struct {
	TaskID        string `json:"task_id"`
	RelatedTaskID string `json:"related_task_id"`
	Kind          string `json:"kind"`
}

// Comment is a task comment.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}
