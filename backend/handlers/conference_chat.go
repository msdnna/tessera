package handlers

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/confroom"
	"tessera/internal/db"
	"tessera/middleware"
)

// In-call chat (#2864, subtask #2873).
//
// The chat is HTTP, not another kind of frame on the room socket, and that is
// the one design decision the rest of this file follows from. The room's
// broadcast contract (internal/confroom) evicts a participant whose buffer
// overflows, because a dropped presence frame would leave a stale roster on
// screen forever. Pushing chat bodies — let alone attachments — through that
// same channel would let a lively conversation evict the people having it. So
// messages are stored and fetched over HTTP, and the socket carries only a
// payload-free "the chat changed" nudge; a nudge lost to a reconnect costs a
// stale panel until the next one, not a phantom message.
//
// Attachments follow handlers/attachments.go rather than inventing a second
// upload path: the same 25 MiB cap, the same sniff-don't-trust rule for the
// content type, the same "files live behind the API" placement. What is
// deliberately *not* reused is the inline-media directory — its only guard is
// an unguessable filename, and a private call's chat is the wrong place to
// relax authorization to that level.

const (
	// maxChatBody caps a message in runes. Long enough for a pasted stack trace,
	// short enough that the rail stays a chat rather than a document editor.
	maxChatBody = 4000
	// maxChatFiles caps one message's attachments. The limit exists so a single
	// request cannot occupy the upload path for 25 MiB × N.
	maxChatFiles = 5
	// defaultChatPage / maxChatPage bound one page of history.
	defaultChatPage = 50
	maxChatPage     = 200
)

// chatAttachment is one file on a message, as clients see it. is_image is
// computed from the *sniffed* content type — not from the filename, and not
// from the extension on disk either. Both of those are downstream of what the
// user typed: `payload.png` holding HTML is free to write, and a client that
// believed either would render it.
type chatAttachment struct {
	ID        uuid.UUID `json:"id"`
	MessageID uuid.UUID `json:"message_id"`
	Filename  string    `json:"filename"`
	Type      string    `json:"content_type"`
	Size      int64     `json:"size"`
	IsImage   bool      `json:"is_image"`
	CreatedAt time.Time `json:"created_at"`
}

// chatMessage is one line of the conversation with its files already attached,
// so the rail never has to fetch per message.
type chatMessage struct {
	ID          uuid.UUID        `json:"id"`
	UserID      *uuid.UUID       `json:"user_id"`
	UserName    *string          `json:"user_name"`
	Body        string           `json:"body"`
	CreatedAt   time.Time        `json:"created_at"`
	Attachments []chatAttachment `json:"attachments"`
}

func viewChatAttachment(a db.ConferenceMessageAttachment) chatAttachment {
	return chatAttachment{
		ID:        a.ID,
		MessageID: a.MessageID,
		Filename:  a.Filename,
		Type:      a.ContentType,
		Size:      a.Size,
		IsImage:   mediaExts[a.ContentType] != "",
		CreatedAt: a.CreatedAt,
	}
}

// ListConferenceMessages returns a page of the chat, oldest first.
//
// Paging is a (created_at, id) cursor rather than an offset: the chat grows
// while it is being read, and an offset would re-serve or skip a message every
// time somebody sends one between two "load older" clicks.
func (h *API) ListConferenceMessages(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	limit := defaultChatPage
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxChatPage {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = n
	}
	params := db.ListConferenceMessagesParams{ConferenceID: conf.ID, Limit: int32(limit)}
	// Both halves of the cursor or neither: a timestamp without the tie-breaking
	// id would compare against a NULL uuid and quietly return nothing.
	if rawAt, rawID := c.Query("before_at"), c.Query("before_id"); rawAt != "" || rawID != "" {
		at, err := time.Parse(time.RFC3339Nano, rawAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_at"})
			return
		}
		id, err := uuid.Parse(rawID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return
		}
		params.BeforeAt, params.BeforeID = &at, &id
	}
	rows, err := h.q.ListConferenceMessages(c, params)
	if err != nil {
		fail(c, err)
		return
	}
	msgs, err := h.hydrateChatPage(c, rows)
	if err != nil {
		fail(c, err)
		return
	}
	// has_more is answered by the page being full rather than by a second count
	// query: the only thing the client does with it is decide whether to draw a
	// "load older" button.
	c.JSON(http.StatusOK, gin.H{"messages": msgs, "has_more": len(rows) == limit})
}

