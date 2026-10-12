package tools

import (
	"context"
	"strings"
	"testing"

	"tessera-mcp/internal/model"
)

// Nothing makes a column or a tag name unique — board_columns carries no
// constraint at all, and the tags index is byte-exact while every reader
// compares case-insensitively — so a name two rows carry is a state the product
// reaches on its own. These pin what the resolvers do when it happens.

// TestResolveColumnRefusesAmbiguousName: two «Готово» on one board. The first in
// list order is the LEFTMOST, and the board's completing column is
// done_column_id (else the rightmost) — so guessing here is what silently put a
// card under the right heading with completed_at unset.
func TestResolveColumnRefusesAmbiguousName(t *testing.T) {
	c := fakeAPI(t, map[string]any{
		"/api/boards/b1/columns": []model.Column{
			{ID: "c1", Name: "К работе", Position: 1},
			{ID: "c2", Name: "Готово", Position: 2},
			{ID: "c3", Name: "готово ", Position: 3},
		},
	})
	_, err := resolveColumn(context.Background(), c, "b1", "Готово")
	if err == nil {
		t.Fatal("two columns read as «Готово»: the resolver must refuse, not pick one")
	}
	// The ids are the point of the message: they are what the caller can retype.
	for _, want := range []string{"c2", "c3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name candidate %s; got %q", want, err.Error())
		}
	}
}

// An exact id is authoritative even when a twin name exists — that is the escape
// hatch the refusal above points at, so it has to work.
func TestResolveColumnByIDBeatsAmbiguity(t *testing.T) {
	c := fakeAPI(t, map[string]any{
		"/api/boards/b1/columns": []model.Column{
			{ID: "c2", Name: "Готово", Position: 2},
			{ID: "c3", Name: "Готово", Position: 3},
		},
	})
	col, err := resolveColumn(context.Background(), c, "b1", "c3")
	if err != nil {
		t.Fatalf("resolve by id: %v", err)
	}
	if col.ID != "c3" {
		t.Fatalf("resolved %q, want c3", col.ID)
	}
}

// An unambiguous name still resolves, trimming and casing included — the
// refusal must not cost the ordinary call anything.
func TestResolveColumnUniqueNameStillResolves(t *testing.T) {
	c := fakeAPI(t, map[string]any{
		"/api/boards/b1/columns": []model.Column{
			{ID: "c1", Name: "К работе", Position: 1},
			{ID: "c2", Name: "Готово", Position: 2},
		},
	})
	col, err := resolveColumn(context.Background(), c, "b1", "  гОтОвО ")
	if err != nil {
		t.Fatalf("resolve by name: %v", err)
	}
	if col.ID != "c2" {
		t.Fatalf("resolved %q, want c2", col.ID)
	}
	// Empty want is still the leftmost by position, not by list order.
	col, err = resolveColumn(context.Background(), c, "b1", "")
	if err != nil || col.ID != "c1" {
		t.Fatalf("default column = %+v, %v", col, err)
	}
}

// TestResolveTagsRefusesAmbiguousName: attaching by a name two rows carry used to
// take whichever the API happened to list last (ORDER BY name leaves ties
// unspecified), so the same call landed on a different row on different days —
// and the board draws one lane per row.
func TestResolveTagsRefusesAmbiguousName(t *testing.T) {
	c := fakeAPI(t, map[string]any{
		"/api/projects/p1/tags": []model.Tag{
			{ID: "t1", Name: "Сложность::высокая"},
			{ID: "t2", Name: "сложность::высокая"},
			{ID: "t3", Name: "backend"},
		},
	})
	_, err := resolveTags(context.Background(), c, "p1", []string{"Сложность::высокая"}, false)
	if err == nil {
		t.Fatal("two tags read as the same name: the resolver must refuse, not pick one")
	}
	for _, want := range []string{"t1", "t2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name candidate %s; got %q", want, err.Error())
		}
	}
	// An unambiguous name is unaffected.
	tags, err := resolveTags(context.Background(), c, "p1", []string{"BACKEND"}, false)
	if err != nil || len(tags) != 1 || tags[0].ID != "t3" {
		t.Fatalf("unambiguous resolve = %+v, %v", tags, err)
	}
}

// TestMatchProjectTagsTakesEveryTwin: detaching is the tolerant half, so it
// resolves the same ambiguity the other way — the caller said "this tag must not
// be on the task" about a name, and both rows read as that name.
func TestMatchProjectTagsTakesEveryTwin(t *testing.T) {
	c := fakeAPI(t, map[string]any{
		"/api/projects/p1/tags": []model.Tag{
			{ID: "t1", Name: "Сложность::высокая"},
			{ID: "t2", Name: "сложность::высокая"},
			{ID: "t3", Name: "backend"},
		},
	})
	tags, err := matchProjectTags(context.Background(), c, "p1", []string{"сложность::ВЫСОКАЯ", "нет такого"})
	if err != nil {
		t.Fatalf("matchProjectTags: %v", err)
	}
	got := map[string]bool{}
	for _, tg := range tags {
		got[tg.ID] = true
	}
	if !got["t1"] || !got["t2"] {
		t.Fatalf("detach must name both twins, got %+v", tags)
	}
	if len(tags) != 2 {
		t.Fatalf("an unknown name is skipped, not an error; got %+v", tags)
	}
}
