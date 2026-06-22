package gitlab

import (
	"reflect"
	"testing"
)

func labels(titles ...string) []Label {
	out := make([]Label, len(titles))
	for i, t := range titles {
		out[i] = Label{Title: t}
	}
	return out
}

func tagNames(r Resolution) []string {
	out := make([]string, len(r.Tags))
	for i, t := range r.Tags {
		out[i] = t.Name
	}
	return out
}

func TestResolve_StatusMapsToColumn(t *testing.T) {
	got := DefaultRules().Resolve(labels("S: In review", "T: bug"))
	if got.ColumnName != "На рассмотрении" {
		t.Errorf("column = %q, want %q", got.ColumnName, "На рассмотрении")
	}
}

func TestResolve_StatusCaseInsensitive(t *testing.T) {
	// GitLab labels are "S: In Progress"/"S: In Review" (capitalised) while the
	// value-map keys are "In progress"/"In review" — they must still map.
	if got := DefaultRules().Resolve(labels("S: In Progress")); got.ColumnName != "В процессе" {
		t.Errorf("In Progress → %q, want В процессе", got.ColumnName)
	}
	if got := DefaultRules().Resolve(labels("S: In Review")); got.ColumnName != "На рассмотрении" {
		t.Errorf("In Review → %q, want На рассмотрении", got.ColumnName)
	}
}

func TestResolve_PriorityMapsToField(t *testing.T) {
	got := DefaultRules().Resolve(labels("P: Critical"))
	if got.Priority != 4 {
		t.Errorf("priority = %d, want 4", got.Priority)
	}
}

func TestResolve_NoStatusFallsBackToDefaultColumn(t *testing.T) {
	got := DefaultRules().Resolve(labels("P: Low"))
	if got.ColumnName != "К работе" {
		t.Errorf("column = %q, want default", got.ColumnName)
	}
	if got.Priority != 1 {
		t.Errorf("priority = %d, want 1", got.Priority)
	}
}

func TestResolve_OtherLabelsBecomeTagsKeepingPrefix(t *testing.T) {
	got := DefaultRules().Resolve(labels("S: Done", "T: feature", "C: Backend", "Scope: Develop"))
	want := []string{"T: feature", "C: Backend", "Scope: Develop"}
	if !reflect.DeepEqual(tagNames(got), want) {
		t.Errorf("tags = %v, want %v", tagNames(got), want)
	}
	if got.ColumnName != "Готово" {
		t.Errorf("column = %q, want %q", got.ColumnName, "Готово")
	}
}

func TestResolve_CarriesLabelColour(t *testing.T) {
	got := DefaultRules().Resolve([]Label{{Title: "T: bug", Color: "#FF0000"}})
	if len(got.Tags) != 1 || got.Tags[0].Color != "#FF0000" {
		t.Errorf("tag colour not carried: %+v", got.Tags)
	}
}

func TestResolve_GroupRule(t *testing.T) {
	got := DefaultRules().Resolve(labels("M: Сгруппированная"))
	if !got.Group {
		t.Error("group rule did not set Group")
	}
	if len(got.Tags) != 0 {
		t.Errorf("group label leaked into tags: %v", tagNames(got))
	}
}

func TestResolve_BoardRuleRoutes(t *testing.T) {
	r := DefaultRules()
	r.Rules = append(r.Rules, Rule{
		Match: "B: ", MatchType: "prefix", Action: "board",
		ValueMap: map[string]string{"Future": "board-uuid-123"},
	})
	got := r.Resolve(labels("B: Future"))
	if got.BoardID != "board-uuid-123" {
		t.Errorf("board = %q, want routed", got.BoardID)
	}
}

func TestResolve_RegexRuleTags(t *testing.T) {
	r := Rules{
		DefaultAction: "ignore",
		Rules: []Rule{
			{Match: "^(T|C): ", MatchType: "regex", Action: "tag", KeepPrefix: true},
		},
	}
	got := r.Resolve(labels("T: bug", "C: QA", "Scope: Develop"))
	want := []string{"T: bug", "C: QA"} // Scope unmatched → ignored
	if !reflect.DeepEqual(tagNames(got), want) {
		t.Errorf("tags = %v, want %v", tagNames(got), want)
	}
}

func TestResolve_FirstStatusWinsAndTagsDeduped(t *testing.T) {
	got := DefaultRules().Resolve(labels("S: In progress", "S: Done", "C: QA", "C: QA"))
	if got.ColumnName != "В процессе" {
		t.Errorf("column = %q, want first-match", got.ColumnName)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "C: QA" {
		t.Errorf("tags = %v, want one deduped 'C: QA'", tagNames(got))
	}
}

func TestResolve_DefaultActionIgnore(t *testing.T) {
	r := DefaultRules()
	r.DefaultAction = "ignore"
	got := r.Resolve(labels("S: To do", "T: bug", "effort::small"))
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none with default ignore", tagNames(got))
	}
}

func TestResolve_DefaultTagStripPrefix(t *testing.T) {
	r := DefaultRules()
	r.TagKeepPrefix = false
	got := r.Resolve(labels("T: bug", "effort::small"))
	want := []string{"bug", "small"}
	if !reflect.DeepEqual(tagNames(got), want) {
		t.Errorf("tags = %v, want %v", tagNames(got), want)
	}
}

func TestResolve_UnmappedStatusFallsBackToDefault(t *testing.T) {
	got := DefaultRules().Resolve(labels("S: Unknown"))
	if got.ColumnName != "К работе" {
		t.Errorf("column = %q, want default", got.ColumnName)
	}
	if len(got.Tags) != 0 {
		t.Errorf("unmapped status leaked into tags: %v", tagNames(got))
	}
}
