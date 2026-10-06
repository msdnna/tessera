package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/quickact"
	"tessera/middleware"
)

// Quick actions: GitLab-style commands typed into a comment ("/close",
// "/assign @msdnna"). quickact does the parsing; this file resolves arguments
// against the workspace and runs them through the shared task ops in
// task_ops.go, so a command fires exactly the same journal entries,
// notifications and GitLab writebacks as the equivalent REST call.
//
// Custom, workspace-dictionary commands ("/approve") are recognised by the
// parser but never reach here: they stay in the comment text as a note to
// whoever reads it.

// cmdOutcome is what one command did — or why it didn't.
type cmdOutcome struct {
	Key     string `json:"key"`
	Arg     string `json:"arg,omitempty"`
	Summary string `json:"summary,omitempty"` // human text, shown in the comment's command summary
	Error   string `json:"error,omitempty"`
}

// commandSummary is the per-request rollup returned to clients and mirrored
// into the task journal.
type commandSummary struct {
	Applied []cmdOutcome `json:"applied"`
	Errors  []cmdOutcome `json:"errors"`
	// Custom lists recognised dictionary commands, which were left in the text.
	Custom []string `json:"custom,omitempty"`
}

func (s commandSummary) empty() bool { return len(s.Applied) == 0 && len(s.Errors) == 0 }

// runQuickActions executes parsed commands top to bottom.
//
// A failing command does not abort the rest (GitLab behaves the same way): its
// error is collected and the next one runs. There is deliberately no
// transaction around the batch — each op has already fanned out WebSocket
// events and GitLab writeback rows by the time the next one fails, and those
// cannot be rolled back.
func (h *API) runQuickActions(c *gin.Context, t db.Task, wsID uuid.UUID, cmds []quickact.Cmd) commandSummary {
	var sum commandSummary
	for _, cmd := range cmds {
		// Re-read the task between commands: "/move В процессе" followed by
		// "/close" must see the column the first one just set.
		if fresh, err := h.q.GetTask(c, t.ID); err == nil {
			t = fresh
		}
		summary, err := h.runQuickAction(c, &t, wsID, cmd)
		switch {
		case err != nil:
			sum.Errors = append(sum.Errors, cmdOutcome{Key: cmd.Key, Arg: cmd.Arg, Error: err.Error()})
		default:
			sum.Applied = append(sum.Applied, cmdOutcome{Key: cmd.Key, Arg: cmd.Arg, Summary: summary})
		}
	}
	return sum
}

// runQuickAction executes one command and returns a human summary of what it did.
func (h *API) runQuickAction(c *gin.Context, t *db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	switch cmd.Key {
	case "assign", "unassign":
		return h.qaAssign(c, *t, wsID, cmd)
	case "due", "start", "remove_due", "remove_start", "estimate", "remove_estimate", "priority", "title":
		return h.qaPatch(c, t, wsID, cmd)
	case "tag", "untag":
		return h.qaTag(c, *t, wsID, cmd)
	case "milestone":
		return h.qaMilestone(c, *t, wsID, cmd)
	case "remove_milestone":
		return "убран майлстоун", h.applyMilestone(c, t.ID, wsID, nil)
	case "move", "close", "reopen":
		return h.qaMove(c, t, wsID, cmd)
	case "archive":
		return "задача архивирована", h.applyArchive(c, *t, wsID, false)
	case "relate", "blocks", "blocked_by", "duplicates", "unlink":
		return h.qaRelation(c, *t, wsID, cmd)
	case "subtask":
		sub, err := h.applyCreateSubtask(c, *t, wsID, cmd.Arg)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("создана подзадача #%s", taskRef(sub.Number)), nil
	case "parent":
		return h.qaParent(c, t, wsID, cmd)
	case "unparent":
		updated, err := h.applyParent(c, *t, wsID, nil)
		if err != nil {
			return "", err
		}
		*t = updated
		return "откреплена от родителя", nil
	}
	// Unreachable: the parser only emits keys present in quickact.Registry.
	return "", userErr("команда /%s пока не поддерживается", cmd.Key)
}

