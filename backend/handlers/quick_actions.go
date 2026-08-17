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
	var done []string
	var missing []string
	for _, login := range logins {
		member, ok := matchMember(members, login)
		if !ok {
			missing = append(missing, "@"+login)
			continue
		}
		if err := h.applyAssignee(c, t, wsID, member, add); err != nil {
			return "", err
		}
		done = append(done, "@"+login)
	}
	if len(done) == 0 {
		return "", userErr("не нашёл участника: %s", strings.Join(missing, ", "))
	}
	verb := "назначен"
	if !add {
		verb = "снят"
	}
	summary := verb + " " + strings.Join(done, ", ")
	if len(missing) > 0 {
		// Partial success is still success — say who was skipped rather than
		// failing the whole command and losing the assignments that worked.
		summary += " (не найдены: " + strings.Join(missing, ", ") + ")"
	}
	return summary, nil
}

// matchMember resolves a mention to a workspace member by GitLab login, email
// local-part, full email or display name (all case-insensitive) — the same
// shapes the frontend's @-autocomplete offers. The GitLab login is what the
// autocomplete now inserts for OAuth-linked members, so `/assign @e.polyansky`
// has to resolve even when it matches neither the email nor the name.
func matchMember(members []db.ListMembersRow, login string) (uuid.UUID, bool) {
	login = strings.ToLower(login)
	for _, m := range members {
		email := strings.ToLower(m.Email)
		if email == login || strings.ToLower(m.Name) == login {
			return m.UserID, true
		}
		if m.GlUsername != "" && strings.ToLower(m.GlUsername) == login {
			return m.UserID, true
		}
		if local, _, ok := strings.Cut(email, "@"); ok && local == login {
			return m.UserID, true
		}
	}
	return uuid.Nil, false
}

