package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/jobs"
	"tessera/middleware"
)

// ── Board export (#3068, #3094) ─────────────────────────────
// An export is a background job: the request queues it and answers 202 with the
// run; the job writes the file into uploadDir/exports and the member downloads it
// through GET /jobs/:id/download once it is done. A small board is done before
// the client's first look; a big one shows progress in the toast and, for an
// admin, in the jobs window.

// exportFields is the column set of an export, in order. The names are stable
// keys (not translated) so a file exported here imports back with the columns
// recognised.
var exportFields = []string{
	"number", "title", "description", "column", "priority", "tags", "assignees",
	"start_date", "due_date", "estimate", "milestone", "parent", "completed_at", "created_at",
}

// priorityNames are the stored priorities' export names (0 none … 4 urgent).
var priorityNames = []string{"none", "low", "normal", "high", "urgent"}

// exportPath is where a run's file lives; the format is the extension. Callers
// pass a format that has been through exportFormat.
func (h *API) exportPath(runID, format string) string {
	return filepath.Join(h.exportDir(), runID+"."+format)
}

// exportFormat maps a format named by a client (or read back from a run's meta)
// onto the extension its file is stored under. The result is always one of the
// constants below, never the caller's own string, so nothing a request carries
// can reach the file name.
func exportFormat(s string) (string, bool) {
	switch s {
	case "csv":
		return "csv", true
	case "json":
		return "json", true
	}
	return "", false
}

type exportRequest struct {
	Format          string      `json:"format"`
	TaskIDs         []uuid.UUID `json:"task_ids"`
	IncludeSubtasks *bool       `json:"include_subtasks"`
}

// ExportBoard queues an export of a board's tasks to CSV or JSON. task_ids, when
// sent, narrows it to the cards the client shows under its current view filters
// (plus their subtasks unless include_subtasks is false); absent, the whole board.
func (h *API) ExportBoard(c *gin.Context) {
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
	if !h.requireMember(c, wsID) {
		return
	}
	var req exportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	format, ok := exportFormat(req.Format)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be csv or json"})
		return
	}
	subtasks := req.IncludeSubtasks == nil || *req.IncludeSubtasks
	uid := middleware.CurrentUser(c)
	key := "export:" + uuid.NewString()
	_, started := h.jobs.Start(jobs.Spec{
		Key:  key,
		Name: jobs.Name{Key: "export_" + format, Arg: board.Name, Text: "Экспорт доски в " + strings.ToUpper(format) + " · " + board.Name},
		Kind: jobs.KindExport, Type: jobs.TypeImportExport, WorkspaceID: wsID.String(), ActorID: uid.String(),
		Trigger: "manual", MaxAttempts: 3, Queued: true,
		Meta: map[string]any{"board_id": boardID.String(), "board_name": board.Name, "format": format, "filtered": req.TaskIDs != nil},
	}, h.runDetached, func(ctx context.Context, jh *jobs.Handle) error {
		return h.runExport(ctx, jh, board, format, req.TaskIDs, subtasks)
	})
	if !started {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server is shutting down"})
		return
	}
	e, _ := h.jobs.Get(key)
	c.JSON(http.StatusAccepted, gin.H{"job": entryToDTO(e)})
}

