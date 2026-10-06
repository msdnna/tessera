package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/middleware"
)

const (
	// maxDocApprovalTitle bounds the route's name. It is a heading in the panel
	// and a line in the document's protocol list — one line, not a paragraph.
	maxDocApprovalTitle = 200

	// maxDocApprovalComment bounds an approver's remark. Long-form objections
	// belong in the document's own threads (D5), which is where a discussion can
	// be replied to and resolved; this field is the "почему" next to a signature.
	maxDocApprovalComment = 2000

	// maxDocApprovalSteps caps how many people one route may ask. A route is a
	// list somebody has to walk in order; past a couple of dozen it is not a
	// согласование but a mailing list, and in sequential mode every extra name is
	// another place the whole thing can stall.
	maxDocApprovalSteps = 25
)

// Approval and step statuses. Kept in one place because the CHECK constraints in
// 0060 spell out exactly these strings, and a typo here becomes a 500 at write
// time rather than a compile error.
const (
	approvalPending   = "pending"
	approvalApproved  = "approved"
	approvalRejected  = "rejected"
	approvalCancelled = "cancelled"

	approvalModeSequential = "sequential"
	approvalModeParallel   = "parallel"
)

// Refusals raised from inside a transaction, where writing the response on the
// spot would mean answering before the rollback. Each maps to one status code
// at the call site.
var (
	errApprovalOpen   = errors.New("document already has an open approval")
	errApprovalClosed = errors.New("approval is already closed")
	errNotOnRoute     = errors.New("caller is not on this approval route")
	errAlreadyDecided = errors.New("caller has already decided")
	errNotYourTurn    = errors.New("an earlier approver has not signed yet")
)

// documentApprovalView is one protocol with its route. The steps are nested
// rather than shipped as a second flat list (the shape comments use) because a
// protocol *is* its route: the panel never renders a step outside the approval it
// belongs to, and a client that had to join the two could render a half-built
// one.
type documentApprovalView struct {
	db.ListDocumentApprovalsRow
	Steps []db.ListDocumentApprovalStepsByDocumentRow `json:"steps"`
}

