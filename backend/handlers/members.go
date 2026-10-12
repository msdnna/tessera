package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// Members v2 (#3063): ownership transfer, leaving a workspace, access requests
// and member groups. The rights model stays the three roles of memberships.role
// (owner | admin | member); nothing here adds a fourth or widens what one may do:
//
//   - the transfer is the owner's alone and hands over exactly one role — the
//     old owner becomes an admin in the same transaction, so a workspace never
//     has zero or two owners;
//   - leaving is open to everyone but the owner (who must transfer first), so a
//     workspace can't be left without one;
//   - approving an access request creates a membership with a manageable role
//     (admin | member — never owner), and only for someone who is not a member
//     yet: an old request can't be used to change an existing member's role;
//   - groups are names for sets of members. They grant nothing; whoever is
//     assigned or mentioned through one is a member already.

// statusErr carries an HTTP refusal out of a transaction body.
type statusErr struct {
	status int
	msg    string
}

func (e statusErr) Error() string { return e.msg }

// writeTxErr renders a statusErr as its refusal and anything else as a 500.
func writeTxErr(c *gin.Context, err error) {
	var se statusErr
	if errors.As(err, &se) {
		c.JSON(se.status, gin.H{"error": se.msg})
		return
	}
	fail(c, err)
}

// ── ownership transfer (#3072) ─────────────────────────────────

// TransferOwnership hands the workspace to another member (owner only). The
// new owner must already be a member; the old one stays on as an admin. The
// confirmation (typing the workspace name) is the client's: it guards against a
// slip of the hand, while the guard against everyone else is the owner check
// here.
func (h *API) TransferOwnership(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	me := middleware.CurrentUser(c)
	if err := h.inTx(c, func(q *db.Queries) error {
		owner, err := q.LockWorkspaceOwner(c, id)
		if err != nil {
			return err
		}
		if owner != me {
			return statusErr{http.StatusForbidden, "only the owner can transfer the workspace"}
		}
		if req.UserID == me {
			return statusErr{http.StatusBadRequest, "you already own the workspace"}
		}
		if _, err := q.GetMembership(c, db.GetMembershipParams{WorkspaceID: id, UserID: req.UserID}); err != nil {
			if errIsNoRows(err) {
				return statusErr{http.StatusBadRequest, "the new owner must be a member of the workspace"}
			}
			return err
		}
		if _, err := q.UpdateMembershipRole(c, db.UpdateMembershipRoleParams{WorkspaceID: id, UserID: me, Role: "admin"}); err != nil {
			return err
		}
		if _, err := q.UpdateMembershipRole(c, db.UpdateMembershipRoleParams{WorkspaceID: id, UserID: req.UserID, Role: "owner"}); err != nil {
			return err
		}
		return q.SetWorkspaceOwner(c, db.SetWorkspaceOwnerParams{ID: id, OwnerID: req.UserID})
	}); err != nil {
		writeTxErr(c, err)
		return
	}
	// Both roles changed: the sockets re-read them on reconnect, the live calls
	// get the moderation right that goes with each (see UpdateMemberRole).
	for uid, role := range map[uuid.UUID]string{me: "admin", req.UserID: "owner"} {
		h.hub.DropUser(uid)
		h.syncRoomModeration(c, id, uid, role)
	}
	ws, err := h.q.GetWorkspace(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	h.notify(c, req.UserID, id, nil, kindOwnership, msgOwnershipTransferred(h.actorName(c), ws.Name, ws.NameKey))
	h.broadcastAs(c, id, "members.changed", gin.H{"workspace_id": id})
	c.JSON(http.StatusOK, ws)
}

// LeaveWorkspace takes the caller out of a workspace. The owner can't leave —
// a workspace without an owner has nobody to delete it or to hand it on — and
// is told to transfer it first.
func (h *API) LeaveWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	me := middleware.CurrentUser(c)
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if ws.OwnerID == me || h.memberRole(c, id) == "owner" {
		c.JSON(http.StatusForbidden, gin.H{"error": "the owner cannot leave the workspace — transfer it first"})
		return
	}
	if err := h.inTx(c, func(q *db.Queries) error {
		if err := q.DropUserFromWorkspaceGroups(c, db.DropUserFromWorkspaceGroupsParams{WorkspaceID: id, UserID: me}); err != nil {
			return err
		}
		return q.DeleteMembership(c, db.DeleteMembershipParams{WorkspaceID: id, UserID: me})
	}); err != nil {
		fail(c, err)
		return
	}
	h.revokeUserSockets(me, "left the workspace")
	h.broadcastAs(c, id, "members.changed", gin.H{"workspace_id": id})
	c.Status(http.StatusNoContent)
}

// ── access requests (#3077) ────────────────────────────────────