// runExport is the export job's body.
func (h *API) runExport(ctx context.Context, jh *jobs.Handle, board db.Board, format string, ids []uuid.UUID, subtasks bool) error {
	jh.Steps(
		jobs.Step{Key: "collect", Label: "Сбор задач"},
		jobs.Step{Key: "write", Label: "Запись файла"},
	)
	jh.Step("collect")
	if _, err := h.q.GetBoard(ctx, board.ID); err != nil {
		return jobs.Permanent(errors.New("доска удалена"))
	}
	rows, err := h.q.ExportBoardTasks(ctx, board.ID)
	if err != nil {
		return err
	}
	if ids != nil {
		rows = filterExportRows(rows, ids, subtasks)
	}
	jh.StepCount(len(rows), 0)
	jh.Logf(jobs.LevelInfo, "Доска «%s»: задач к выгрузке — %d", board.Name, len(rows))

	jh.Step("write")
	if err := os.MkdirAll(h.exportDir(), 0o750); err != nil {
		return err
	}
	path := h.exportPath(jh.RunID(), format)
	tmp := path + ".tmp"
	f, err := os.Create(tmp) // #nosec G304 -- name is the run id under our own upload dir
	if err != nil {
		return err
	}
	progress := func(i int) {
		jh.Progress(i, len(rows))
		jh.StepCount(i, len(rows))
	}
	if format == "csv" {
		err = writeExportCSV(f, rows, progress)
	} else {
		err = writeExportJSON(f, board, rows, progress)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	info, _ := os.Stat(path)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	name := exportFileName(board, format, time.Now())
	jh.SetMeta("file_name", name)
	jh.SetMeta("file_size", size)
	jh.SetMeta("rows", len(rows))
	jh.Progress(len(rows), len(rows))
	jh.Logf(jobs.LevelInfo, "Файл %s готов: %d задач, %s", name, len(rows), humanBytes(size))
	return nil
}

// filterExportRows keeps the listed tasks and, with subtasks, every task under
// one of them (at any depth).
func filterExportRows(rows []db.ExportBoardTasksRow, ids []uuid.UUID, subtasks bool) []db.ExportBoardTasksRow {
	keep := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		keep[id] = true
	}
	if subtasks {
		for changed := true; changed; {
			changed = false
			for _, r := range rows {
				if !keep[r.ID] && r.ParentID != nil && keep[*r.ParentID] {
					keep[r.ID] = true
					changed = true
				}
			}
		}
	}
	out := rows[:0:0]
	for _, r := range rows {
		if keep[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// exportDate renders a task date: a date-only value (UTC midnight, the way the
// app stores a date without a time) as YYYY-MM-DD, anything else as RFC 3339.
func exportDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	u := t.UTC()
	if u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0 {
		return u.Format("2006-01-02")
	}
	return u.Format(time.RFC3339)
}

func exportPriority(p int32) string {
	if p >= 0 && int(p) < len(priorityNames) {
		return priorityNames[p]
	}
	return strconv.Itoa(int(p))
}

func exportNumber(n *int64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatInt(*n, 10)
}

func exportRecord(r db.ExportBoardTasksRow) []string {
	estimate := ""
	if r.Estimate != nil {
		estimate = strconv.FormatFloat(*r.Estimate, 'f', -1, 64)
	}
	completed := ""
	if r.CompletedAt != nil {
		completed = r.CompletedAt.UTC().Format(time.RFC3339)
	}
	return []string{
		exportNumber(r.Number), r.Title, r.Description, r.ColumnName, exportPriority(r.Priority),
		strings.Join(r.TagNames, ", "), strings.Join(r.AssigneeEmails, ", "),
		exportDate(r.StartDate), exportDate(r.DueDate), estimate, r.MilestoneName,
		exportNumber(r.ParentNumber), completed, r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// writeExportCSV writes RFC 4180 CSV with a UTF-8 byte-order mark, so a
// spreadsheet opens the Cyrillic right on a double click.
func writeExportCSV(f *os.File, rows []db.ExportBoardTasksRow, progress func(int)) error {
	if _, err := f.WriteString("\uFEFF"); err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write(exportFields); err != nil {
		return err
	}
	for i, r := range rows {
		if err := w.Write(exportRecord(r)); err != nil {
			return err
		}
		progress(i + 1)
	}
	w.Flush()
	return w.Error()
}

// exportTask is one task of a JSON export.
type exportTask struct {
	Number      *int64   `json:"number"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Column      string   `json:"column"`
	Priority    string   `json:"priority"`
	Tags        []string `json:"tags"`
	Assignees   []string `json:"assignees"`
	StartDate   string   `json:"start_date,omitempty"`
	DueDate     string   `json:"due_date,omitempty"`
	Estimate    *float64 `json:"estimate,omitempty"`
	Milestone   string   `json:"milestone,omitempty"`
	Parent      *int64   `json:"parent,omitempty"`
	CompletedAt string   `json:"completed_at,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

func writeExportJSON(f *os.File, board db.Board, rows []db.ExportBoardTasksRow, progress func(int)) error {
	tasks := make([]exportTask, 0, len(rows))
	for i, r := range rows {
		et := exportTask{
			Number: r.Number, Title: r.Title, Description: r.Description, Column: r.ColumnName,
			Priority: exportPriority(r.Priority), Tags: nonNilStrings(r.TagNames), Assignees: nonNilStrings(r.AssigneeEmails),
			StartDate: exportDate(r.StartDate), DueDate: exportDate(r.DueDate), Estimate: r.Estimate,
			Milestone: r.MilestoneName, Parent: r.ParentNumber, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		}
		if r.CompletedAt != nil {
			et.CompletedAt = r.CompletedAt.UTC().Format(time.RFC3339)
		}
		tasks = append(tasks, et)
		progress(i + 1)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(gin.H{
		"board":       gin.H{"id": board.ID, "name": board.Name},
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"tasks":       tasks,
	})
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// exportFileName is the download's name: the board's address and the date.
func exportFileName(board db.Board, format string, now time.Time) string {
	base := board.Slug
	if base == "" {
		base = "board"
	}
	return fmt.Sprintf("%s-%s.%s", base, now.Format("2006-01-02"), format)
}

// DownloadJobFile serves a finished export's file to the member who ran it (or a
// global admin). 410 once the file is past its retention.
func (h *API) DownloadJobFile(c *gin.Context) {
	d, ok := h.ownJob(c)
	if !ok {
		return
	}
	if d.Kind != jobs.KindExport || d.Status != jobs.StatusDone {
		c.JSON(http.StatusConflict, gin.H{"error": "export is not ready"})
		return
	}
	if ws := optUUID(d.WorkspaceID); ws != nil && !h.requireMember(c, *ws) {
		return
	}
	stored, _ := d.Meta["format"].(string)
	format, ok := exportFormat(stored)
	if !ok {
		c.JSON(http.StatusGone, gin.H{"error": "export file is gone"})
		return
	}
	path := h.exportPath(d.RunID, format)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "export file is gone"})
		return
	}
	name, _ := d.Meta["file_name"].(string)
	if name == "" {
		name = "export." + format
	}
	if format == "csv" {
		c.Header("Content-Type", "text/csv; charset=utf-8")
	} else {
		c.Header("Content-Type", "application/json; charset=utf-8")
	}
	c.FileAttachment(path, name)
}
