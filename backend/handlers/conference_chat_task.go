package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/confroom"
	"tessera/internal/db"
	"tessera/middleware"
)

// «/» in a call's chat creates a task without leaving the call (#3055, PackageE
// §4 «В чате / создаёт задачу прямо из встречи»).
//
// One endpoint rather than "create a task, then post a message" from the client:
// the task and the line «создал(а) задачу #N» that tells the room about it are one
// event, and two requests would leave a task nobody in the call heard of whenever
// the second one failed. It is also what keeps Android a single call away from
// the same feature.

// CreateConferenceTask creates a task on a board of the call's workspace and
// leaves a task line in the chat.
//
// The column is the board's «К работе» — the seeded todo column, else the first —
// the column every other quick capture uses. When the call is tied to a stage of
// the board's own project, the task goes into that stage too (journalled like any
// other stage change, through applyMilestone): a planning meeting's tasks belong
// to the sprint it plans.
func (h *API) CreateConferenceTask(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if conf.Status == "ended" {
		c.JSON(http.StatusConflict, gin.H{"error": "conference has ended"})
		return
	}
	uid := middleware.CurrentUser(c)
	// A kicked participant must not be able to talk to the room this way either —
	// the line lands in everyone's chat (see refuseKicked).
	if h.refuseKicked(c, conf.ID, uid) {
		return
	}
	var req struct {
		Title    string     `json:"title" binding:"required"`
		BoardID  uuid.UUID  `json:"board_id" binding:"required"`
		ColumnID *uuid.UUID `json:"column_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	title := normalizeTitle(req.Title)
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "заголовок не может быть пустым"})
		return
	}
	// The board must be a live one of the call's own workspace: a task created
	// from this call is shown to the call, and the call is shown to its workspace.
	boardWS, err := h.q.WorkspaceIDForBoard(c, req.BoardID)
	if err != nil || boardWS != conf.WorkspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board is not in this conference's workspace"})
		return
	}
	col, ok := h.chatTaskColumn(c, req.BoardID, req.ColumnID)
	if !ok {
		return
	}
	pos, err := h.nextTaskPosition(c, col, nil)
	if err != nil {
		fail(c, err)
		return
	}
	num, err := h.q.NextWorkspaceTaskNumber(c, conf.WorkspaceID)
	if err != nil {
		fail(c, err)
		return
	}
	var (
		task db.Task
		msg  db.ConferenceMessage
	)
	err = h.inTx(c, func(q *db.Queries) error {
		var err error
		task, err = q.CreateTask(c, db.CreateTaskParams{
			BoardID:   req.BoardID,
			ColumnID:  col,
			Title:     title,
			Position:  pos,
			CreatedBy: &uid,
			Number:    &num,
		})
		if err != nil {
			return err
		}
		msg, err = q.CreateConferenceTaskMessage(c, db.CreateConferenceTaskMessageParams{
			ConferenceID: conf.ID,
			UserID:       &uid,
			Body:         fmt.Sprintf("#%d %s", num, title),
			TaskID:       &task.ID,
		})
		return err
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.logEvent(c, task.ID, "created", nil)
	h.broadcastAs(c, conf.WorkspaceID, "task.created", task)
	h.stageChatTask(c, conf, task)

	out := chatMessage{
		ID: msg.ID, UserID: msg.UserID, Body: msg.Body, Kind: msg.Kind,
		CreatedAt: msg.CreatedAt, Attachments: []chatAttachment{},
	}
	if name := h.userName(c, uid); name != "" {
		out.UserName = &name
	}
	if ref, err := h.chatTaskRef(c, task); err == nil {
		out.Task = ref
	}
	h.notifyConfRoom(conf.ID, confroom.TypeChat)
	c.JSON(http.StatusCreated, gin.H{"task": task, "message": out})
}

// chatTaskColumn resolves the column a chat task lands in: the one asked for
// (it must be on the board), else the board's «К работе».
func (h *API) chatTaskColumn(c *gin.Context, boardID uuid.UUID, want *uuid.UUID) (uuid.UUID, bool) {
	if want != nil {
		col, err := h.q.GetColumn(c, *want)
		if err != nil || col.BoardID != boardID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to this board"})
			return uuid.Nil, false
		}
		return col.ID, true
	}
	cols, err := h.q.ListColumns(c, boardID)
	if err != nil {
		fail(c, err)
		return uuid.Nil, false
	}
	if len(cols) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board has no columns"})
		return uuid.Nil, false
	}
	for _, col := range cols {
		if col.NameKey != nil && *col.NameKey == "todo" {
			return col.ID, true
		}
	}
	return cols[0].ID, true
}

// stageChatTask puts a task created from a call into the call's stage, when the
// stage is of the task's own project. A stage of another project is left alone:
// a stage is a project's, and a task on a board elsewhere pinned to it would be
// counted by a roadmap it does not appear on. Best-effort — the task exists and
// the line is in the chat either way.
func (h *API) stageChatTask(c *gin.Context, conf db.Conference, task db.Task) {
	if conf.MilestoneID == nil {
		return
	}
	m, err := h.q.GetMilestone(c, *conf.MilestoneID)
	if err != nil {
		return
	}
	project, err := h.q.ProjectIDForBoard(c, task.BoardID)
	if err != nil || project != m.ProjectID {
		return
	}
	soft(c, "stageChatTask", h.applyMilestone(c, task.ID, conf.WorkspaceID, conf.MilestoneID))
}

// chatTaskRef builds the link of a task line just written.
func (h *API) chatTaskRef(c *gin.Context, task db.Task) (*chatTaskRef, error) {
	b, err := h.q.GetBoard(c, task.BoardID)
	if err != nil {
		return nil, err
	}
	p, err := h.q.GetProject(c, b.ProjectID)
	if err != nil {
		return nil, err
	}
	return &chatTaskRef{
		ID: task.ID, Number: task.Number, Title: task.Title,
		BoardSlug: b.Slug, ProjectSlug: p.Slug,
	}, nil
}