// A command whose argument is a *list* — "/assign @a @b", "/tag x, y" — is
// partially satisfiable: some entries name something in this workspace and some
// do not. The rule is "apply what resolves, name the rest", and it only fails
// when nothing at all resolved.
//
// The dry run behind the editor hint has to reach the same verdict, and it used
// to not: it refused the whole command on the first unknown entry, so
// "/assign @known @typo" was shown as a failure and then went on to assign
// @known. Both sides now build the answer through the types below, which is the
// only way they cannot drift apart again.

// namedRef is one resolved entry: what to act on, and how to name it back.
type namedRef struct {
	id    uuid.UUID
	label string
}

// partialList is the outcome of resolving a list argument.
type partialList struct {
	done    []string // labels of entries that resolved (and, in a real run, applied)
	missing []string // labels of entries that named nothing
	// ambiguous are entries that named MORE than one member — a miss of a
	// different kind, so they get their own clause rather than being folded into
	// missing: "не нашёл" would be a lie about a handle the workspace does carry,
	// twice over. Only mentions can land here; tags and relations are resolved by
	// a unique name and a unique number. Each label carries its candidates.
	ambiguous []string
}

// summary renders the applied half plus, in parentheses, what was skipped.
func (p partialList) summary(verb, skipLabel string) string {
	s := verb + " " + strings.Join(p.done, ", ")
	if len(p.missing) > 0 {
		s += " (" + skipLabel + ": " + strings.Join(p.missing, ", ") + ")"
	}
	if len(p.ambiguous) > 0 {
		s += " (неоднозначно: " + strings.Join(p.ambiguous, "; ") + ")"
	}
	return s
}

// missed lists every unresolved entry — every one of them, not just the first:
// a user who mistyped two logins wants both named in one go.
func (p partialList) missed() string { return strings.Join(p.missing, ", ") }

// resolveMentions splits parsed @logins into workspace members and misses. Both
// halves keep the "@" the user typed, so the summary and the error read back the
// way the command was written.
func resolveMentions(members []db.ListMembersRow, logins []string) ([]namedRef, partialList) {
	var refs []namedRef
	var p partialList
	for _, login := range logins {
		member, rivals := matchMember(members, login)
		switch {
		case len(rivals) > 0:
			p.ambiguous = append(p.ambiguous, "@"+login+" → "+memberLabels(rivals))
		case member == uuid.Nil:
			p.missing = append(p.missing, "@"+login)
		default:
			refs = append(refs, namedRef{id: member, label: "@" + login})
		}
	}
	return refs, p
}

// memberLabels names the members a mention could have meant, each with the email
// that tells them apart — an email is unique per account, so it is the one thing
// the writer can retype to mean exactly one person.
func memberLabels(rows []db.ListMembersRow) string {
	out := make([]string, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.Name+" <"+m.Email+">")
	}
	return strings.Join(out, ", ")
}

// mentionMissErr is why an /assign or /unassign applied to nobody: handles that
// named no one, handles that named several, or both. Kept apart from missed()
// because the two halves need different sentences — see partialList.ambiguous.
func mentionMissErr(p partialList) error {
	switch {
	case len(p.ambiguous) == 0:
		return userErr("не нашёл участника: %s", p.missed())
	case len(p.missing) == 0:
		return userErr("неоднозначно: %s", strings.Join(p.ambiguous, "; "))
	}
	return userErr("не нашёл участника: %s; неоднозначно: %s",
		p.missed(), strings.Join(p.ambiguous, "; "))
}

// resolveTagNames does the same for tag names within the task's project. The
// label is the name as typed, matching what the error says about the misses.
func resolveTagNames(tags []db.Tag, names []string) ([]namedRef, partialList) {
	var refs []namedRef
	var p partialList
	for _, name := range names {
		tag, ok := matchTag(tags, name)
		if !ok {
			p.missing = append(p.missing, name)
			continue
		}
		refs = append(refs, namedRef{id: tag, label: name})
	}
	return refs, p
}