// qaPatch handles the commands that are a partial edit of the task's own fields.
func (h *API) qaPatch(c *gin.Context, t *db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	var patch taskPatch
	var summary string

	switch cmd.Key {
	case "due", "start":
		when, err := quickact.ParseDate(cmd.Arg, time.Now())
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
		hpd, dpw := h.estimateUnits(c, *t, wsID)
		v, err := quickact.ParseEstimate(cmd.Arg, hpd, dpw)
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
	var done, missing []string
	for _, name := range names {
		tag, ok := matchTag(tags, name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		if err := h.applyTag(c, t.ID, wsID, tag, add); err != nil {
			return "", err
		}
		done = append(done, name)
	}
	if len(done) == 0 {
		return "", userErr("нет такого тега в проекте: %s", strings.Join(missing, ", "))
	}
	verb := "добавлен тег"
	if !add {
		verb = "убран тег"
	}
	summary := verb + " " + strings.Join(done, ", ")
	if len(missing) > 0 {
		summary += " (нет в проекте: " + strings.Join(missing, ", ") + ")"
	}
	return summary, nil
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

	var target *db.BoardColumn
	var summary string
	switch cmd.Key {
	case "move":
		name := strings.TrimSpace(cmd.Arg)
		if name == "" {
			return "", userErr("не указана колонка")
		}
		for i := range cols {
			if strings.EqualFold(strings.TrimSpace(cols[i].Name), name) {
				target = &cols[i]
				break
			}
		}
		if target == nil {
			return "", userErr("нет колонки «%s» на этой доске", name)
		}
		summary = "перенесена в «" + target.Name + "»"
	case "close", "reopen":
		board, berr := h.q.GetBoard(c, t.BoardID)
		if berr != nil {
			return "", berr
		}
		doneID := doneColumnID(board)
		if doneID != nil {
			if cmd.Key == "close" {
				for i := range cols {
					if cols[i].ID == *doneID {
						target = &cols[i]
					}
				}
			} else if t.ColumnID == *doneID {
				// Reopening out of the done column: the leftmost column is where
				// the board puts new work.
				for i := range cols {
					if cols[i].ID != *doneID {
						target = &cols[i]
						break
					}
				}
			}
		}
		if target == nil {
			// No done column configured (or already out of it) — toggle the flag.
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
		if cmd.Key == "close" {
			summary = "задача закрыта («" + target.Name + "»)"
		} else {
			summary = "задача переоткрыта («" + target.Name + "»)"
		}
	}

	if target.ID == t.ColumnID {
		return "", userErr("задача уже в «%s»", target.Name)
	}
	// Append to the target column (or to the parent's subtask list): a quick
	// action has no drag neighbours, and positionBetween(nil, nil) would drop the
	// task onto the same position as everything else appended that way.
	pos, err := h.nextTaskPosition(c, target.ID, t.ParentID)
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
		} else if rerr := h.applyRelation(c, t, wsID, target, cmd.Key); rerr != nil {
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

// estimateUnits resolves the hours-per-day / days-per-week in force for a task,
// following the same project → workspace → built-in (8h/5d) chain the clients
// use, so "/estimate 1d" means what "1d" typed into the UI means.
func (h *API) estimateUnits(c *gin.Context, t db.Task, wsID uuid.UUID) (hoursPerDay, daysPerWeek float64) {
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
	hoursPerDay, daysPerWeek = 8, 5
	if raw == nil {
		return
	}
	var cfg estimationConfig
	if err := json.Unmarshal(*raw, &cfg); err != nil {
		return
	}
	if cfg.HoursPerDay > 0 {
		hoursPerDay = cfg.HoursPerDay
	}
	if cfg.DaysPerWeek > 0 {
		daysPerWeek = cfg.DaysPerWeek
	}
	return
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
				return "снять всех исполнителей", nil
			}
			return "", userErr("не указан исполнитель (например @msdnna)")
		}
		members, err := h.q.ListMembers(c, wsID)
		if err != nil {
			return "", err
		}
		for _, login := range logins {
			if _, ok := matchMember(members, login); !ok {
				return "", userErr("не нашёл участника: @%s", login)
			}
		}
		if cmd.Key == "assign" {
			return "назначить @" + strings.Join(logins, ", @"), nil
		}
		return "снять @" + strings.Join(logins, ", @"), nil

	case "due", "start":
		when, err := quickact.ParseDate(cmd.Arg, time.Now())
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
		hpd, dpw := h.estimateUnits(c, t, wsID)
		if _, err := quickact.ParseEstimate(cmd.Arg, hpd, dpw); err != nil {
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
		for _, name := range names {
			if _, ok := matchTag(tags, name); !ok {
				return "", userErr("нет такого тега в проекте: %s", name)
			}
		}
		if cmd.Key == "tag" {
			return "добавить тег " + strings.Join(names, ", "), nil
		}
		return "убрать тег " + strings.Join(names, ", "), nil

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

	case "move":
		name := strings.TrimSpace(cmd.Arg)
		if name == "" {
			return "", userErr("не указана колонка")
		}
		cols, err := h.q.ListColumns(c, t.BoardID)
		if err != nil {
			return "", err
		}
		for _, col := range cols {
			if strings.EqualFold(strings.TrimSpace(col.Name), name) {
				return "перенести в «" + col.Name + "»", nil
			}
		}
		return "", userErr("нет колонки «%s» на этой доске", name)
	case "close":
		return "закрыть задачу", nil
	case "reopen":
		return "переоткрыть задачу", nil
	case "archive":
		return "архивировать задачу", nil

	case "relate", "blocks", "blocked_by", "duplicates", "unlink", "parent":
		refs, err := quickact.ParseRefs(cmd.Arg)
		if err != nil {
			return "", opError{msg: err.Error()}
		}
		for _, num := range refs {
			n := int64(num)
			if _, err := h.q.GetTaskByNumber(c, db.GetTaskByNumberParams{WorkspaceID: wsID, Number: &n}); err != nil {
				return "", userErr("задача #%d не найдена", num)
			}
		}
		labels := map[string]string{
			"relate": "связать с", "blocks": "блокирует", "blocked_by": "заблокирована",
			"duplicates": "дублирует", "unlink": "снять связь с", "parent": "сделать подзадачей",
		}
		var parts []string
		for _, num := range refs {
			parts = append(parts, fmt.Sprintf("#%d", num))
		}
		return labels[cmd.Key] + " " + strings.Join(parts, ", "), nil

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
