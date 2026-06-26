package gitlab

import (
	"reflect"
	"sort"
	"testing"
)

func TestWriteback_AllowsDisabled(t *testing.T) {
	w := Writeback{Enabled: false, PushState: true, PushPriority: true, PushComments: true}
	for _, kind := range []string{"state", "priority", "comment"} {
		if w.Allows(kind) {
			t.Errorf("disabled writeback should not allow %q", kind)
		}
	}
}

func TestWriteback_AllowsPerKind(t *testing.T) {
	w := Writeback{Enabled: true, PushState: true, PushComments: true, PushLabels: true}
	cases := map[string]bool{
		"state": true, "priority": false, "comment": true, "labels": true,
		"due": false, "title_desc": false, "bogus": false,
	}
	for kind, want := range cases {
		if got := w.Allows(kind); got != want {
			t.Errorf("Allows(%q) = %v, want %v", kind, got, want)
		}
	}
}

func TestTagLabelClass_Default(t *testing.T) {
	rs := DefaultRules()
	cases := map[string]bool{
		"S: Done":    false, // status
		"P: High":    false, // priority
		"M: epic":    false, // group
		"T: bug":     true,  // default → tag
		"Scope: api": true,  // default → tag
		"":           false,
	}
	for title, want := range cases {
		if got := rs.TagLabelClass(title); got != want {
			t.Errorf("TagLabelClass(%q) = %v, want %v", title, got, want)
		}
	}
}

func TestTagLabelClass_IgnoreDefault(t *testing.T) {
	rs := Rules{DefaultAction: "ignore", Rules: []Rule{
		{Match: "T: ", MatchType: "prefix", Action: "tag", KeepPrefix: true},
	}}
	if !rs.TagLabelClass("T: bug") {
		t.Error("explicit tag rule should classify as tag")
	}
	if rs.TagLabelClass("random") {
		t.Error("unmatched label with default ignore should NOT be a tag")
	}
}

func TestTagsInvertible(t *testing.T) {
	if !DefaultRules().TagsInvertible() {
		t.Error("default rules keep the prefix → invertible")
	}
	stripped := Rules{TagKeepPrefix: false, DefaultAction: "tag"}
	if stripped.TagsInvertible() {
		t.Error("stripped default tags are not invertible")
	}
	mixedRule := Rules{TagKeepPrefix: true, Rules: []Rule{
		{Match: "T: ", Action: "tag", KeepPrefix: false},
	}}
	if mixedRule.TagsInvertible() {
		t.Error("a tag rule that strips the prefix breaks invertibility")
	}
}

func TestInversePriority_Default(t *testing.T) {
	inv, ok := DefaultRules().InversePriority()
	if !ok {
		t.Fatal("default priority rule should be invertible")
	}
	want := map[int32]string{4: "P: Critical", 3: "P: High", 2: "P: Medium", 1: "P: Low", 0: "P: Nice to have"}
	if !reflect.DeepEqual(inv, want) {
		t.Errorf("inverse = %v, want %v", inv, want)
	}
}

func TestInversePriority_Ambiguous(t *testing.T) {
	rs := Rules{Rules: []Rule{{Match: "P: ", MatchType: "prefix", Action: "priority", ValueMap: map[string]string{
		"High": "3", "Urgent": "3", // two labels → same level
	}}}}
	if _, ok := rs.InversePriority(); ok {
		t.Error("ambiguous priority mapping should not be invertible")
	}
}

func TestInversePriority_NoRule(t *testing.T) {
	rs := Rules{Rules: []Rule{{Match: "S: ", Action: "status"}}}
	if _, ok := rs.InversePriority(); ok {
		t.Error("no priority rule → not invertible")
	}
}

func TestAllPriorityLabels(t *testing.T) {
	got := DefaultRules().AllPriorityLabels()
	sort.Strings(got)
	want := []string{"P: Critical", "P: High", "P: Low", "P: Medium", "P: Nice to have"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}
