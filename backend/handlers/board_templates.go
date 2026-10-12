package handlers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/i18n"
)

// Board templates for an empty project (#3047, mockup CatalogE «Пустой проект»):
// «Канбан спринта», «Баг-трекер», «Роадмап» — a board born with its columns, the
// view it opens in and the project tags it is worked with, instead of the four
// default columns.
//
// Built-in presets in code, not rows. Same reasoning as the built-in document
// templates (frontend utils/docTemplates.js): a seeded row is a per-workspace
// copy that ages on its own and needs a data migration to improve, while three
// constants cost nothing and are the same for everyone. A team that wants its
// own set builds a board and keeps it.
//
// Columns carry a name_key like the default four, so each client captions them
// in its reader's language (board.column.defaultName.* on web,
// default_column_* on Android) and `/move` recognises the caption the user sees
// (seededColumnCaptions). Tags have no key — a tag is user data and its name is
// what grouping by tag shows — so they are written in the creator's language.
type boardTemplateColumn struct{ name, nameKey, color string }

type boardTemplateTag struct {
	names map[string]string // language → name
	color string
}

type boardTemplate struct {
	layout  string
	columns []boardTemplateColumn // the last one is the board's done column
	tags    []boardTemplateTag
}

// v2 lane palette (#2951) plus the accent purple and the alert red.
const (
	laneTeal   = "#0eb0a9"
	laneBlue   = "#2f80ed"
	laneOrange = "#f0a020"
	laneGreen  = "#18a058"
	lanePurple = "#7c5cff"
	laneRed    = "#e0533d"
)

var boardTemplates = map[string]boardTemplate{
	// A sprint board: the default four behind a sprint backlog lane, and the
	// type of work as tags — what grouping the board by tag is for.
	"sprint": {
		layout: "board",
		columns: []boardTemplateColumn{
			{"Бэклог спринта", "backlog", lanePurple},
			{"К работе", "todo", laneTeal},
			{"В процессе", "in_progress", laneBlue},
			{"На рассмотрении", "review", laneOrange},
			{doneColumnName, "done", laneGreen},
		},
		tags: []boardTemplateTag{
			{map[string]string{"ru": "тип::фича", "en": "type::feature"}, laneBlue},
			{map[string]string{"ru": "тип::баг", "en": "type::bug"}, laneRed},
			{map[string]string{"ru": "тип::техдолг", "en": "type::tech-debt"}, laneOrange},
		},
	},
	// A bug tracker opens as a list (the mockup's glyph) and sorts by severity.
	"bugs": {
		layout: "list",
		columns: []boardTemplateColumn{
			{"Новые", "new", laneRed},
			{"Подтверждены", "confirmed", laneOrange},
			{"В процессе", "in_progress", laneBlue},
			{"Проверка", "verify", lanePurple},
			{"Закрыты", "closed", laneGreen},
		},
		tags: []boardTemplateTag{
			{map[string]string{"ru": "серьёзность::критическая", "en": "severity::critical"}, laneRed},
			{map[string]string{"ru": "серьёзность::высокая", "en": "severity::high"}, laneOrange},
			{map[string]string{"ru": "серьёзность::средняя", "en": "severity::medium"}, laneBlue},
			{map[string]string{"ru": "серьёзность::низкая", "en": "severity::low"}, laneTeal},
		},
	},
	// A roadmap opens on the timeline and is planned by quarter.
	"roadmap": {
		layout: "timeline",
		columns: []boardTemplateColumn{
			{"Идеи", "ideas", lanePurple},
			{"Запланировано", "planned", laneTeal},
			{"В процессе", "in_progress", laneBlue},
			{"Выпущено", "shipped", laneGreen},
		},
		tags: []boardTemplateTag{
			{map[string]string{"ru": "квартал::Q1", "en": "quarter::Q1"}, laneTeal},
			{map[string]string{"ru": "квартал::Q2", "en": "quarter::Q2"}, laneBlue},
			{map[string]string{"ru": "квартал::Q3", "en": "quarter::Q3"}, lanePurple},
			{map[string]string{"ru": "квартал::Q4", "en": "quarter::Q4"}, laneOrange},
		},
	},
}

// defaultBoardTemplate is the plain board: the four default columns, no tags.
func defaultBoardTemplate() boardTemplate {
	cols := make([]boardTemplateColumn, 0, len(defaultColumns))
	for _, dc := range defaultColumns {
		cols = append(cols, boardTemplateColumn{dc.name, dc.nameKey, dc.color})
	}
	return boardTemplate{layout: "board", columns: cols}
}

// boardTemplateFor resolves the create request's template key; "" is the plain
// board. ok=false for a key nobody knows.
func boardTemplateFor(key string) (boardTemplate, bool) {
	if key == "" {
		return defaultBoardTemplate(), true
	}
	tpl, ok := boardTemplates[key]
	return tpl, ok
}

// seedTemplateTags adds the template's tags to the project in lang, skipping
// names the project already has. Returns the tags it created, for the broadcast.
func seedTemplateTags(ctx context.Context, q *db.Queries, project db.Project, tpl boardTemplate, lang string) ([]db.Tag, error) {
	var created []db.Tag
	for _, tt := range tpl.tags {
		name := tt.names[lang]
		if name == "" {
			name = tt.names[i18n.Default]
		}
		tag, err := q.SeedTag(ctx, db.SeedTagParams{
			WorkspaceID: project.WorkspaceID, ProjectID: project.ID, Name: name, Color: tt.color,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // the project already has it
		}
		if err != nil {
			return nil, err
		}
		created = append(created, tag)
	}
	return created, nil
}

// seedBoardColumns creates the template's columns on a new board and makes the
// last one its done column. Returns the board as updated.
func seedBoardColumns(ctx context.Context, q *db.Queries, b db.Board, tpl boardTemplate) (db.Board, error) {
	var lastColID *uuid.UUID
	for i, tc := range tpl.columns {
		col, err := q.CreateColumn(ctx, db.CreateColumnParams{
			BoardID: b.ID, Name: tc.name, NameKey: ptr(tc.nameKey),
			Color: tc.color, Position: float64(i+1) * positionGap,
		})
		if err != nil {
			return b, err
		}
		id := col.ID
		lastColID = &id
	}
	if lastColID == nil {
		return b, nil
	}
	return q.SetBoardDoneColumn(ctx, db.SetBoardDoneColumnParams{ID: b.ID, DoneColumnID: lastColID})
}
