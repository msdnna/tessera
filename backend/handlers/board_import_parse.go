package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── Board import: file parsing (#3068, #3095) ───────────────
// Pure functions: the file → rows of named cells, and the cells → task fields.
// The client parses the same file for its preview (utils/taskImport.js); this is
// the authoritative pass.

// maxImportRows caps one import.
const maxImportRows = 5000

// importFields are the task fields a column of the file can be mapped onto.
var importFields = []string{"title", "description", "column", "tags", "assignees", "priority", "start_date", "due_date"}

// importTable is a parsed file: the column names in file order and the rows,
// each a column → cell map.
type importTable struct {
	Columns []string
	Rows    []map[string]string
	// Lines is each row's place in the file, for the log's «Строка N»: the CSV
	// line it starts on (blank rows skipped), the JSON item's number.
	Lines []int
}

var errImportEmpty = errors.New("файл пуст")

// parseImportFile reads a CSV or JSON file into a table. format is "csv" or
// "json"; anything else is decided by the content.
func parseImportFile(data []byte, format string) (importTable, error) {
	data = bytes.TrimPrefix(data, []byte("\uFEFF"))
	if len(bytes.TrimSpace(data)) == 0 {
		return importTable{}, errImportEmpty
	}
	if format == "" {
		if t := bytes.TrimSpace(data); t[0] == '[' || t[0] == '{' {
			format = "json"
		} else {
			format = "csv"
		}
	}
	if format == "json" {
		return parseImportJSON(data)
	}
	return parseImportCSV(data)
}

// sniffDelimiter picks the separator of a CSV header line: the most frequent of
// comma, semicolon and tab outside quotes (a spreadsheet in a Russian locale saves
// with semicolons).
func sniffDelimiter(data []byte) rune {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	counts := map[rune]int{',': 0, ';': 0, '\t': 0}
	inQuote := false
	for _, r := range string(line) {
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			if _, ok := counts[r]; ok {
				counts[r]++
			}
		}
	}
	best, n := ',', counts[',']
	for _, r := range []rune{';', '\t'} {
		if counts[r] > n {
			best, n = r, counts[r]
		}
	}
	return best
}

func parseImportCSV(data []byte) (importTable, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sniffDelimiter(data)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return importTable{}, fmt.Errorf("не удалось прочитать заголовок CSV: %w", err)
	}
	cols := uniqueColumns(header)
	t := importTable{Columns: cols}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return importTable{}, fmt.Errorf("ошибка CSV в строке %d: %w", len(t.Rows)+2, err)
		}
		if blankRecord(rec) {
			continue
		}
		if len(t.Rows) >= maxImportRows {
			return importTable{}, fmt.Errorf("в файле больше %d строк", maxImportRows)
		}
		row := make(map[string]string, len(cols))
		for i, c := range cols {
			if i < len(rec) {
				row[c] = strings.TrimSpace(rec[i])
			}
		}
		line, _ := r.FieldPos(0)
		t.Rows = append(t.Rows, row)
		t.Lines = append(t.Lines, line)
	}
	return t, nil
}

func blankRecord(rec []string) bool {
	for _, v := range rec {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// uniqueColumns trims the header and names blank or repeated columns so every
// column has a distinct key.
func uniqueColumns(header []string) []string {
	seen := map[string]int{}
	out := make([]string, len(header))
	for i, h := range header {
		name := strings.TrimSpace(h)
		if name == "" {
			name = "column " + strconv.Itoa(i+1)
		}
		if n := seen[name]; n > 0 {
			seen[name] = n + 1
			name = fmt.Sprintf("%s (%d)", name, n+1)
		} else {
			seen[name] = 1
		}
		out[i] = name
	}
	return out
}

// parseImportJSON accepts an array of objects or an object holding one under
// "tasks" (the shape our own export writes). Arrays inside a cell become a
// comma-separated list; nested objects are skipped.
func parseImportJSON(data []byte) (importTable, error) {
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		var wrapped struct {
			Tasks []map[string]any `json:"tasks"`
		}
		if werr := json.Unmarshal(data, &wrapped); werr != nil || wrapped.Tasks == nil {
			return importTable{}, errors.New("JSON должен быть массивом объектов или объектом с полем tasks")
		}
		items = wrapped.Tasks
	}
	if len(items) > maxImportRows {
		return importTable{}, fmt.Errorf("в файле больше %d задач", maxImportRows)
	}
	colSet := map[string]bool{}
	var cols []string
	t := importTable{}
	for _, it := range items {
		row := map[string]string{}
		keys := make([]string, 0, len(it))
		for k := range it {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, ok := jsonCell(it[k])
			if !ok {
				continue
			}
			if !colSet[k] {
				colSet[k] = true
				cols = append(cols, k)
			}
			row[k] = v
		}
		t.Rows = append(t.Rows, row)
		t.Lines = append(t.Lines, len(t.Rows))
	}
	t.Columns = cols
	return t, nil
}

func jsonCell(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case string:
		return strings.TrimSpace(x), true
	case bool:
		return strconv.FormatBool(x), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := jsonCell(e); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", "), true
	}
	return "", false
}

// splitImportList splits a tags/assignees cell on commas and semicolons.
func splitImportList(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(f))
	seen := map[string]bool{}
	for _, p := range f {
		p = strings.TrimSpace(p)
		k := strings.ToLower(p)
		if p != "" && !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

// parseImportPriority understands our export names, their Russian counterparts and
// the stored numbers 0–4. ok is false for anything else (the caller warns and
// leaves the priority unset).
func parseImportPriority(s string) (int32, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, true
	}
	switch s {
	case "none", "нет", "без приоритета", "-":
		return 0, true
	case "low", "низкий":
		return 1, true
	case "normal", "medium", "средний", "обычный", "нормальный":
		return 2, true
	case "high", "высокий":
		return 3, true
	case "urgent", "critical", "срочный", "критический":
		return 4, true
	}
	if n, err := strconv.ParseInt(s, 10, 32); err == nil && n >= 0 && n <= 4 {
		return int32(n), true
	}
	return 0, false
}

// parseImportDate understands ISO dates (with or without a time), RFC 3339 and
// the day-first dotted form. A date without a time is UTC midnight — the app's
// convention for a date-only value.
func parseImportDate(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, true
		}
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2.1.2006", "02/01/2006"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return &t, true
		}
	}
	return nil, false
}
