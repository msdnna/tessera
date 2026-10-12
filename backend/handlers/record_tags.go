package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// Tags and @-mentions of a «Входящие» record — a note or a reminder (#3040,
// migration 0088).
//
// Tags are names. A record tied to a task (or, for a note, to a project) wears
// that project's tags: a name the project has is stored in the project's own
// spelling, compared the way every reader of a tag name compares one (trimmed,
// case-insensitive — see GetProjectTagByName), and the client paints the pill
// with the tag's colour. Any other name is the record's free label. Names rather
// than an M:N onto `tags`, so a private record stays out of the project tags'
// life cycle (merge, bulk delete with undo, project transfer) and never loses a
// label its author wrote when the project's tag goes away.
//
// Mentions are the ids of workspace members the record names. Only a note shared
// with the workspace tells them (kind "mention"): a private note and a reminder
// are their author's alone, so there the mention stays a reference to a person
// and nobody is notified — a notification would announce a record its reader
// cannot open.

const (
	recordTagMax     = 20
	recordTagMaxLen  = 64
	recordMentionMax = 50
)

// normalizeRecordTags trims the names, drops a leading «#» and the empty ones,
// and folds case-insensitive repeats onto the first spelling. A list that is too
// long, or a name that is, is the client's mistake and answers 400.
func normalizeRecordTags(c *gin.Context, in []string) ([]string, bool) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		name := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "#"))
		if name == "" {
			continue
		}
		if utf8.RuneCountInString(name) > recordTagMaxLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tag name too long"})
			return nil, false
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	if len(out) > recordTagMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many tags"})
		return nil, false
	}
	return out, true
}

// canonRecordTags spells each name the way the project's tag of that name is
// spelled. Without a project — or when the project's tags cannot be read — the
// names stay as written: free labels lose nothing.
func (h *API) canonRecordTags(c *gin.Context, projectID *uuid.UUID, names []string) []string {
	out := make([]string, 0, len(names))
	out = append(out, names...)
	if projectID == nil || len(names) == 0 {
		return out
	}
	tags, err := h.q.ListTags(c, *projectID)
	if err != nil {
		return out
	}
	spelled := make(map[string]string, len(tags))
	for _, tg := range tags {
		key := strings.ToLower(strings.TrimSpace(tg.Name))
		if _, ok := spelled[key]; !ok {
			spelled[key] = tg.Name
		}
	}
	seen := map[string]bool{}
	canon := out[:0]
	for _, n := range out {
		if s, ok := spelled[strings.ToLower(n)]; ok {
			n = s
		}
		key := strings.ToLower(strings.TrimSpace(n))
		if seen[key] {
			continue
		}
		seen[key] = true
		canon = append(canon, n)
	}
	return canon
}

// taskProjectID is the project a task lives in, or nil when it cannot be read.
func (h *API) taskProjectID(c *gin.Context, taskID *uuid.UUID) *uuid.UUID {
	if taskID == nil {
		return nil
	}
	t, err := h.q.GetTask(c, *taskID)
	if err != nil {
		return nil
	}
	b, err := h.q.GetBoard(c, t.BoardID)
	if err != nil {
		return nil
	}
	return &b.ProjectID
}

// noteProjectID is the project whose tags a note wears: its task's, else the
// project it was written in.
func (h *API) noteProjectID(c *gin.Context, taskID, projectID *uuid.UUID) *uuid.UUID {
	if p := h.taskProjectID(c, taskID); p != nil {
		return p
	}
	return projectID
}

// recordMentions keeps the ids that name members of the record's workspace, in
// order and without repeats. A record outside every workspace (an old reminder)
// has nobody to name. The ids come from the client — only it renders a handle
// against the roster (utils/mentions.mentionIdsIn) — so a stranger's id is
// dropped rather than trusted.
func (h *API) recordMentions(c *gin.Context, wsID *uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, bool) {
	out := make([]uuid.UUID, 0, len(ids))
	if len(ids) > recordMentionMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many mentions"})
		return nil, false
	}
	if wsID == nil {
		return out, true
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := h.q.GetMembership(c, db.GetMembershipParams{WorkspaceID: *wsID, UserID: id}); err != nil {
			continue
		}
		out = append(out, id)
	}
	return out, true
}

// notifyNoteMentions tells the people a shared note names that it names them —
// only those it did not name (as a shared note) before, so an edit that keeps
// the same mentions stays quiet, and a private note going shared tells everyone
// it names at that moment. The actor is skipped by notify.
func (h *API) notifyNoteMentions(c *gin.Context, n db.Note, before []uuid.UUID) {
	if n.Visibility != noteShared || len(n.Mentions) == 0 {
		return
	}
	told := map[uuid.UUID]bool{}
	for _, id := range before {
		told[id] = true
	}
	var msg *notifyMsg
	for _, id := range n.Mentions {
		if told[id] || id == middleware.CurrentUser(c) {
			continue
		}
		told[id] = true
		// Mentions are checked when they are stored; the person may have left
		// since, and a private-turned-shared note carries the old set.
		if _, err := h.q.GetMembership(c, db.GetMembershipParams{WorkspaceID: n.WorkspaceID, UserID: id}); err != nil {
			continue
		}
		if msg == nil {
			m := msgNoteMention(h.actorName(c), n.ID, n.Title, n.Body)
			msg = &m
		}
		h.notify(c, id, n.WorkspaceID, nil, "mention", *msg)
	}
}

// noteTitleLimit bounds the note title a mention notification carries: a note
// has no #N to point at, so its title is the only handle the reader gets.
const noteTitleLimit = 60

func clipRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

func msgNoteMention(actor string, noteID uuid.UUID, title, body string) notifyMsg {
	p := map[string]any{"event": evNoteMention, "note_id": noteID.String(), "actor": actor}
	t := clipRunes(title, noteTitleLimit)
	if t != "" {
		p["title"] = t
	}
	withQuote(p, body)
	ctx := ""
	if t != "" {
		ctx = " «" + t + "»"
	}
	return notifyMsg{
		text:    fmt.Sprintf("%s упомянул(а) вас в заметке%s", actor, ctx),
		payload: p,
	}
}
