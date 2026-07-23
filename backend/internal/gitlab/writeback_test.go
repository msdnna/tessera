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
	w := Writeback{Enabled: true, PushState: true, PushComments: true, PushLabels: true, PushAssignees: true}
	cases := map[string]bool{
		"state": true, "priority": false, "comment": true, "labels": true,
		"due": false, "assignees": true, "estimate": false, "title_desc": false, "bogus": false,
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

func TestNamespacePrefix(t *testing.T) {
	cases := map[string]string{
		"S: In Progress": "S:",
		"P: Medium":      "P:",
		"effort::small":  "effort:",
		"plain":          "",
		"  S: X ":        "S:",
	}
	for in, want := range cases {
		if got := namespacePrefix(in); got != want {
			t.Errorf("namespacePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

// effectiveBindings must reproduce the legacy toggle behavior when no explicit
// bindings are set: each enabled flag → exactly one synthesized binding (priority
// fans out to one per level).
func TestEffectiveBindings_LegacySynthesis(t *testing.T) {
	rules := DefaultRules()
	w := Writeback{Enabled: true, PushState: true, PushComments: true, PushLabels: true,
		PushDue: true, PushAssignees: true, PushEstimate: true, PushMilestone: true, PushTitleDesc: true}
	got := w.effectiveBindings(rules)
	// 8 non-priority flags → 8 bindings; add priority to check the fan-out separately.
	if len(got) != 8 {
		t.Fatalf("expected 8 synthesized bindings, got %d", len(got))
	}
	seen := map[string]string{} // trigger.Type → action.Type
	for _, b := range got {
		if !b.Enabled {
			t.Errorf("synthesized binding %+v should be enabled", b)
		}
		seen[b.Trigger.Type] = b.Action.Type
	}
	want := map[string]string{
		TrigCompletion: ActSetState, TrigComment: ActPostComment, TrigLabels: ActReconcileLabels,
		TrigDue: ActSetDue, TrigAssignees: ActSetAssignees, TrigEstimate: ActSetEstimate,
		TrigMilestone: ActSetMilestone, TrigTitleDesc: ActSetTitleDesc,
	}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("synthesized trigger→action = %v, want %v", seen, want)
	}
}

func TestEffectiveBindings_PriorityFanout(t *testing.T) {
	w := Writeback{Enabled: true, PushPriority: true}
	got := w.effectiveBindings(DefaultRules())
	if len(got) != 5 {
		t.Fatalf("priority should fan out to 5 per-level bindings, got %d", len(got))
	}
	for _, b := range got {
		if b.Trigger.Type != TrigPriority || b.Trigger.Priority == nil {
			t.Errorf("expected per-level priority trigger, got %+v", b.Trigger)
		}
		if b.Action.Type != ActSetLabel || !b.Action.ClearPrefix {
			t.Errorf("expected set_label with clear_prefix, got %+v", b.Action)
		}
	}
	// Level 2 → "P: Medium" (scenario 1 shape, but synthesized from the rule inverse).
	acts := w.ResolveActions(BindTrigger{Type: TrigPriority, Priority: p32(2)}, DefaultRules())
	if len(acts) != 1 || acts[0].Label != "P: Medium" {
		t.Errorf("priority=2 → %v, want single set_label P: Medium", acts)
	}
}

func TestEffectiveBindings_Disabled(t *testing.T) {
	w := Writeback{Enabled: false, PushState: true}
	if got := w.effectiveBindings(DefaultRules()); got != nil {
		t.Errorf("disabled writeback should synthesize no bindings, got %v", got)
	}
}

func TestEffectiveBindings_ExplicitWins(t *testing.T) {
	explicit := []Binding{{Enabled: true,
		Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c1", ColumnName: "В процессе"},
		Action:  BindAction{Type: ActSetLabel, Label: "S: In Progress", ClearPrefix: true}}}
	w := Writeback{Enabled: true, PushState: true, Bindings: explicit}
	got := w.effectiveBindings(DefaultRules())
	if !reflect.DeepEqual(got, explicit) {
		t.Errorf("explicit bindings must win verbatim; got %v", got)
	}
}

// ResolveActions: the flagship column→label path, matched by id then name.
func TestResolveActions_Column(t *testing.T) {
	w := Writeback{Bindings: []Binding{{Enabled: true,
		Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c1", ColumnName: "В процессе"},
		Action:  BindAction{Type: ActSetLabel, Label: "S: In Progress", ClearPrefix: true}}}}

	// id match
	acts := w.ResolveActions(BindTrigger{Type: TrigColumn, ColumnID: "c1", ColumnName: "renamed"}, DefaultRules())
	if len(acts) != 1 || acts[0].Label != "S: In Progress" {
		t.Errorf("id match failed: %v", acts)
	}
	// name fallback (id absent on occurrence)
	acts = w.ResolveActions(BindTrigger{Type: TrigColumn, ColumnName: "в процессе"}, DefaultRules())
	if len(acts) != 1 {
		t.Errorf("case-insensitive name fallback failed: %v", acts)
	}
	// no match
	acts = w.ResolveActions(BindTrigger{Type: TrigColumn, ColumnID: "c2", ColumnName: "Готово"}, DefaultRules())
	if len(acts) != 0 {
		t.Errorf("non-matching column should resolve to nothing: %v", acts)
	}
}

func TestResolveActions_StartDropped(t *testing.T) {
	w := Writeback{Bindings: []Binding{{Enabled: true,
		Trigger: BindTrigger{Type: TrigDue, DateKind: "start"},
		Action:  BindAction{Type: ActSetDue, DateKind: "start"}}}}
	if acts := w.ResolveActions(BindTrigger{Type: TrigDue, DateKind: "start"}, DefaultRules()); len(acts) != 0 {
		t.Errorf("start-date binding must be dropped for issues, got %v", acts)
	}
}

func TestResolveActions_SamePrefixDedup(t *testing.T) {
	w := Writeback{Bindings: []Binding{
		{Enabled: true, Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c1"},
			Action: BindAction{Type: ActSetLabel, Label: "S: In Progress"}},
		{Enabled: true, Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c1"},
			Action: BindAction{Type: ActSetLabel, Label: "S: Blocked"}},
	}}
	acts := w.ResolveActions(BindTrigger{Type: TrigColumn, ColumnID: "c1"}, DefaultRules())
	if len(acts) != 1 || acts[0].Label != "S: In Progress" {
		t.Errorf("same-prefix set_labels should dedup keep-first, got %v", acts)
	}
}

func TestResolveActions_DisabledBindingSkipped(t *testing.T) {
	w := Writeback{Bindings: []Binding{{Enabled: false,
		Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c1"},
		Action:  BindAction{Type: ActSetLabel, Label: "S: X"}}}}
	if acts := w.ResolveActions(BindTrigger{Type: TrigColumn, ColumnID: "c1"}, DefaultRules()); len(acts) != 0 {
		t.Errorf("disabled binding must not resolve, got %v", acts)
	}
}

func TestSiblingLabels_ColumnStatus(t *testing.T) {
	w := Writeback{Bindings: []Binding{
		{Enabled: true, Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c1"},
			Action: BindAction{Type: ActSetLabel, Label: "S: In Progress", ClearPrefix: true}},
		{Enabled: true, Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c2"},
			Action: BindAction{Type: ActSetLabel, Label: "S: Done", ClearPrefix: true}},
		{Enabled: true, Trigger: BindTrigger{Type: TrigColumn, ColumnID: "c3"},
			Action: BindAction{Type: ActSetLabel, Label: "T: bug"}}, // different prefix — not a sibling
	}}
	got := w.SiblingLabels("S: In Progress", Rules{})
	want := []string{"S: Done"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SiblingLabels = %v, want %v", got, want)
	}
	if got := w.SiblingLabels("plain", Rules{}); got != nil {
		t.Errorf("label without prefix has no siblings, got %v", got)
	}
}

// A partial priority set_label binding must still clear stale P: labels via the
// AllPriorityLabels union.
func TestSiblingLabels_PriorityUnion(t *testing.T) {
	w := Writeback{Bindings: []Binding{
		{Enabled: true, Trigger: BindTrigger{Type: TrigPriority, Priority: p32(2)},
			Action: BindAction{Type: ActSetLabel, Label: "P: Medium", ClearPrefix: true}},
	}}
	got := w.SiblingLabels("P: Medium", DefaultRules())
	sort.Strings(got)
	want := []string{"P: Critical", "P: High", "P: Low", "P: Nice to have"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("priority siblings = %v, want %v", got, want)
	}
}

func p32(v int32) *int32 { return &v }
