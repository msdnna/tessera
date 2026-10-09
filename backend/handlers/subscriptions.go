package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// Task subscriptions (#2963). Who is subscribed is computed, not stored: the
// author, the assignees and the commenters are in unless they opted out, anyone
// else is in once they opted in (migration 0083). notifyTaskParticipants sends
// the task's news to exactly this list.

// taskSubscribers returns the effective subscribers of a task and whether the
// current user is one of them.
func (h *API) taskSubscribers(c *gin.Context, taskID, wsID uuid.UUID) ([]db.ListTaskSubscribersRow, bool, error) {
	subs, err := h.q.ListTaskSubscribers(c, db.ListTaskSubscribersParams{TaskID: taskID, WorkspaceID: wsID})
	if err != nil {
		return nil, false, err
	}
	if subs == nil {
		subs = []db.ListTaskSubscribersRow{}
	}
	me := middleware.CurrentUser(c)
	for _, u := range subs {
		if u.ID == me {
			return subs, true, nil
		}
	}
	return subs, false, nil
}

type subscriptionReq struct {
	Subscribed *bool `json:"subscribed" binding:"required"`
}

// SetTaskSubscription — PUT /tasks/:id/subscription {subscribed}: the viewer
// follows or stops following a task. Returns the new list, and tells the
// workspace so an open task window redraws its «Подписаны» row.
func (h *API) SetTaskSubscription(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req subscriptionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subscribed is required"})
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	if err := h.q.SetTaskSubscription(c, db.SetTaskSubscriptionParams{
		TaskID: t.ID, UserID: middleware.CurrentUser(c), Subscribed: *req.Subscribed,
	}); err != nil {
		fail(c, err)
		return
	}
	subs, subscribed, err := h.taskSubscribers(c, t.ID, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "task.subscribers", gin.H{"task_id": t.ID, "subscribers": subs})
	c.JSON(http.StatusOK, gin.H{"subscribers": subs, "subscribed": subscribed})
}