// ListDocumentApprovals returns the document's protocols, newest first, each
// with its route.
func (h *API) ListDocumentApprovals(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	rows, err := h.q.ListDocumentApprovals(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	steps, err := h.q.ListDocumentApprovalStepsByDocument(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	byApproval := make(map[uuid.UUID][]db.ListDocumentApprovalStepsByDocumentRow, len(rows))
	for _, s := range steps {
		byApproval[s.ApprovalID] = append(byApproval[s.ApprovalID], s)
	}
	out := make([]documentApprovalView, 0, len(rows))
	for _, a := range rows {
		v := documentApprovalView{ListDocumentApprovalsRow: a, Steps: byApproval[a.ID]}
		if v.Steps == nil {
			v.Steps = []db.ListDocumentApprovalStepsByDocumentRow{}
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// CreateDocumentApproval raises a route against the document as it stands.
//
// The snapshot is taken here, unconditionally, and marked manual. That is the
// endpoint's real work: an approval has to name the exact text that was agreed,
// and pinning a manual version is what makes that text both immutable and
// exempt from retention — the FK from document_approvals is RESTRICT, so a
// prunable snapshot would eventually make pruning the document's history fail.
// The extra journal entry is not waste either: "отправлен на согласование" is one
// of the things that happened to the document.
func (h *API) CreateDocumentApproval(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		Title     string      `json:"title"`
		Mode      string      `json:"mode"`
		Approvers []uuid.UUID `json:"approvers" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = approvalModeSequential
	}
	if mode != approvalModeSequential && mode != approvalModeParallel {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown approval mode"})
		return
	}
	approvers := dedupeUUIDs(req.Approvers)
	if len(approvers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "route needs at least one approver"})
		return
	}
	if len(approvers) > maxDocApprovalSteps {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many approvers"})
		return
	}
	// Every approver must be a member: the route is a list of people asked to
	// read the document, and someone who cannot open it can only ever stall it.
	// One read of the member list serves both the check and the names, so a route
	// of twenty does not cost forty queries.
	members, err := h.q.ListMembers(c, doc.WorkspaceID)
	if err != nil {
		fail(c, err)
		return
	}
	memberNames := make(map[uuid.UUID]string, len(members))
	for _, m := range members {
		memberNames[m.UserID] = m.Name
	}
	names := make([]string, len(approvers))
	for i, a := range approvers {
		name, member := memberNames[a]
		if !member {
			c.JSON(http.StatusBadRequest, gin.H{"error": "approver is not a member of this workspace"})
			return
		}
		names[i] = name
	}
	uid := middleware.CurrentUser(c)
	title := truncateRunes(req.Title, maxDocApprovalTitle)
	label := "На согласование"
	if title != "" {
		label = fmt.Sprintf("На согласование: %s", title)
	}
	// Snapshot, route and steps are written together. A failure partway through
	// would leave a route short of an approver, and in sequential mode such a
	// route stalls forever: the missing step is the one everyone behind it waits
	// on, and there is no endpoint that adds one to a route already raised.
	//
	// The "one open route" check lives inside the same transaction, behind a lock
	// on the document: it is a read that decides an insert, so outside one two
	// simultaneous requests both read "none open" and both write.
	var approval db.DocumentApproval
	if err := h.inTx(c, func(q *db.Queries) error {
		if _, err := q.LockDocumentApprovalSlot(c, doc.ID); err != nil {
			return err
		}
		// One open route per document. Two would be collecting signatures on two
		// different revisions at the same time, and "документ согласован" would stop
		// having a single answer — including for the badge in the documents list.
		if _, err := q.PendingDocumentApproval(c, doc.ID); err == nil {
			return errApprovalOpen
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		version, err := q.CreateDocumentVersion(c, db.CreateDocumentVersionParams{
			DocumentID: doc.ID,
			AuthorID:   &uid,
			Title:      doc.Title,
			Content:    doc.Content,
			Preview:    doc.Preview,
			Label:      truncateRunes(label, maxDocVersionLabel),
			Manual:     true,
		})
		if err != nil {
			return err
		}
		approval, err = q.CreateDocumentApproval(c, db.CreateDocumentApprovalParams{
			DocumentID: doc.ID,
			VersionID:  version.ID,
			Title:      title,
			Mode:       mode,
			CreatedBy:  &uid,
		})
		if err != nil {
			return err
		}
		for i, a := range approvers {
			approver := a
			if _, err := q.CreateDocumentApprovalStep(c, db.CreateDocumentApprovalStepParams{
				ApprovalID:   approval.ID,
				ApproverID:   &approver,
				ApproverName: names[i],
				Position:     int32(i),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if errors.Is(err, errApprovalOpen) {
			c.JSON(http.StatusConflict, gin.H{"error": errApprovalOpen.Error()})
			return
		}
		fail(c, err)
		return
	}
	// Raising a route pins a manual snapshot. After the commit, so the route row
	// exists and the version it pins is protected from the prune (#2932).
	h.pruneManualVersions(c, doc.ID)
	h.notifyDocLinks(doc.ID)
	c.JSON(http.StatusCreated, approval)
}

// DecideDocumentApproval records one approver's signature.
//
// Rejection closes the whole route rather than merely marking one step: the
// point of согласование is that the text goes back for changes as soon as
// somebody objects, and leaving the remaining approvers to sign a document that
// is already going to be rewritten wastes their reading, not just their time.
func (h *API) DecideDocumentApproval(c *gin.Context) {
	approval, doc, ok := h.loadDocumentApproval(c)
	if !ok {
		return
	}
	var req struct {
		Decision  string `json:"decision" binding:"required"`
		Comment   string `json:"comment"`
		Signature string `json:"signature"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Decision != approvalApproved && req.Decision != approvalRejected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decision must be approved or rejected"})
		return
	}
	uid := middleware.CurrentUser(c)
	// Signing is a read-modify-write over the whole route — the step list decides
	// what the route's status becomes — so it runs in one transaction behind a
	// lock on the route. Two approvers signing at the same instant would otherwise
	// each recompute from a list that is missing the other's signature, and a
	// route whose last two steps land together would stay 'pending' with nothing
	// left for anyone to sign.
	var updated db.DocumentApprovalStep
	if err := h.inTx(c, func(q *db.Queries) error {
		locked, err := q.LockDocumentApproval(c, approval.ID)
		if err != nil {
			return err
		}
		if locked.Status != approvalPending {
			return errApprovalClosed
		}
		steps, err := q.ListDocumentApprovalSteps(c, locked.ID)
		if err != nil {
			return err
		}
		mine, found := stepForApprover(steps, uid)
		if !found {
			return errNotOnRoute
		}
		if mine.Status != approvalPending {
			return errAlreadyDecided
		}
		if !canDecideNow(locked.Mode, steps, mine.ID) {
			return errNotYourTurn
		}
		// The signature defaults to the name the route recorded. Letting it default
		// rather than requiring the client to echo it back keeps the two from ever
		// disagreeing about who signed.
		signature := truncateRunes(req.Signature, maxDocApprovalTitle)
		if signature == "" {
			signature = mine.ApproverName
		}
		updated, err = q.DecideDocumentApprovalStep(c, db.DecideDocumentApprovalStepParams{
			ID:        mine.ID,
			Status:    req.Decision,
			Comment:   truncateRunes(req.Comment, maxDocApprovalComment),
			Signature: signature,
		})
		if err != nil {
			return err
		}
		for i := range steps {
			if steps[i].ID == updated.ID {
				steps[i] = updated
			}
		}
		if next := nextApprovalStatus(steps); next != approvalPending {
			if _, err := q.CloseDocumentApproval(c, db.CloseDocumentApprovalParams{
				ID: locked.ID, Status: next,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, errApprovalClosed):
			c.JSON(http.StatusConflict, gin.H{"error": "approval is already closed"})
		case errors.Is(err, errNotOnRoute):
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not on this approval route"})
		case errors.Is(err, errAlreadyDecided):
			c.JSON(http.StatusConflict, gin.H{"error": "you have already decided"})
		case errors.Is(err, errNotYourTurn):
			c.JSON(http.StatusConflict, gin.H{"error": "an earlier approver has not signed yet"})
		default:
			fail(c, err)
		}
		return
	}
	h.notifyDocLinks(doc.ID)
	c.JSON(http.StatusOK, updated)
}

// CancelDocumentApproval withdraws an open route.
//
// Open to whoever raised it and to workspace managers, not to any member: a
// route someone else is halfway through signing is not something a passer-by
// should be able to void. Cancelling never deletes the protocol — the record
// that a document was sent for approval and pulled back is exactly the kind of
// thing a журнал согласований exists to keep.
func (h *API) CancelDocumentApproval(c *gin.Context) {
	approval, doc, ok := h.loadDocumentApproval(c)
	if !ok {
		return
	}
	if approval.Status != approvalPending {
		c.JSON(http.StatusConflict, gin.H{"error": "approval is already closed"})
		return
	}
	uid := middleware.CurrentUser(c)
	mine := approval.CreatedBy != nil && *approval.CreatedBy == uid
	if !mine && !h.requireManager(c, doc.WorkspaceID) {
		return
	}
	// The UPDATE is itself guarded on 'pending', so a cancellation that loses to a
	// signature landing in the same moment changes nothing and says so, instead of
	// overwriting an outcome the approvers already reached.
	updated, err := h.q.CloseDocumentApproval(c, db.CloseDocumentApprovalParams{
		ID: approval.ID, Status: approvalCancelled,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{"error": "approval is already closed"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.notifyDocLinks(doc.ID)
	c.JSON(http.StatusOK, updated)
}

// loadDocumentApproval fetches a protocol (path param :id) and authorizes
// through the document it hangs on — approvals have no workspace of their own.
func (h *API) loadDocumentApproval(c *gin.Context) (db.DocumentApproval, db.Document, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.DocumentApproval{}, db.Document{}, false
	}
	a, err := h.q.GetDocumentApproval(c, id)
	if notFound(c, err) {
		return db.DocumentApproval{}, db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentApproval{}, db.Document{}, false
	}
	doc, err := h.q.GetDocument(c, a.DocumentID)
	if notFound(c, err) {
		return db.DocumentApproval{}, db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentApproval{}, db.Document{}, false
	}
	if !h.requireMember(c, doc.WorkspaceID) {
		return db.DocumentApproval{}, db.Document{}, false
	}
	return a, doc, true
}

// ── route rules (pure, so they are testable without a database) ─────────────

// nextApprovalStatus reports what a route's status becomes given its steps.
//
// One rejection decides the whole route; otherwise it is settled only when
// everyone has signed. Anything else stays pending — including a route whose
// steps somehow vanished, which must not read as "approved by nobody".
func nextApprovalStatus(steps []db.DocumentApprovalStep) string {
	if len(steps) == 0 {
		return approvalPending
	}
	for _, s := range steps {
		if s.Status == approvalRejected {
			return approvalRejected
		}
	}
	for _, s := range steps {
		if s.Status != approvalApproved {
			return approvalPending
		}
	}
	return approvalApproved
}

// canDecideNow reports whether the given step may be signed right now.
//
// In parallel mode everyone is asked at once, so any pending step may go. In
// sequential mode only the earliest pending one may: that ordering is the whole
// difference between the two modes, and enforcing it on the client alone would
// make the order advisory.
func canDecideNow(mode string, steps []db.DocumentApprovalStep, stepID uuid.UUID) bool {
	if mode != approvalModeSequential {
		for _, s := range steps {
			if s.ID == stepID {
				return s.Status == approvalPending
			}
		}
		return false
	}
	ordered := make([]db.DocumentApprovalStep, len(steps))
	copy(ordered, steps)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	for _, s := range ordered {
		if s.Status == approvalPending {
			return s.ID == stepID
		}
	}
	return false
}

// stepForApprover finds the caller's place in the route.
func stepForApprover(steps []db.DocumentApprovalStep, uid uuid.UUID) (db.DocumentApprovalStep, bool) {
	for _, s := range steps {
		if s.ApproverID != nil && *s.ApproverID == uid {
			return s, true
		}
	}
	return db.DocumentApprovalStep{}, false
}

// dedupeUUIDs keeps the first occurrence of each id, preserving order — the
// order is the route.
func dedupeUUIDs(in []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(in))
	out := make([]uuid.UUID, 0, len(in))
	for _, id := range in {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