// qaAssign resolves @logins to workspace members and (un)assigns them.
func (h *API) qaAssign(c *gin.Context, t db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	add := cmd.Key == "assign"
	logins := quickact.ParseMentions(cmd.Arg)

	// "/unassign" with no argument clears everyone — the documented shorthand.
	if !add && len(logins) == 0 {
		assignees, err := h.q.ListTaskAssignees(c, t.ID)
		if err != nil {
			return "", err
		}
		if len(assignees) == 0 {
			return "", userErr("у задачи нет исполнителей")
		}
		for _, a := range assignees {
			if err := h.applyAssignee(c, t, wsID, a.ID, false); err != nil {
				return "", err
			}
		}
		return "сняты все исполнители", nil
	}
	if len(logins) == 0 {
		return "", userErr("не указан исполнитель (например @msdnna)")
	}

	members, err := h.q.ListMembers(c, wsID)
	if err != nil {
		return "", err
	}
	refs, part := resolveMentions(members, logins)
	for _, ref := range refs {
		if err := h.applyAssignee(c, t, wsID, ref.id, add); err != nil {
			return "", err
		}
		part.done = append(part.done, ref.label)
	}
	if len(part.done) == 0 {
		return "", mentionMissErr(part)
	}
	verb := "назначен"
	if !add {
		verb = "снят"
	}
	// Partial success is still success — say who was skipped rather than
	// failing the whole command and losing the assignments that worked.
	return part.summary(verb, "не найдены"), nil
}

// mentionShapes are the ways a mention can name a workspace member, MOST
// SPECIFIC FIRST: full email (unique per account), then the GitLab login the
// @-autocomplete inserts for OAuth-linked members, then the email's local part,
// then the display name — whatever the person typed into their profile, and the
// only shape a workspace can legitimately hold twice.
//
// The order is the whole point. This used to be a loop over MEMBERS that tried
// all four shapes on each row, so the winner was not the closest match but the
// earliest row: a member whose local part or display name happened to read like
// another member's GitLab login took the mention away from its real owner, and
// «/assign @ivan» put the task on the wrong Ivan while reporting back the very
// handle that was asked for. Nothing downstream can tell that apart from a
// deliberate assignment. Same fix, and same reasoning, as the MCP server's
// matchMember (#2932).
var mentionShapes = []func(m db.ListMembersRow, want string) bool{
	func(m db.ListMembersRow, want string) bool {
		return strings.ToLower(strings.TrimSpace(m.Email)) == want
	},
	func(m db.ListMembersRow, want string) bool {
		return m.GlUsername != "" && strings.ToLower(strings.TrimSpace(m.GlUsername)) == want
	},
	func(m db.ListMembersRow, want string) bool {
		local, _, ok := strings.Cut(strings.ToLower(strings.TrimSpace(m.Email)), "@")
		return ok && local == want
	},
	func(m db.ListMembersRow, want string) bool {
		return strings.ToLower(strings.TrimSpace(m.Name)) == want
	},
}

// matchMember resolves a mention to a workspace member, trying the shapes above
// in order. A shape that exactly one member answers to wins outright; a shape
// two of them answer to resolves to nobody and returns the rivals, so the caller
// can name them instead of guessing between them (the ambiguity never reaches a
// later, vaguer shape — two «Иван» are not disambiguated by looking at names
// harder). A miss returns uuid.Nil and no rivals.
func matchMember(members []db.ListMembersRow, login string) (uuid.UUID, []db.ListMembersRow) {
	want := strings.ToLower(strings.TrimSpace(login))
	if want == "" {
		return uuid.Nil, nil
	}
	for _, hits := range mentionShapes {
		var found []db.ListMembersRow
		for _, m := range members {
			if hits(m, want) {
				found = append(found, m)
			}
		}
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0].UserID, nil
		default:
			return uuid.Nil, found
		}
	}
	return uuid.Nil, nil
}

// commandNow is the clock a date command reads: the present moment, in the
// calendar of whoever typed the command.
//
// «/due сегодня» is a statement about the writer's day, and the day the server
// process happens to be in is not it. ParseDate resolves every relative form and
// every bare date to midnight in the location it is handed, and the clients store
// a due date as exactly that local midnight (resolvedTimezone in
// frontend/src/utils/format.js) — so passing the server's clock lands on the wrong
// day twice: the stored instant is a different day's start, and the summary
// written back into the comment ("срок 14.08.2026") then names a date the due pill
// on that same card does not show, because the frontend renders it in the user's
// zone. A comment written at 01:30 in +03:00 is 22:30 of the previous day in UTC,
// which is how «/due сегодня» used to file a task as already overdue.
func (h *API) commandNow(c *gin.Context) time.Time {
	return time.Now().In(h.callerLocation(c, middleware.CurrentUser(c)))
}

