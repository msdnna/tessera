package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/jobs"
	"tessera/middleware"
)

// ── Board import (#3068, #3095) ─────────────────────────────
// The client reads the file itself for the column mapping and the preview, then
// uploads it with the mapping. The server parses it again (authoritatively),
// refuses what can't work up front (400), and queues a job that creates the
// tasks row by row, each in its own transaction — a retry resumes after the last
// row that landed instead of creating the first ones twice.

// MaxBoardImportBytes is the largest file an import accepts. The route's body budget
// (router bodyLimitRoutes) is the attachment limit, which is larger; this is the
// handler's own, stricter ceiling.
const MaxBoardImportBytes = 10 << 20

// importOptions are the choices of the mapping screen besides the mapping itself.
type importOptions struct {
	// ColumnID receives the rows whose «Колонка» is empty or matches no column
	// (default: the board's first column).
	ColumnID *uuid.UUID `json:"column_id"`
	// CreateTags creates a tag the project doesn't have yet (default true);
	// false drops unknown tags with a warning.
	CreateTags *bool `json:"create_tags"`
}

// ImportBoard queues an import of tasks from an uploaded CSV/JSON file onto a
// board. Form fields: file, mapping (JSON object task field → file column),
// options (JSON importOptions, optional).
func (h *API) ImportBoard(c *gin.Context) {
	boardID, ok := parseID(c, "id")
	if !ok {
		return
	}
	board, err := h.q.GetBoard(c, boardID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, boardID)
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) || !h.requireProjectWritable(c, boardID) {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не передан"})
		return
	}
	if fileHeader.Size > MaxBoardImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "файл больше 10 МБ"})
		return
	}
	src, err := readUpload(fileHeader, MaxBoardImportBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	format := ""
	switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
	case ".csv", ".tsv", ".txt":
		format = "csv"
	case ".json":
		format = "json"
	}
	table, err := parseImportFile(src, format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var mapping map[string]string
	if err := json.Unmarshal([]byte(c.PostForm("mapping")), &mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mapping: ожидается JSON-объект «поле → колонка файла»"})
		return
	}
	mapping = cleanMapping(mapping, table.Columns)
	if mapping["title"] == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "не выбрана колонка с заголовком задачи"})
		return
	}
	var opts importOptions
	if raw := c.PostForm("options"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "options: " + err.Error()})
			return
		}
	}
	if opts.ColumnID != nil {
		col, cerr := h.q.GetColumn(c, *opts.ColumnID)
		if cerr != nil || col.BoardID != boardID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "column does not belong to this board"})
			return
		}
	}
	if len(table.Rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "в файле нет строк с задачами"})
		return
	}

	uid := middleware.CurrentUser(c)
	key := "import:" + uuid.NewString()
	imp := &boardImport{
		h: h, board: board, wsID: wsID, actor: uid, table: table, mapping: mapping, opts: opts,
		fileName: filepath.Base(fileHeader.Filename),
	}
	_, started := h.jobs.Start(jobs.Spec{
		Key:  key,
		Name: jobs.Name{Key: "import", Arg: board.Name, Text: "Импорт задач · " + board.Name},
		Kind: jobs.KindImport, Type: jobs.TypeImportExport, WorkspaceID: wsID.String(), ActorID: uid.String(),
		Trigger: "manual", MaxAttempts: 3, Queued: true,
		Meta: map[string]any{"board_id": boardID.String(), "board_name": board.Name, "file_name": imp.fileName, "rows": len(table.Rows)},
	}, h.runDetached, imp.run)
	if !started {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server is shutting down"})
		return
	}
	e, _ := h.jobs.Get(key)
	c.JSON(http.StatusAccepted, gin.H{"job": entryToDTO(e)})
}

// cleanMapping keeps the known task fields mapped onto columns the file has.
func cleanMapping(m map[string]string, cols []string) map[string]string {
	have := make(map[string]bool, len(cols))
	for _, c := range cols {
		have[c] = true
	}
	out := map[string]string{}
	for _, f := range importFields {
		if col := m[f]; col != "" && have[col] {
			out[f] = col
		}
	}
	return out
}

