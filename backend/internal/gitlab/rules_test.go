package gitlab

import (
	"reflect"
	"testing"
)

func TestResolve_StatusMapsToColumn(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve([]string{"S: In review", "T: bug"})
	if got.ColumnName != "На рассмотрении" {
		t.Errorf("column = %q, want %q", got.ColumnName, "На рассмотрении")
	}
}

func TestResolve_PriorityMapsToField(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve([]string{"P: Critical"})
	if got.Priority != 4 {
		t.Errorf("priority = %d, want 4", got.Priority)
	}
}

func TestResolve_NoStatusFallsBackToDefaultColumn(t *testing.T) {
	r := DefaultRules()
	// Only a priority label — task should land in the default column.
	got := r.Resolve([]string{"P: Low"})
	if got.ColumnName != "К работе" {
		t.Errorf("column = %q, want default %q", got.ColumnName, "К работе")
	}
	if got.Priority != 1 {
		t.Errorf("priority = %d, want 1", got.Priority)
	}
}

func TestResolve_OtherLabelsBecomeTagsKeepingPrefix(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve([]string{"S: Done", "T: feature", "C: Backend", "Scope: Develop"})
	want := []string{"T: feature", "C: Backend", "Scope: Develop"}
	if !reflect.DeepEqual(got.TagNames, want) {
		t.Errorf("tags = %v, want %v", got.TagNames, want)
	}
	if got.ColumnName != "Готово" {
		t.Errorf("column = %q, want %q", got.ColumnName, "Готово")
	}
}

func TestResolve_FirstStatusWinsAndTagsDeduped(t *testing.T) {
	r := DefaultRules()
	got := r.Resolve([]string{"S: In progress", "S: Done", "C: QA", "C: QA"})
	if got.ColumnName != "В процессе" {
		t.Errorf("column = %q, want first-match %q", got.ColumnName, "В процессе")
	}
	if len(got.TagNames) != 1 || got.TagNames[0] != "C: QA" {
		t.Errorf("tags = %v, want one deduped 'C: QA'", got.TagNames)
	}
}

func TestResolve_TagModeIgnore(t *testing.T) {
	r := DefaultRules()
	r.TagMode = "ignore"
	got := r.Resolve([]string{"S: To do", "T: bug", "effort::small"})
	if len(got.TagNames) != 0 {
		t.Errorf("tags = %v, want none in ignore mode", got.TagNames)
	}
}

func TestResolve_StripNamespaceWhenNotKeepingPrefix(t *testing.T) {
	r := DefaultRules()
	r.TagKeepPrefix = false
	got := r.Resolve([]string{"T: bug", "effort::small"})
	want := []string{"bug", "small"}
	if !reflect.DeepEqual(got.TagNames, want) {
		t.Errorf("tags = %v, want %v", got.TagNames, want)
	}
}

func TestResolve_UnmappedStatusFallsBackToDefault(t *testing.T) {
	r := DefaultRules()
	// A status label not present in the map should not become a tag nor a column.
	got := r.Resolve([]string{"S: Unknown"})
	if got.ColumnName != "К работе" {
		t.Errorf("column = %q, want default", got.ColumnName)
	}
	if len(got.TagNames) != 0 {
		t.Errorf("unmapped status leaked into tags: %v", got.TagNames)
	}
}