// qaPatch handles the commands that are a partial edit of the task's own fields.
func (h *API) qaPatch(c *gin.Context, t *db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	var patch taskPatch
	var summary string

	switch cmd.Key {
	case "due", "start":
		when, err := quickact.ParseDate(cmd.Arg, h.commandNow(c))
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		if cmd.Key == "due" {
			patch.DueDate = setTime(&when)
			summary = "срок " + when.Format("02.01.2006")
		} else {
			patch.StartDate = setTime(&when)
			summary = "начало " + when.Format("02.01.2006")
		}
	case "remove_due":
		patch.DueDate = clearTime()
		summary = "убран срок"
	case "remove_start":
		patch.StartDate = clearTime()
		summary = "убрана дата начала"
	case "estimate":
		v, err := quickact.ParseEstimate(cmd.Arg, h.estimateConfig(c, *t, wsID))
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		patch.Estimate = setFloat(&v)
		summary = "оценка " + cmd.Arg
	case "remove_estimate":
		patch.Estimate = clearFloat()
		summary = "убрана оценка"
	case "priority":
		p, err := quickact.ParsePriority(cmd.Arg)
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		patch.Priority = &p
		summary = "приоритет " + cmd.Arg
	case "title":
		title := strings.TrimSpace(cmd.Arg)
		if title == "" {
			return "", userErr("не указан заголовок")
		}
		patch.Title = &title
		summary = "заголовок «" + title + "»"
	}

	updated, err := h.applyTaskPatch(c, *t, wsID, patch)
	if err != nil {
		return "", err
	}
	*t = updated
	return summary, nil
}

// qaTag resolves tag names within the task's project. A missing tag is an error
// rather than an implicit create: a typo would otherwise litter the project with
// one-off tags nobody meant to make.
func (h *API) qaTag(c *gin.Context, t db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	names := quickact.ParseTags(cmd.Arg)
	if len(names) == 0 {
		return "", userErr("не указан тег")
	}
	projectID, err := h.q.ProjectIDForBoard(c, t.BoardID)
	if err != nil {
		return "", err
	}
	tags, err := h.q.ListTags(c, projectID)
	if err != nil {
		return "", err
	}
	add := cmd.Key == "tag"
	refs, part := resolveTagNames(tags, names)
	for _, ref := range refs {
		if err := h.applyTag(c, t.ID, wsID, ref.id, add); err != nil {
			return "", err
		}
		part.done = append(part.done, ref.label)
	}
	if len(part.done) == 0 {
		return "", userErr("нет такого тега в проекте: %s", part.missed())
	}
	verb := "добавлен тег"
	if !add {
		verb = "убран тег"
	}
	return part.summary(verb, "нет в проекте"), nil
}

func matchTag(tags []db.Tag, name string) (uuid.UUID, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, t := range tags {
		if strings.ToLower(t.Name) == name {
			return t.ID, true
		}
	}
	return uuid.Nil, false
}

// qaMilestone pins a milestone of the task's project by title.
func (h *API) qaMilestone(c *gin.Context, t db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	title := strings.TrimSpace(cmd.Arg)
	if title == "" {
		return "", userErr("не указан майлстоун")
	}
	projectID, err := h.q.ProjectIDForBoard(c, t.BoardID)
	if err != nil {
		return "", err
	}
	rows, err := h.q.ListMilestones(c, projectID)
	if err != nil {
		return "", err
	}
	for _, m := range rows {
		if strings.EqualFold(strings.TrimSpace(m.Title), title) {
			id := m.ID
			if err := h.applyMilestone(c, t.ID, wsID, &id); err != nil {
				return "", err
			}
			return "майлстоун «" + m.Title + "»", nil
		}
	}
	return "", userErr("нет такого майлстоуна в проекте: %s", title)
}