// accessRequestCooldown is how long a declined request keeps its author from
// asking the same workspace again.
const accessRequestCooldown = 24 * time.Hour

// maxAccessMessage caps the note a requester may attach.
const maxAccessMessage = 500

// RequestAccess files a request to join a workspace the caller is not in. The
// workspace is named directly (a link to the space) or through a board the
// caller has a link to. The answer says nothing about the workspace — not its
// name, not who runs it: the requester learns those when they are let in.
func (h *API) RequestAccess(c *gin.Context) {
	var req struct {
		WorkspaceID *uuid.UUID `json:"workspace_id"`
		BoardID     *uuid.UUID `json:"board_id"`
		ProjectSlug string     `json:"project_slug"`
		BoardSlug   string     `json:"board_slug"`
		Message     string     `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	msg := strings.TrimSpace(req.Message)
	if utf8.RuneCountInString(msg) > maxAccessMessage {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message too long"})
		return
	}
	wsID, boardID, ok := h.accessTarget(c, req.WorkspaceID, req.BoardID, req.ProjectSlug, req.BoardSlug)
	if !ok {
		return
	}
	me := middleware.CurrentUser(c)
	if h.lookupMembership(c, wsID).ok {
		c.JSON(http.StatusConflict, gin.H{"error": "already a member of this workspace"})
		return
	}
	if _, err := h.q.GetPendingAccessRequest(c, db.GetPendingAccessRequestParams{WorkspaceID: wsID, UserID: me}); err == nil {
		c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
		return
	} else if !errIsNoRows(err) {
		fail(c, err)
		return
	}
	if at, err := h.q.LastDeclinedAccessRequestAt(c, db.LastDeclinedAccessRequestAtParams{WorkspaceID: wsID, UserID: me}); err == nil {
		if at != nil && time.Since(*at) < accessRequestCooldown {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "access request declined recently"})
			return
		}
	} else if !errIsNoRows(err) {
		fail(c, err)
		return
	}
	r, err := h.q.CreateAccessRequest(c, db.CreateAccessRequestParams{
		WorkspaceID: wsID, UserID: me, BoardID: boardID, Message: msg,
	})
	if isUniqueViolation(err) {
		// A double click raced the pending check — the other one landed.
		c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	who := h.actorName(c)
	if managers, err := h.q.ListWorkspaceManagerIDs(c, wsID); err == nil {
		for _, m := range managers {
			h.deliverNotification(c, m, wsID, nil, &me, kindAccessRequest, msgAccessRequested(who, r.ID.String()))
		}
	}
	h.broadcast(wsID, "access_requests.changed", gin.H{"workspace_id": wsID})
	c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
}

// accessTarget resolves what a request is for. ok=false means the response is
// written. Boards that were deleted are as missing as boards that never were.
func (h *API) accessTarget(c *gin.Context, wsID, boardID *uuid.UUID, projectSlug, boardSlug string) (uuid.UUID, *uuid.UUID, bool) {
	switch {
	case wsID != nil:
		if _, err := h.q.GetWorkspace(c, *wsID); err != nil {
			if notFound(c, err) {
				return uuid.Nil, nil, false
			}
			fail(c, err)
			return uuid.Nil, nil, false
		}
		return *wsID, nil, true
	case boardID != nil || (projectSlug != "" && boardSlug != ""):
		var b db.Board
		var err error
		if boardID != nil {
			b, err = h.q.GetBoard(c, *boardID)
		} else {
			var p db.Project
			if p, err = h.q.GetProjectBySlug(c, projectSlug); err == nil {
				b, err = h.q.GetBoardInProjectBySlug(c, db.GetBoardInProjectBySlugParams{ProjectID: p.ID, Slug: boardSlug})
			}
		}
		if err == nil && b.DeletedAt != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return uuid.Nil, nil, false
		}
		if notFound(c, err) {
			return uuid.Nil, nil, false
		}
		if err != nil {
			fail(c, err)
			return uuid.Nil, nil, false
		}
		ws, err := h.q.WorkspaceIDForBoard(c, b.ID)
		if notFound(c, err) {
			return uuid.Nil, nil, false
		}
		if err != nil {
			fail(c, err)
			return uuid.Nil, nil, false
		}
		return ws, &b.ID, true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id or a board is required"})
	return uuid.Nil, nil, false
}

// ListAccessRequests is the managers' queue of pending requests.
func (h *API) ListAccessRequests(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	items, err := h.q.ListPendingAccessRequests(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(items))
}

// loadAccessRequest reads a request of this workspace for a manager's decision.
func (h *API) loadAccessRequest(c *gin.Context) (uuid.UUID, db.WorkspaceAccessRequest, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	if !h.requireManager(c, id) {
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	reqID, ok := parseID(c, "reqId")
	if !ok {
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	r, err := h.q.GetAccessRequest(c, reqID)
	// A request of another workspace reads as missing: the manager rights are
	// for this one.
	if err == nil && r.WorkspaceID != id {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	if notFound(c, err) {
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	if err != nil {
		fail(c, err)
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	if r.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "the request is already decided"})
		return uuid.Nil, db.WorkspaceAccessRequest{}, false
	}
	return id, r, true
}

// ApproveAccessRequest lets the requester in with the role given (member by
// default, admin allowed — what AddMember may grant, never owner). Someone who
// became a member meanwhile keeps the role they have: approving a stale request
// is not a way to change a member's role.
func (h *API) ApproveAccessRequest(c *gin.Context) {
	wsID, r, ok := h.loadAccessRequest(c)
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	if !manageableRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or member"})
		return
	}
	me := middleware.CurrentUser(c)
	if err := h.inTx(c, func(q *db.Queries) error {
		if _, err := q.DecideAccessRequest(c, db.DecideAccessRequestParams{ID: r.ID, Status: "approved", DecidedBy: &me}); err != nil {
			if errIsNoRows(err) {
				return statusErr{http.StatusConflict, "the request is already decided"}
			}
			return err
		}
		if _, err := q.GetMembership(c, db.GetMembershipParams{WorkspaceID: wsID, UserID: r.UserID}); err == nil {
			return nil // already in — the role they hold stands
		} else if !errIsNoRows(err) {
			return err
		}
		_, err := q.CreateMembership(c, db.CreateMembershipParams{WorkspaceID: wsID, UserID: r.UserID, Role: role})
		return err
	}); err != nil {
		writeTxErr(c, err)
		return
	}
	// The new member's sockets snapshot their workspaces at connect.
	h.hub.DropUser(r.UserID)
	if ws, err := h.q.GetWorkspace(c, wsID); err == nil {
		h.notify(c, r.UserID, wsID, nil, kindAccessRequest, msgAccessApproved(h.actorName(c), ws.Name, ws.NameKey, wsID.String()))
	}
	h.broadcast(wsID, "access_requests.changed", gin.H{"workspace_id": wsID})
	h.broadcastAs(c, wsID, "members.changed", gin.H{"workspace_id": wsID})
	c.Status(http.StatusNoContent)
}

// DeclineAccessRequest closes a request without letting its author in. The
// author is told — without the workspace's name, which a refusal must not hand
// out — and may ask again after accessRequestCooldown.
func (h *API) DeclineAccessRequest(c *gin.Context) {
	wsID, r, ok := h.loadAccessRequest(c)
	if !ok {
		return
	}
	me := middleware.CurrentUser(c)
	if _, err := h.q.DecideAccessRequest(c, db.DecideAccessRequestParams{ID: r.ID, Status: "declined", DecidedBy: &me}); err != nil {
		if errIsNoRows(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "the request is already decided"})
			return
		}
		fail(c, err)
		return
	}
	// uuid.Nil: the author is not a member, and the notification belongs to no
	// workspace they could open.
	h.deliverNotification(c, r.UserID, uuid.Nil, nil, nil, kindAccessRequest, msgAccessDeclined())
	h.broadcast(wsID, "access_requests.changed", gin.H{"workspace_id": wsID})
	c.Status(http.StatusNoContent)
}

// ── member groups (#3074) ──────────────────────────────────────

const maxGroupName = 64

// memberGroupDTO is a group with its members.
type memberGroupDTO struct {
	ID          uuid.UUID   `json:"id"`
	WorkspaceID uuid.UUID   `json:"workspace_id"`
	Name        string      `json:"name"`
	Color       string      `json:"color"`
	MemberIDs   []uuid.UUID `json:"member_ids"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// ListMemberGroups lists a workspace's groups (any member: they assign and
// mention through them).
func (h *API) ListMemberGroups(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	out, err := h.memberGroups(c, h.q, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *API) memberGroups(c *gin.Context, q *db.Queries, wsID uuid.UUID) ([]memberGroupDTO, error) {
	rows, err := q.ListMemberGroups(c, wsID)
	if err != nil {
		return nil, err
	}
	out := make([]memberGroupDTO, 0, len(rows))
	for _, r := range rows {
		ids := r.MemberIds
		if ids == nil {
			ids = []uuid.UUID{}
		}
		out = append(out, memberGroupDTO{
			ID: r.ID, WorkspaceID: r.WorkspaceID, Name: r.Name, Color: r.Color,
			MemberIDs: ids, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// groupOut returns the one group from the workspace's list (the list is where
// member_ids is computed).
func (h *API) groupOut(c *gin.Context, status int, wsID, groupID uuid.UUID) {
	all, err := h.memberGroups(c, h.q, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	for _, g := range all {
		if g.ID == groupID {
			c.JSON(status, g)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}

// cleanGroupName validates a group name; ok=false means the response is written.
func cleanGroupName(c *gin.Context, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxGroupName {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group name must be 1–64 characters"})
		return "", false
	}
	return name, true
}

// setGroupMembers replaces a group's members, refusing a list that names
// anyone outside the workspace (AddMemberGroupMembers filters them as well).
func setGroupMembers(c *gin.Context, q *db.Queries, wsID, groupID uuid.UUID, ids []uuid.UUID) error {
	uniq := make([]uuid.UUID, 0, len(ids))
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	n, err := q.CountWorkspaceMembersIn(c, db.CountWorkspaceMembersInParams{WorkspaceID: wsID, UserIds: uniq})
	if err != nil {
		return err
	}
	if int(n) != len(uniq) {
		return statusErr{http.StatusBadRequest, "every group member must be a member of the workspace"}
	}
	if err := q.ClearMemberGroupMembers(c, groupID); err != nil {
		return err
	}
	if len(uniq) == 0 {
		return nil
	}
	return q.AddMemberGroupMembers(c, db.AddMemberGroupMembersParams{ID: groupID, UserIds: uniq})
}

// CreateMemberGroup adds a group (owner/admin).
func (h *API) CreateMemberGroup(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	var req struct {
		Name      string      `json:"name" binding:"required"`
		Color     string      `json:"color"`
		MemberIDs []uuid.UUID `json:"member_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, ok := cleanGroupName(c, req.Name)
	if !ok {
		return
	}
	if len(req.Color) > maxIconLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "color too long"})
		return
	}
	var g db.MemberGroup
	err := h.inTx(c, func(q *db.Queries) error {
		var err error
		if g, err = q.CreateMemberGroup(c, db.CreateMemberGroupParams{WorkspaceID: id, Name: name, Color: req.Color}); err != nil {
			return err
		}
		return setGroupMembers(c, q, id, g.ID, req.MemberIDs)
	})
	if isUniqueViolation(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "a group with this name already exists"})
		return
	}
	if err != nil {
		writeTxErr(c, err)
		return
	}
	h.broadcastAs(c, id, "member_groups.changed", gin.H{"workspace_id": id})
	h.groupOut(c, http.StatusCreated, id, g.ID)
}

// loadMemberGroup resolves /member-groups/:id for a manager of its workspace.
func (h *API) loadMemberGroup(c *gin.Context) (db.MemberGroup, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.MemberGroup{}, false
	}
	g, err := h.q.GetMemberGroup(c, id)
	if notFound(c, err) {
		return db.MemberGroup{}, false
	}
	if err != nil {
		fail(c, err)
		return db.MemberGroup{}, false
	}
	if !h.requireManager(c, g.WorkspaceID) {
		return db.MemberGroup{}, false
	}
	return g, true
}

// UpdateMemberGroup renames/recolours a group and/or replaces its members
// (owner/admin). Absent fields are kept.
func (h *API) UpdateMemberGroup(c *gin.Context) {
	g, ok := h.loadMemberGroup(c)
	if !ok {
		return
	}
	var req struct {
		Name      *string      `json:"name"`
		Color     *string      `json:"color"`
		MemberIDs *[]uuid.UUID `json:"member_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, color := g.Name, g.Color
	if req.Name != nil {
		if name, ok = cleanGroupName(c, *req.Name); !ok {
			return
		}
	}
	if req.Color != nil {
		if len(*req.Color) > maxIconLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "color too long"})
			return
		}
		color = *req.Color
	}
	err := h.inTx(c, func(q *db.Queries) error {
		if _, err := q.UpdateMemberGroup(c, db.UpdateMemberGroupParams{ID: g.ID, Name: name, Color: color}); err != nil {
			return err
		}
		if req.MemberIDs != nil {
			return setGroupMembers(c, q, g.WorkspaceID, g.ID, *req.MemberIDs)
		}
		return nil
	})
	if isUniqueViolation(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "a group with this name already exists"})
		return
	}
	if err != nil {
		writeTxErr(c, err)
		return
	}
	h.broadcastAs(c, g.WorkspaceID, "member_groups.changed", gin.H{"workspace_id": g.WorkspaceID})
	h.groupOut(c, http.StatusOK, g.WorkspaceID, g.ID)
}

// DeleteMemberGroup removes a group (owner/admin). Its members stay members;
// nothing assigned through it changes (assignment went to the people).
func (h *API) DeleteMemberGroup(c *gin.Context) {
	g, ok := h.loadMemberGroup(c)
	if !ok {
		return
	}
	if err := h.q.DeleteMemberGroup(c, g.ID); err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, g.WorkspaceID, "member_groups.changed", gin.H{"workspace_id": g.WorkspaceID})
	c.Status(http.StatusNoContent)
}