// hydrateChatPage turns a newest-first page of rows into reading order and
// attaches every file in one extra statement.
func (h *API) hydrateChatPage(c *gin.Context, rows []db.ListConferenceMessagesRow) ([]chatMessage, error) {
	msgs := make([]chatMessage, 0, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	// The query returns newest first (that is what makes the cursor cheap); the
	// rail reads top to bottom, so the page is reversed here.
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		ids = append(ids, r.ID)
		msgs = append(msgs, chatMessage{
			ID: r.ID, UserID: r.UserID, UserName: r.UserName,
			Body: r.Body, CreatedAt: r.CreatedAt, Attachments: []chatAttachment{},
		})
	}
	if len(ids) == 0 {
		return msgs, nil
	}
	atts, err := h.q.ListConferenceMessageAttachmentsFor(c, ids)
	if err != nil {
		return nil, err
	}
	at := make(map[uuid.UUID]int, len(msgs))
	for i := range msgs {
		at[msgs[i].ID] = i
	}
	for _, a := range atts {
		if i, ok := at[a.MessageID]; ok {
			msgs[i].Attachments = append(msgs[i].Attachments, viewChatAttachment(a))
		}
	}
	return msgs, nil
}

// PostConferenceMessage adds a line to the chat, with or without files.
//
// It accepts both JSON (`{"body": "..."}`) and multipart, because the two are
// genuinely different requests: typing is the common case and should not pay
// for a multipart encoder, while a file cannot travel any other way.
func (h *API) PostConferenceMessage(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	// An ended call is read-only. Refusing here rather than accepting silently is
	// what keeps a tab left open overnight from appending to yesterday's meeting
	// where nobody will ever see it.
	if conf.Status == "ended" {
		c.JSON(http.StatusConflict, gin.H{"error": "conference has ended"})
		return
	}
	body, files, ok := h.readChatSubmission(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)

	// Files land on disk before the transaction so their paths can go into it,
	// and are swept back up if it fails: a row without its bytes is a broken
	// download, while bytes without a row are invisible and would leak.
	stored, ok := h.storeChatFiles(c, conf.ID, files)
	if !ok {
		return
	}
	var (
		msg  db.ConferenceMessage
		atts []db.ConferenceMessageAttachment
	)
	err := h.inTx(c, func(q *db.Queries) error {
		var err error
		msg, err = q.CreateConferenceMessage(c, db.CreateConferenceMessageParams{
			ConferenceID: conf.ID, UserID: &uid, Body: body,
		})
		if err != nil {
			return err
		}
		atts = atts[:0]
		for _, f := range stored {
			a, err := q.CreateConferenceMessageAttachment(c, db.CreateConferenceMessageAttachmentParams{
				MessageID:   msg.ID,
				UploaderID:  &uid,
				Filename:    f.name,
				ContentType: f.contentType,
				Size:        f.size,
				StoragePath: f.path,
			})
			if err != nil {
				return err
			}
			atts = append(atts, a)
		}
		return nil
	})
	if err != nil {
		for _, f := range stored {
			_ = os.Remove(f.path)
		}
		fail(c, err)
		return
	}

	out := chatMessage{
		ID: msg.ID, UserID: msg.UserID, Body: msg.Body, CreatedAt: msg.CreatedAt,
		Attachments: make([]chatAttachment, 0, len(atts)),
	}
	if name := h.userName(c, uid); name != "" {
		out.UserName = &name
	}
	for _, a := range atts {
		out.Attachments = append(out.Attachments, viewChatAttachment(a))
	}
	// The nudge carries no payload — see the note at the top of this file.
	h.notifyConfRoom(conf.ID, confroom.TypeChat)
	c.JSON(http.StatusCreated, out)
}

// storedChatFile is one upload that has already been written to disk.
type storedChatFile struct {
	path        string
	name        string
	contentType string
	size        int64
}

