package gitlab

import (
	"reflect"
	"testing"
)

// labels builds a []Label from plain titles (no colour) for the tests.
func labels(titles ...string) []Label {
	out := make([]Label, len(titles))
	for i, t := range titles {
		out[i] = Label{Title: t}
	}
	return out
}

// tagNames extracts the resolved tag names in order.
func tagNames(r Resolution) []string {
	out := make([]string, len(r.Tags))
	for i, t := range r.Tags {
		out[i] = t.Name
	}
	return out
}

func TestResolve_StatusMapsToColumn(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve(labels("S: In review", "T: bug"))
	if got.ColumnName != "На рассмотрении" {
		t.Errorf("column = %q, want %q", got.ColumnName, "На рассмотрении")
	}
}

func TestResolve_PriorityMapsToField(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve(labels("P: Critical"))
	if got.Priority != 4 {
		t.Errorf("priority = %d, want 4", got.Priority)
	}
}

func TestResolve_NoStatusFallsBackToDefaultColumn(t *testing.T) {
	r := DefaultRules()
	// Only a priority label — task should land in the default column.
	got := r.Resolve(labels("P: Low"))
	if got.ColumnName != "К работе" {
		t.Errorf("column = %q, want default %q", got.ColumnName, "К работе")
	}
	if got.Priority != 1 {
		t.Errorf("priority = %d, want 1", got.Priority)
	}
}

func TestResolve_OtherLabelsBecomeTagsKeepingPrefix(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve(labels("S: Done", "T: feature", "C: Backend", "Scope: Develop"))
	want := []string{"T: feature", "C: Backend", "Scope: Develop"}
	if !reflect.DeepEqual(tagNames(got), want) {
		t.Errorf("tags = %v, want %v", tagNames(got), want)
	}
	if got.ColumnName != "Готово" {
		t.Errorf("column = %q, want %q", got.ColumnName, "Готово")
	}
}

func TestResolve_CarriesLabelColour(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve([]Label{{Title: "T: bug", Color: "#FF0000"}})
	if len(got.Tags) != 1 || got.Tags[0].Color != "#FF0000" {
		t.Errorf("tag colour not carried: %+v", got.Tags)
	}
}

func TestResolve_FirstStatusWinsAndTagsDeduped(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve(labels("S: In progress", "S: Done", "C: QA", "C: QA"))
	if got.ColumnName != "В процессе" {
		t.Errorf("column = %q, want first-match %q", got.ColumnName, "В процессе")
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "C: QA" {
		t.Errorf("tags = %v, want one deduped 'C: QA'", tagNames(got))
	}
}

func TestResolve_TagModeIgnore(t *testing.T) {
	r := DefaultRules()
	r.TagMode = "ignore"
	got := r.Resolve(labels("S: To do", "T: bug", "effort::small"))
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none in ignore mode", tagNames(got))
	}
}

func TestResolve_StripNamespaceWhenNotKeepingPrefix(t *testing.T) {
	r := DefaultRules()
	r.TagKeepPrefix = false
	got := r.Resolve(labels("T: bug", "effort::small"))
	want := []string{"bug", "small"}
	if !reflect.DeepEqual(tagNames(got), want) {
		t.Errorf("tags = %v, want %v", tagNames(got), want)
	}
}

func TestResolve_UnmappedStatusFallsBackToDefault(t *testing.T) {
	r := DefaultRules()
	// A status label not present in the map should not become a tag nor a column.
	got := r.Resolve(labels("S: Unknown"))
	if got.ColumnName != "К работе" {
		t.Errorf("column = %q, want default", got.ColumnName)
	}
	if len(got.Tags) != 0 {
		t.Errorf("unmapped status leaked into tags: %v", tagNames(got))
	}
}
