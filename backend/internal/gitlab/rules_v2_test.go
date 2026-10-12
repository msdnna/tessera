package gitlab

import (
	"fmt"
	"testing"
)

// «лейбл → этап» (#3086): a value-map entry wins, then the template, then the bare
// value; the first label that names a stage keeps it.
func TestResolve_MilestoneRule(t *testing.T) {
	rs := Rules{Rules: []Rule{
		{Match: "sprint::", Action: "milestone", Template: "Спринт {value}", ValueMap: map[string]string{"next": "Бэклог"}},
		{Match: "release::", Action: "milestone"},
	}}
	cases := []struct {
		labels []string
		want   string
	}{
		{[]string{"sprint::42"}, "Спринт 42"},
		{[]string{"sprint::next"}, "Бэклог"},
		{[]string{"release::2.0"}, "2.0"},
		{[]string{"release::2.0", "sprint::42"}, "2.0"},
		{[]string{"bug"}, ""},
	}
	for _, c := range cases {
		if got := rs.Resolve(labels(c.labels...)).Milestone; got != c.want {
			t.Errorf("Resolve(%v).Milestone = %q, want %q", c.labels, got, c.want)
		}
	}
	// A template without the placeholder names one fixed stage.
	fixed := Rules{Rules: []Rule{{Match: "hot", Action: "milestone", Template: "Горящие"}}}
	if got := fixed.Resolve(labels("hotfix")).Milestone; got != "Горящие" {
		t.Errorf("fixed template = %q", got)
	}
}

// A switched-off rule (#3087) is passed over as if it were not there: the label
// falls through to the next rule or to the default action.
func TestResolve_DisabledRule(t *testing.T) {
	rs := Rules{DefaultAction: "tag", TagKeepPrefix: true, Rules: []Rule{
		{Match: "S: ", Action: "status", ValueMap: map[string]string{"Done": "Готово"}, Disabled: true},
		{Match: "P: ", Action: "priority", ValueMap: map[string]string{"High": "3"}},
	}}
	res := rs.Resolve(labels("S: Done", "P: High"))
	if res.StatusSet || res.Priority != 3 {
		t.Fatalf("res = %+v, want no status and priority 3", res)
	}
	if len(res.Tags) != 1 || res.Tags[0].Name != "S: Done" {
		t.Fatalf("tags = %+v, want the status label as a plain tag", res.Tags)
	}
	if !rs.TagLabelClass("S: Done") {
		t.Fatal("a label under a disabled rule is a tag label now")
	}
	if _, ok := rs.InversePriority(); !ok {
		t.Fatal("the enabled priority rule still inverts")
	}
	rs.Rules[1].Disabled = true
	if _, ok := rs.InversePriority(); ok {
		t.Fatal("a disabled priority rule must not drive the priority push")
	}
}

// Explain is the dry run of «Проверить лейбл»: the deciding rule, its action and
// result, and whether an earlier label already took the slot.
func TestExplain(t *testing.T) {
	rs := DefaultRules()
	outs := rs.Explain(labels("S: In progress", "S: Done", "T: bug", "M: group"))
	got := ""
	for _, o := range outs {
		got += fmt.Sprintf("%s=%d/%s/%s/%v;", o.Label, o.Rule, o.Action, o.Result, o.Shadowed)
	}
	want := "S: In progress=0/status/В процессе/false;S: Done=0/status/Готово/true;T: bug=-1/tag/T: bug/false;M: group=2/group//false;"
	if got != want {
		t.Fatalf("Explain =\n%s\nwant\n%s", got, want)
	}
}

// RuleHits counts each issue once per rule that decided any of its labels.
func TestRuleHits(t *testing.T) {
	rs := DefaultRules()
	hits := rs.RuleHits([][]Label{
		labels("S: To do", "S: Done", "P: High"),
		labels("S: Done"),
		labels("bug"),
	})
	if fmt.Sprint(hits) != "[2 1 0]" {
		t.Fatalf("hits = %v, want [2 1 0]", hits)
	}
}