// readChatSubmission pulls the body and any files out of either encoding,
// writing the response and returning false when the submission is not usable.
func (h *API) readChatSubmission(c *gin.Context) (string, []*multipart.FileHeader, bool) {
	var body string
	var files []*multipart.FileHeader
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid form"})
			return "", nil, false
		}
		if v := form.Value["body"]; len(v) > 0 {
			body = v[0]
		}
		files = form.File["files"]
	} else {
		var req struct {
			Body string `json:"body"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return "", nil, false
		}
		body = req.Body
	}
	body = strings.TrimSpace(body)
	// An empty message with no files is not a message. Allowing it would put
	// blank rows in the rail every time somebody leans on Enter.
	if body == "" && len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message must have text or a file"})
		return "", nil, false
	}
	if utf8.RuneCountInString(body) > maxChatBody {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("message is longer than %d characters", maxChatBody)})
		return "", nil, false
	}
	if len(files) > maxChatFiles {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("at most %d files per message", maxChatFiles)})
		return "", nil, false
	}
	return body, files, true
}

// storeChatFiles writes the uploads under UPLOAD_DIR/conf/<conference>/ and
// describes them for the rows that follow. On any failure it removes whatever it
// had already written, writes the response and returns false.
func (h *API) storeChatFiles(c *gin.Context, confID uuid.UUID, files []*multipart.FileHeader) ([]storedChatFile, bool) {
	if len(files) == 0 {
		return nil, true
	}
	dir := filepath.Join(h.uploadDir, "conf", confID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return nil, false
	}
	stored := make([]storedChatFile, 0, len(files))
	abort := func(status int, msg string, err error) ([]storedChatFile, bool) {
		for _, f := range stored {
			_ = os.Remove(f.path)
		}
		if err != nil {
			fail(c, err)
		} else {
			c.JSON(status, gin.H{"error": msg})
		}
		return nil, false
	}
	for _, f := range files {
		if f.Size > maxAttachmentBytes {
			return abort(http.StatusRequestEntityTooLarge, "файл больше 25 МБ", nil)
		}
		// Sniffed, never declared: this value is what tells the rail to render a
		// picture, and the extension it produces is what the download route reads
		// back to decide the same thing.
		ct, err := sniffContentType(f)
		if err != nil {
			return abort(0, "", err)
		}
		// The extension on disk comes from the sniff, and only for the types we
		// recognise; everything else is stored suffix-less. Carrying the user's
		// extension over would put a file called `payload.png` on disk holding
		// HTML, and then the name on disk would be one more thing that can lie
		// about the bytes. The original name lives in the row, which is the only
		// place the download filename is read from.
		path := filepath.Join(dir, uuid.NewString()+mediaExts[ct])
		if err := saveUploaded(f, path); err != nil {
			return abort(0, "", err)
		}
		stored = append(stored, storedChatFile{
			path: path, name: filepath.Base(f.Filename), contentType: ct, size: f.Size,
		})
	}
	return stored, true
}

// DownloadConferenceMessageAttachment streams a chat file to a member of the
// conference's workspace.
//
// Everything here is served as a download rather than inline. The client that
// shows a picture in the rail fetches these bytes with its bearer credential and
// renders them from a blob, so it never needs this route to be renderable — and
// making it renderable would put attacker-supplied bytes on the app's own origin
// behind nothing but a bearer token, which is the trade the inline-media route
// makes on purpose and this one has no reason to.
func (h *API) DownloadConferenceMessageAttachment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	att, err := h.q.GetConferenceMessageAttachment(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	conf, err := h.q.ConferenceForMessageAttachment(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, conf.WorkspaceID) {
		return
	}
	if _, err := os.Stat(att.StoragePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "файл недоступен"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
	c.Header("X-Content-Type-Options", "nosniff")
	// The type is the sniffed one, so a client that renders the bytes from a blob
	// gets a picture rather than a download prompt.
	if att.ContentType != "" {
		c.Header("Content-Type", att.ContentType)
	}
	c.File(att.StoragePath)
}

// DeleteConferenceMessage removes a line of the chat and its files. The author
// may retract their own; a host or workspace admin may remove anyone's, which is
// the same moderation boundary the rest of the call uses.
func (h *API) DeleteConferenceMessage(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	msg, err := h.q.GetConferenceMessage(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	conf, err := h.q.GetConference(c, msg.ConferenceID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, conf.WorkspaceID) {
		return
	}
	uid := middleware.CurrentUser(c)
	mine := msg.UserID != nil && *msg.UserID == uid
	if !mine && !h.canManageConference(c, conf) {
		c.JSON(http.StatusForbidden, gin.H{"error": "requires the message author, the conference host or a workspace admin"})
		return
	}
	// Read the paths before the row goes: ON DELETE CASCADE takes the attachment
	// rows with it, and after that nothing remembers where the bytes are.
	paths, err := h.q.ListConferenceMessageAttachmentPaths(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	if err := h.q.DeleteConferenceMessage(c, id); err != nil {
		fail(c, err)
		return
	}
	for _, p := range paths {
		_ = os.Remove(p) // best-effort; the rows are already gone
	}
	h.notifyConfRoom(conf.ID, confroom.TypeChat)
	c.Status(http.StatusNoContent)
}

// userName resolves a display name for the author of a message just created, so
// the poster's own client can render it without refetching the page.
func (h *API) userName(c *gin.Context, uid uuid.UUID) string {
	u, err := h.q.GetUserByID(c, uid)
	if err != nil {
		return ""
	}
	return u.Name
}