// matchColumn resolves the column name typed in `/move …` against a board's
// columns, or nil when nothing on the board answers to it.
//
// Two passes, and the order matters. The stored name comes first: a column
// somebody named themselves is user data and must win outright. Only then the
// seeded captions (#2800) — those four columns are stored under their Russian
// names but captioned by each client from name_key, so an English reader's board
// says "Done" where the row says «Готово», and `/move Done` was answering «нет
// колонки «Done» на этой доске» about a column they were looking at. /close and
// /reopen never had the problem: they go through done_column_id, no name at all.
//
// Only a column that still carries a name_key is reachable by caption — renaming
// one clears the key (see UpdateColumn), and a renamed column means what it now
// says, not what it was seeded as. Same name-or-key resolution the GitLab column
// rules do.
func matchColumn(cols []db.BoardColumn, typed string) *db.BoardColumn {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return nil
	}
	for i := range cols {
		if strings.EqualFold(strings.TrimSpace(cols[i].Name), typed) {
			return &cols[i]
		}
	}
	key := seededColumnKey(typed)
	if key == "" {
		return nil
	}
	for i := range cols {
		if cols[i].NameKey != nil && *cols[i].NameKey == key {
			return &cols[i]
		}
	}
	return nil
}

// moveTarget resolves the column /move, /close or /reopen sends the task to.
//
// A nil column with no error is not a refusal: it means the command is a plain
// completed-flag toggle, which is what /close and /reopen fall back to on a board
// with no done column configured (and what /reopen does for a task already
// outside it) — the same thing the UI checkbox does in that situation.
//
// Shared with previewCommand on purpose. The dry run and the run have to answer
// about the same column, and every time the preview kept its own copy of this
// resolution it drifted: #2800's caption fallback was in matchColumn and not in
// the preview's loop, so `/move Done` was painted red and then moved the task
// anyway. One resolver means the next rule added here reaches both callers
// whether or not anyone remembers the second one.
func (h *API) moveTarget(c *gin.Context, t db.Task, cols []db.BoardColumn, cmd quickact.Cmd) (*db.BoardColumn, error) {
	switch cmd.Key {
	case "move":
		name := strings.TrimSpace(cmd.Arg)
		if name == "" {
			return nil, userErr("не указана колонка")
		}
		target := matchColumn(cols, name)
		if target == nil {
			return nil, userErr("нет колонки «%s» на этой доске", name)
		}
		return target, nil
	case "close", "reopen":
		board, err := h.q.GetBoard(c, t.BoardID)
		if err != nil {
			return nil, err
		}
		doneID := doneColumnID(board)
		if doneID == nil {
			return nil, nil
		}
		if cmd.Key == "close" {
			for i := range cols {
				if cols[i].ID == *doneID {
					return &cols[i], nil
				}
			}
			return nil, nil
		}
		if t.ColumnID != *doneID {
			return nil, nil
		}
		// Reopening out of the done column: the leftmost column is where the
		// board puts new work.
		for i := range cols {
			if cols[i].ID != *doneID {
				return &cols[i], nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

// qaMove handles /move, /close and /reopen — all three are a column change.
//
// /close and /reopen resolve the board's done column; a board without one falls
// back to toggling the completed flag directly, which is what the UI checkbox
// does in the same situation.
func (h *API) qaMove(c *gin.Context, t *db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	cols, err := h.q.ListColumns(c, t.BoardID)
	if err != nil {
		return "", err
	}
	target, err := h.moveTarget(c, *t, cols, cmd)
	if err != nil {
		return "", err
	}
	if target == nil {
		// No column to move into — toggle the completed flag instead.
		completed := cmd.Key == "close"
		updated, perr := h.applyTaskPatch(c, *t, wsID, taskPatch{Completed: &completed})
		if perr != nil {
			return "", perr
		}
		*t = updated
		if completed {
			return "задача закрыта", nil
		}
		return "задача переоткрыта", nil
	}

	if target.ID == t.ColumnID {
		return "", userErr("задача уже в «%s»", target.Name)
	}
	var summary string
	switch cmd.Key {
	case "move":
		summary = "перенесена в «" + target.Name + "»"
	case "close":
		summary = "задача закрыта («" + target.Name + "»)"
	default:
		summary = "задача переоткрыта («" + target.Name + "»)"
	}
	// A quick action has no drag neighbours, so the slot is the server's to pick —
	// the same answer the move endpoint gives an unanchored move: a top-level card
	// appends to the end of the destination column, a subtask keeps its place among
	// its parent's children (unanchoredPosition).
	pos, err := h.unanchoredPosition(c, *t, target.ID)
	if err != nil {
		return "", err
	}
	updated, err := h.applyMove(c, *t, wsID, target.ID, pos)
	if err != nil {
		return "", err
	}
	*t = updated
	// A recurring task can bounce straight back out of the done column, so report
	// where it actually landed rather than where it was sent.
	if updated.ColumnID != target.ID {
		for i := range cols {
			if cols[i].ID == updated.ColumnID {
				summary += " → фактически «" + cols[i].Name + "» (повторяющаяся задача)"
			}
		}
	}
	return summary, nil
}

// qaRelation links or unlinks the task with the referenced #N.
func (h *API) qaRelation(c *gin.Context, t db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	refs, err := quickact.ParseRefs(cmd.Arg)
	if err != nil {
		return "", opError{msg: err.Error()}
	}
	var done []string
	var missing []string
	for _, num := range refs {
		n := int64(num)
		target, terr := h.q.GetTaskByNumber(c, db.GetTaskByNumberParams{WorkspaceID: wsID, Number: &n})
		if terr != nil {
			missing = append(missing, fmt.Sprintf("#%d", num))
			continue
		}
		if cmd.Key == "unlink" {
			removed, uerr := h.applyUnlink(c, t, wsID, target)
			if uerr != nil {
				return "", uerr
			}
			if removed == 0 {
				missing = append(missing, fmt.Sprintf("#%d (не было связи)", num))
				continue
			}
		} else if rerr := h.applyRelation(c, t, wsID, target, relationKindForCmd(cmd.Key)); rerr != nil {
			return "", rerr
		}
		done = append(done, fmt.Sprintf("#%d", num))
	}
	if len(done) == 0 {
		return "", userErr("не найдено: %s", strings.Join(missing, ", "))
	}
	verb := map[string]string{
		"relate": "связана с", "blocks": "блокирует",
		"blocked_by": "заблокирована", "duplicates": "дублирует", "unlink": "снята связь с",
	}[cmd.Key]
	summary := verb + " " + strings.Join(done, ", ")
	if len(missing) > 0 {
		summary += " (пропущено: " + strings.Join(missing, ", ") + ")"
	}
	return summary, nil
}

// taskNumberExists reports whether a per-workspace task number names a task —
// the read the dry run of every reference-taking command needs, and nothing else.
func (h *API) taskNumberExists(c *gin.Context, wsID uuid.UUID, num int32) bool {
	n := int64(num)
	_, err := h.q.GetTaskByNumber(c, db.GetTaskByNumberParams{WorkspaceID: wsID, Number: &n})
	return err == nil
}

// qaParent re-parents the task under the referenced #N.
func (h *API) qaParent(c *gin.Context, t *db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	refs, err := quickact.ParseRefs(cmd.Arg)
	if err != nil {
		return "", opError{msg: err.Error()}
	}
	n := int64(refs[0])
	parent, err := h.q.GetTaskByNumber(c, db.GetTaskByNumberParams{WorkspaceID: wsID, Number: &n})
	if err != nil {
		return "", userErr("задача #%d не найдена", refs[0])
	}
	updated, err := h.applyParent(c, *t, wsID, &parent.ID)
	if err != nil {
		return "", err
	}
	*t = updated
	return fmt.Sprintf("стала подзадачей #%d", refs[0]), nil
}

// estimateConfig resolves the estimate settings in force for a task, following
// the same project → workspace → built-in (time, 8h/5d) chain the clients use,
// so "/estimate 1d" means what "1d" typed into the UI means — and so a points
// board reads the argument as points rather than as a duration.
func (h *API) estimateConfig(c *gin.Context, t db.Task, wsID uuid.UUID) quickact.Estimation {
	var raw *json.RawMessage
	if projectID, err := h.q.ProjectIDForBoard(c, t.BoardID); err == nil {
		if p, perr := h.q.GetProject(c, projectID); perr == nil {
			raw = p.Estimation
		}
	}
	if raw == nil {
		if ws, err := h.q.GetWorkspace(c, wsID); err == nil {
			raw = ws.Estimation
		}
	}
	if raw == nil {
		return quickact.Estimation{}
	}
	var cfg estimationConfig
	if err := json.Unmarshal(*raw, &cfg); err != nil {
		return quickact.Estimation{}
	}
	return quickact.Estimation{
		Unit:        cfg.Unit,
		HoursPerDay: cfg.HoursPerDay,
		DaysPerWeek: cfg.DaysPerWeek,
		PointsScale: cfg.PointsScale,
	}
}

// PreviewCommands dry-runs a comment body: it reports what each command would
// do without executing anything, so the editor can show "Будет применено: …"
// from the same parser the backend runs. Read-only by construction — it never
// calls an apply* op.
func (h *API) PreviewCommands(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res := quickact.Parse(req.Body, h.customCommandKeys(c, wsID))

	out := make([]cmdOutcome, 0, len(res.Cmds))
	for _, cmd := range res.Cmds {
		summary, err := h.previewCommand(c, t, wsID, cmd)
		oc := cmdOutcome{Key: cmd.Key, Arg: cmd.Arg, Summary: summary}
		if err != nil {
			oc.Summary, oc.Error = "", err.Error()
		}
		out = append(out, oc)
	}
	custom := make([]string, 0, len(res.Custom))
	for _, cmd := range res.Custom {
		custom = append(custom, cmd.Key)
	}
	c.JSON(http.StatusOK, gin.H{
		"commands":   out,
		"custom":     custom,
		"rest_empty": strings.TrimSpace(res.Rest) == "",
	})
}

// previewCommand validates one command's argument and describes the intent,
// without touching anything.
func (h *API) previewCommand(c *gin.Context, t db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	switch cmd.Key {
	case "assign", "unassign":
		logins := quickact.ParseMentions(cmd.Arg)
		if len(logins) == 0 {
			if cmd.Key == "unassign" {
				// The argument-less shorthand clears everyone, and qaAssign refuses
				// it on a task that has nobody to clear. The dry run has to read the
				// same list: answering «снять всех исполнителей» in green and then
				// «у задачи нет исполнителей» in red describes two different tasks.
				assignees, err := h.q.ListTaskAssignees(c, t.ID)
				if err != nil {
					return "", err
				}
				if len(assignees) == 0 {
					return "", userErr("у задачи нет исполнителей")
				}
				return "снять всех исполнителей", nil
			}
			return "", userErr("не указан исполнитель (например @msdnna)")
		}
		members, err := h.q.ListMembers(c, wsID)
		if err != nil {
			return "", err
		}
		refs, part := resolveMentions(members, logins)
		for _, ref := range refs {
			part.done = append(part.done, ref.label)
		}
		if len(part.done) == 0 {
			return "", mentionMissErr(part)
		}
		verb := "назначить"
		if cmd.Key != "assign" {
			verb = "снять"
		}
		return part.summary(verb, "не найдены"), nil

	case "due", "start":
		when, err := quickact.ParseDate(cmd.Arg, h.commandNow(c))
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		if cmd.Key == "due" {
			return "срок " + when.Format("02.01.2006"), nil
		}
		return "начало " + when.Format("02.01.2006"), nil
	case "remove_due":
		return "убрать срок", nil
	case "remove_start":
		return "убрать дату начала", nil

	case "estimate":
		if _, err := quickact.ParseEstimate(cmd.Arg, h.estimateConfig(c, t, wsID)); err != nil {
			return "", opError{msg: err.Error()}
		}
		return "оценка " + cmd.Arg, nil
	case "remove_estimate":
		return "убрать оценку", nil

	case "priority":
		if _, err := quickact.ParsePriority(cmd.Arg); err != nil {
			return "", opError{msg: err.Error()}
		}
		return "приоритет " + cmd.Arg, nil
	case "title":
		if strings.TrimSpace(cmd.Arg) == "" {
			return "", userErr("не указан заголовок")
		}
		return "заголовок «" + strings.TrimSpace(cmd.Arg) + "»", nil

	case "tag", "untag":
		names := quickact.ParseTags(cmd.Arg)
		if len(names) == 0 {
			return "", userErr("не указан тег")
		}
		projectID, err := h.q.ProjectIDForBoard(c, t.BoardID)
		if err != nil {
			return "", err
		}
		tags, err := h.q.ListTags(c, projectID)
		if err != nil {
			return "", err
		}
		refs, part := resolveTagNames(tags, names)
		for _, ref := range refs {
			part.done = append(part.done, ref.label)
		}
		if len(part.done) == 0 {
			return "", userErr("нет такого тега в проекте: %s", part.missed())
		}
		verb := "добавить тег"
		if cmd.Key != "tag" {
			verb = "убрать тег"
		}
		return part.summary(verb, "нет в проекте"), nil

	case "milestone":
		title := strings.TrimSpace(cmd.Arg)
		if title == "" {
			return "", userErr("не указан майлстоун")
		}
		projectID, err := h.q.ProjectIDForBoard(c, t.BoardID)
		if err != nil {
			return "", err
		}
		rows, err := h.q.ListMilestones(c, projectID)
		if err != nil {
			return "", err
		}
		for _, m := range rows {
			if strings.EqualFold(strings.TrimSpace(m.Title), title) {
				return "майлстоун «" + m.Title + "»", nil
			}
		}
		return "", userErr("нет такого майлстоуна в проекте: %s", title)
	case "remove_milestone":
		return "убрать майлстоун", nil

	// The same resolver qaMove runs on, including the "already there" refusal.
	// The preview used to carry its own column loop — which knew nothing about
	// the seeded-caption fallback of #2800, so `/move Done` was painted red and
	// then moved the task anyway — and it did not look at where the task was
	// standing at all, so `/move Готово` on a task already in «Готово», and
	// `/close` on a task already closed by column, went out green and came back
	// «задача уже в «Готово»».
	case "move", "close", "reopen":
		cols, err := h.q.ListColumns(c, t.BoardID)
		if err != nil {
			return "", err
		}
		target, err := h.moveTarget(c, t, cols, cmd)
		if err != nil {
			return "", err
		}
		if target == nil {
			// No column to move into: the run toggles the completed flag, which
			// always applies — there is nothing here for the dry run to refuse.
			if cmd.Key == "reopen" {
				return "переоткрыть задачу", nil
			}
			return "закрыть задачу", nil
		}
		if target.ID == t.ColumnID {
			return "", userErr("задача уже в «%s»", target.Name)
		}
		switch cmd.Key {
		case "move":
			return "перенести в «" + target.Name + "»", nil
		case "close":
			return "закрыть задачу («" + target.Name + "»)", nil
		}
		return "переоткрыть задачу («" + target.Name + "»)", nil
	case "archive":
		return "архивировать задачу", nil

	// A list of task references, resolved the way qaRelation resolves it: a
	// partial list, not an all-or-nothing check. It refused the whole command at
	// the first unknown #N while the run linked everything it found and named the
	// rest as skipped — so «/relate #2591 #999999» was painted red and then went
	// on to create the link to #2591. The same divergence /assign and /tag had.
	case "relate", "blocks", "blocked_by", "duplicates", "unlink":
		refs, err := quickact.ParseRefs(cmd.Arg)
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		var part partialList
		for _, num := range refs {
			label := fmt.Sprintf("#%d", num)
			if !h.taskNumberExists(c, wsID, num) {
				part.missing = append(part.missing, label)
				continue
			}
			part.done = append(part.done, label)
		}
		if len(part.done) == 0 {
			return "", userErr("не найдено: %s", part.missed())
		}
		labels := map[string]string{
			"relate": "связать с", "blocks": "блокирует", "blocked_by": "заблокирована",
			"duplicates": "дублирует", "unlink": "снять связь с",
		}
		return part.summary(labels[cmd.Key], "пропущено"), nil

	// /parent is not a list even though its argument parses like one: qaParent
	// re-parents under refs[0] and ignores the rest. Checking every reference made
	// «/parent #2591 #999999» a red «задача #999999 не найдена» about a number the
	// run never looks at.
	case "parent":
		refs, err := quickact.ParseRefs(cmd.Arg)
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		if !h.taskNumberExists(c, wsID, refs[0]) {
			return "", userErr("задача #%d не найдена", refs[0])
		}
		return fmt.Sprintf("сделать подзадачей #%d", refs[0]), nil

	case "subtask":
		if strings.TrimSpace(cmd.Arg) == "" {
			return "", userErr("не указан заголовок подзадачи")
		}
		return "создать подзадачу «" + strings.TrimSpace(cmd.Arg) + "»", nil
	case "unparent":
		return "открепить от родителя", nil
	}
	return "", userErr("команда /%s пока не поддерживается", cmd.Key)
}
