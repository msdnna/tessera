package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// The personal workspace every account is seeded with (see Register / the OAuth
// first-login path). Its caption is drawn by the client from name_key, so the
// stored Russian name is only a fallback — for old clients, e-mail and DB
// lookups. A rename clears the key (UpdateWorkspace), and from then on the name
// is the user's, in every language.
const (
	personalWorkspaceName = "Личное пространство"
	personalWorkspaceKey  = "personal"
)

// CreateWorkspace makes a new (shared) workspace owned by the caller.
func (h *API) CreateWorkspace(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	// Transactional: a workspace without its owner membership is invisible and
	// unreachable even to its creator — only removable by hand in the DB.
	var ws db.Workspace
	if err := h.inTx(c, func(q *db.Queries) error {
		var err error
		ws, err = q.CreateWorkspace(c, db.CreateWorkspaceParams{Name: req.Name, OwnerID: uid})
		if err != nil {
			return err
		}
		_, err = q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: ws.ID, UserID: uid, Role: "owner",
		})
		return err
	}); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, ws)
}

// ListWorkspaces returns workspaces the caller belongs to.
func (h *API) ListWorkspaces(c *gin.Context) {
	ws, err := h.q.ListWorkspacesForUser(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// GetWorkspace returns a single workspace the caller belongs to.
func (h *API) GetWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// UpdateWorkspace renames a workspace (owner or admin).
func (h *API) UpdateWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ws, err := h.q.UpdateWorkspace(c, db.UpdateWorkspaceParams{ID: id, Name: req.Name})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(id, "workspace.updated", ws)
	c.JSON(http.StatusOK, ws)
}

// maxIconLen caps a workspace icon: an emoji, a glyph name or a few letters.
const maxIconLen = 64

// SetWorkspaceAppearance sets the colour and icon of the workspace's tile on
// the rail (#2955). Owners and admins only, like a rename: every member sees it.
func (h *API) SetWorkspaceAppearance(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	var req struct {
		Color    string `json:"color"`
		Icon     string `json:"icon"`
		IconMode string `json:"icon_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Icon) > maxIconLen || len(req.Color) > maxIconLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "icon or color too long"})
		return
	}
	ws, err := h.q.SetWorkspaceAppearance(c, db.SetWorkspaceAppearanceParams{
		ID: id, Color: req.Color, Icon: req.Icon, IconMode: normIconMode(req.IconMode),
	})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(id, "workspace.updated", ws)
	c.JSON(http.StatusOK, ws)
}

// DeleteWorkspace removes a workspace (owner only).
func (h *API) DeleteWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if ws.OwnerID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can delete the workspace"})
		return
	}
	// The widest of the four container deletes — three cascades from here to a
	// task — and the only one after which nobody is left to notice the files.
	//
	// Tasks are not the only thing on disk a workspace owns, though. Documents and
	// conferences hang off it by ON DELETE CASCADE too, and each keeps its bytes
	// somewhere no row will name once the cascade has run: a document's images are
	// pointed at by nothing but the body leaving with them, a recording by its
	// file_path, a chat upload by its attachment row. The per-object deletes all
	// know this (purgeDocAssets, and the path read DeleteConference does before its
	// own delete, #2877 — "the largest thing this application ever writes to
	// disk"); deleting the whole workspace owes the same to every one of them at
	// once, on the very gesture an owner makes to get the space back.
	var (
		gone  []uuid.UUID
		docs  []uuid.UUID
		calls []doomedConference
		derr  error
	)
	if err := h.inTx(c, func(q *db.Queries) error {
		if gone, derr = q.TaskIDsInWorkspace(c, id); derr != nil {
			return derr
		}
		if docs, derr = q.DocumentIDsInWorkspace(c, id); derr != nil {
			return derr
		}
		if calls, derr = h.doomedConferences(c, q, id); derr != nil {
			return derr
		}
		return q.DeleteWorkspace(c, id)
	}); err != nil {
		fail(c, err)
		return
	}
	h.purgeTaskUploads(gone)
	h.purgeDocAssets(c, docs)
	// The same ids also name live rooms, and the cascade is as invisible to them
	// as it is to the files: purgeConferences below ends the calls it sweeps
	// ("workspace deleted"), and a document open in someone's tab is owed the
	// same. Without it the space is gone while its documents still look editable.
	h.dropDocRooms(docs)
	h.purgeConferences(calls)
	c.Status(http.StatusNoContent)
}

// ListMembers lists the members of a workspace.
func (h *API) ListMembers(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	members, err := h.q.ListMembers(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

// manageableRole reports whether a role string is one a manager (owner/admin)
// may grant via add/update. Ownership (owner) is transferred separately, not
// assigned here.
func manageableRole(role string) bool {
	return role == "admin" || role == "member"
}

// The self-lockout guards. A manager may do anything to anybody's membership
// except their own, for the same reason admin.go refuses "cannot change your own
// admin role" and users.go refuses "cannot change your own active state": every
// other refusal on these routes protects someone who can still complain, while
// this one protects the only person whose mistake nobody will notice and who
// cannot undo it. An admin who picks «Участник» on their own row loses members,
// roles, invitations and settings on the next request, and getting them back
// needs another admin — or the owner, who may not be around.
//
// Removal has its own sentence rather than sharing the role one because it is a
// different loss (the workspace disappears from the sidebar mid-session:
// RemoveMember cuts the member's live sockets), and because it is not a way out
// that is being taken away — RemoveMember sits behind requireManager, so an
// ordinary member never had it, and there is no leave-workspace route to speak
// of. Both are checked *after* the owner guard, so the owner keeps hearing the
// owner rule, which is true regardless of who is asking.
const (
	errSelfRole   = "cannot change your own role"
	errSelfRemove = "cannot remove yourself from the workspace"
)

// AddMember invites an existing user (by email) to the workspace (owner/admin).
func (h *API) AddMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	if !manageableRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or member"})
		return
	}
	user, err := h.q.GetUserByEmail(c, strings.ToLower(req.Email))
	if notFoundMsg(c, err, "no user with that email") {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	// Adding an existing member is an upsert, so this route carries the same role
	// change as UpdateMemberRole — and must refuse the owner for the same reason,
	// or an admin takes the workspace by re-adding its owner as a plain member.
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if user.ID == ws.OwnerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot change the workspace owner's role"})
		return
	}
	if user.ID == middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": errSelfRole})
		return
	}
	m, err := h.q.CreateMembership(c, db.CreateMembershipParams{WorkspaceID: id, UserID: user.ID, Role: role})
	if err != nil {
		fail(c, err)
		return
	}
	// Live sockets snapshot their workspace set at connect; drop the new
	// member's so they reconnect and start receiving this workspace's events.
	// This route is an upsert, so it is also a role change — and carries the same
	// duty to the live calls as UpdateMemberRole below: re-adding an admin as a
	// plain member must take their moderation with it.
	h.hub.DropUser(user.ID)
	h.syncRoomModeration(c, id, user.ID, role)
	c.JSON(http.StatusCreated, m)
}

// UpdateMemberRole changes a member's role (owner/admin). The workspace owner's
// role is immutable here — ownership transfer is a separate, owner-only flow.
func (h *API) UpdateMemberRole(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !manageableRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or member"})
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if userID == ws.OwnerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot change the workspace owner's role"})
		return
	}
	if userID == middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": errSelfRole})
		return
	}
	m, err := h.q.UpdateMembershipRole(c, db.UpdateMembershipRoleParams{WorkspaceID: id, UserID: userID, Role: req.Role})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	// A role change is a membership change, and Hub.DropUser is how one reaches a
	// client that is already connected: membership is snapshotted at the upgrade,
	// so the socket has to be closed for the set — and the role inside it — to be
	// re-read on reconnect. Every other door here already does it (AddMember, which
	// is this same change by upsert; AcceptInvitation; RemoveMember via
	// revokeUserSockets); this one did not, so a member promoted to admin kept a
	// members dialog with no invite form in it until they reloaded the page, and a
	// demoted admin kept one whose every control answers 403 (#2932).
	//
	// Not revokeUserSockets: the membership itself still holds, and the document
	// and conference rooms authorise on that — evicting them would throw a member
	// out of the call they are in over a change that never took their seat away.
	//
	// The moderation right inside those calls *is* taken away by a demotion,
	// though, and it is not re-read from the membership: conferenceSeat copies the
	// workspace role into the seat at the upgrade. Closing the board socket does
	// nothing for it, so a demoted admin went on kicking and force-muting the
	// meeting they were already in. syncRoomModeration is the narrow revocation
	// that case needs — and it grants as well as takes, so a promotion reaches a
	// call in progress instead of waiting for the next join.
	h.hub.DropUser(userID)
	h.syncRoomModeration(c, id, userID, req.Role)
	c.JSON(http.StatusOK, m)
}

// RemoveMember removes a member from the workspace (owner/admin). The workspace
// owner cannot be removed.
func (h *API) RemoveMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if userID == ws.OwnerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove the workspace owner"})
		return
	}
	if userID == middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": errSelfRemove})
		return
	}
	if err := h.q.DeleteMembership(c, db.DeleteMembershipParams{WorkspaceID: id, UserID: userID}); err != nil {
		fail(c, err)
		return
	}
	// Cut the removed member's live sockets — the board stream, but also the
	// document and conference rooms, which authorise once at the upgrade and
	// would otherwise outlive the membership for as long as the tab stays open.
	h.revokeUserSockets(userID, "access revoked")
	c.Status(http.StatusNoContent)
}