// boardImport is one import's state, shared across its attempts: next is the
// first row not yet created, so a retry picks up where the failed attempt
// stopped.
type boardImport struct {
	h        *API
	board    db.Board
	wsID     uuid.UUID
	actor    uuid.UUID
	table    importTable
	mapping  map[string]string
	opts     importOptions
	fileName string

	next    int
	created int
	skipped int

	columns   []db.ImportBoardColumnsRow
	projectID uuid.UUID
	tags      map[string]uuid.UUID // lower(name) → id
	members   map[string]uuid.UUID // lower(email) and lower(name) → user id
}

func (b *boardImport) cell(row map[string]string, field string) string {
	col := b.mapping[field]
	if col == "" {
		return ""
	}
	return strings.TrimSpace(row[col])
}

func (b *boardImport) run(ctx context.Context, jh *jobs.Handle) error {
	h := b.h
	jh.Steps(
		jobs.Step{Key: "prepare", Label: "Подготовка"},
		jobs.Step{Key: "create", Label: "Создание задач"},
		jobs.Step{Key: "refresh", Label: "Обновление доски"},
	)
	jh.Step("prepare")
	if _, err := h.q.GetBoard(ctx, b.board.ID); err != nil {
		return jobs.Permanent(errors.New("доска удалена"))
	}
	if archived, err := h.q.ProjectArchivedForBoard(ctx, b.board.ID); err == nil && archived {
		return jobs.Permanent(errors.New("проект в архиве"))
	}
	if err := b.prepare(ctx); err != nil {
		return err
	}
	if b.next == 0 {
		jh.Logf(jobs.LevelInfo, "Файл %s: строк — %d, колонка заголовка — «%s»", b.fileName, len(b.table.Rows), b.mapping["title"])
	} else {
		jh.Logf(jobs.LevelInfo, "Продолжение со строки %d", b.next+2)
	}

	jh.Step("create")
	total := len(b.table.Rows)
	for b.next < total {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := b.next + 2 // the header is line 1
		if err := b.importRow(ctx, jh, b.table.Rows[b.next], line); err != nil {
			return fmt.Errorf("строка %d: %w", line, err)
		}
		b.next++
		jh.Progress(b.next, total)
		jh.StepCount(b.next, total)
		jh.SetCounts(b.created, 0)
	}

	jh.Step("refresh")
	if b.created > 0 {
		h.broadcastByActor(b.wsID, "task.synced", b.actor, gin.H{"board_id": b.board.ID, "created": b.created, "updated": 0})
	}
	jh.SetMeta("created", b.created)
	jh.SetMeta("skipped", b.skipped)
	jh.Logf(jobs.LevelInfo, "Готово: создано задач — %d, пропущено строк — %d", b.created, b.skipped)
	return nil
}

// prepare loads what rows are matched against: the columns, the project's tags,
// the workspace's members.
func (b *boardImport) prepare(ctx context.Context) error {
	h := b.h
	cols, err := h.q.ImportBoardColumns(ctx, b.board.ID)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return jobs.Permanent(errors.New("на доске нет колонок"))
	}
	b.columns = cols
	if b.projectID, err = h.q.ProjectIDForBoard(ctx, b.board.ID); err != nil {
		return err
	}
	tags, err := h.q.ListTags(ctx, b.projectID)
	if err != nil {
		return err
	}
	b.tags = make(map[string]uuid.UUID, len(tags))
	for _, t := range tags {
		k := strings.ToLower(strings.TrimSpace(t.Name))
		if _, dup := b.tags[k]; !dup {
			b.tags[k] = t.ID
		}
	}
	members, err := h.q.ListMembers(ctx, b.wsID)
	if err != nil {
		return err
	}
	b.members = make(map[string]uuid.UUID, 2*len(members))
	for _, m := range members {
		b.members[strings.ToLower(m.Email)] = m.UserID
		if n := strings.ToLower(strings.TrimSpace(m.Name)); n != "" {
			if _, dup := b.members[n]; !dup {
				b.members[n] = m.UserID
			}
		}
	}
	return nil
}

// columnFor matches a row's «Колонка» to a board column: by name, then by the
// seeded column's key (a «To do» file onto a «К работе» board). An empty or
// unknown value lands in the default column.
func (b *boardImport) columnFor(v string) (uuid.UUID, bool) {
	def := b.columns[0].ID
	if b.opts.ColumnID != nil {
		def = *b.opts.ColumnID
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return def, true
	}
	for _, c := range b.columns {
		if strings.EqualFold(strings.TrimSpace(c.Name), v) {
			return c.ID, true
		}
	}
	if key := seededColumnKey(v); key != "" {
		for _, c := range b.columns {
			if c.NameKey != nil && *c.NameKey == key {
				return c.ID, true
			}
		}
	}
	return def, false
}

func (b *boardImport) importRow(ctx context.Context, jh *jobs.Handle, row map[string]string, line int) error {
	h := b.h
	title := normalizeTitle(b.cell(row, "title"))
	if title == "" {
		b.skipped++
		jh.Logf(jobs.LevelWarn, "Строка %d: пустой заголовок, пропущена", line)
		return nil
	}
	colID, matched := b.columnFor(b.cell(row, "column"))
	if !matched {
		jh.Logf(jobs.LevelWarn, "Строка %d: колонки «%s» нет на доске — задача в колонке по умолчанию", line, b.cell(row, "column"))
	}
	priority, ok := parseImportPriority(b.cell(row, "priority"))
	if !ok {
		jh.Logf(jobs.LevelWarn, "Строка %d: приоритет «%s» не распознан", line, b.cell(row, "priority"))
	}
	due, ok := parseImportDate(b.cell(row, "due_date"))
	if !ok {
		jh.Logf(jobs.LevelWarn, "Строка %d: срок «%s» не распознан", line, b.cell(row, "due_date"))
	}
	start, ok := parseImportDate(b.cell(row, "start_date"))
	if !ok {
		jh.Logf(jobs.LevelWarn, "Строка %d: дата начала «%s» не распознана", line, b.cell(row, "start_date"))
	}
	var assignees []uuid.UUID
	for _, a := range splitList(b.cell(row, "assignees")) {
		if id, ok := b.members[strings.ToLower(a)]; ok {
			assignees = append(assignees, id)
		} else {
			jh.Logf(jobs.LevelWarn, "Строка %d: «%s» не участник пространства — не назначен", line, a)
		}
	}
	tagNames := splitList(b.cell(row, "tags"))

	pos, err := h.nextTaskPosition(ctx, colID, nil)
	if err != nil {
		return err
	}
	num, err := h.q.NextWorkspaceTaskNumber(ctx, b.wsID)
	if err != nil {
		return err
	}
	actor := b.actor
	var task db.Task
	var newTags []db.Tag
	err = h.inTx(ctx, func(q *db.Queries) error {
		var terr error
		task, terr = q.CreateTask(ctx, db.CreateTaskParams{
			BoardID: b.board.ID, ColumnID: colID, Title: title, Description: b.cell(row, "description"),
			Priority: priority, DueDate: due, StartDate: start, Position: pos, CreatedBy: &actor, Number: &num,
		})
		if terr != nil {
			return terr
		}
		for _, name := range tagNames {
			id, known := b.tags[strings.ToLower(name)]
			if !known {
				if b.opts.CreateTags != nil && !*b.opts.CreateTags {
					continue
				}
				tag, cerr := q.CreateTag(ctx, db.CreateTagParams{WorkspaceID: b.wsID, ProjectID: b.projectID, Name: name})
				if cerr != nil {
					return cerr
				}
				id = tag.ID
				newTags = append(newTags, tag)
			}
			if _, aerr := q.AddTaskTag(ctx, db.AddTaskTagParams{TaskID: task.ID, TagID: id}); aerr != nil {
				return aerr
			}
		}
		for _, uid := range assignees {
			if aerr := q.AddTaskAssignee(ctx, db.AddTaskAssigneeParams{TaskID: task.ID, UserID: uid}); aerr != nil {
				return aerr
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Committed: the row is done whatever happens next.
	for _, t := range newTags {
		b.tags[strings.ToLower(t.Name)] = t.ID
		h.broadcast(b.wsID, "tag.created", t)
		jh.Logf(jobs.LevelInfo, "Новый тег «%s»", t.Name)
	}
	if b.opts.CreateTags != nil && !*b.opts.CreateTags {
		for _, name := range tagNames {
			if _, known := b.tags[strings.ToLower(name)]; !known {
				jh.Logf(jobs.LevelWarn, "Строка %d: тега «%s» нет в проекте — пропущен", line, name)
			}
		}
	}
	h.logEventActor(ctx, task.ID, b.actor, "created", map[string]any{"source": "import"})
	b.created++
	jh.Logf(jobs.LevelInfo, "#%d «%s»", num, title)
	return nil
}
