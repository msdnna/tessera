package gitlab

import (
	"sort"
	"strconv"
	"strings"
)

// Writeback is the per-integration write-back config (stored as JSONB on
// gitlab_integrations.writeback). The legacy boolean flags remain the "basic"
// opt-in surface (all default false); Bindings is the customizable replacement —
// when it is non-empty it takes over completely (see effectiveBindings). The
// legacy flags are still read to synthesize a default binding set that reproduces
// the pre-bindings behavior byte-for-byte, so existing integrations keep working
// with zero migration.
type Writeback struct {
	Enabled             bool              `json:"enabled"`
	PushState           bool              `json:"push_state"`
	PushPriority        bool              `json:"push_priority"`
	PushComments        bool              `json:"push_comments"`
	PushLabels          bool              `json:"push_labels"`     // reconcile tag-namespace labels
	PushDue             bool              `json:"push_due"`        // push the issue's own due_date
	PushAssignees       bool              `json:"push_assignees"`  // push the resolved assignee set
	PushEstimate        bool              `json:"push_estimate"`   // push timeEstimate (time unit only)
	PushMilestone       bool              `json:"push_milestone"`  // push the task's milestone (GitLab-linked only)
	PushTitleDesc       bool              `json:"push_title_desc"`
	PushCreate          bool              `json:"push_create"`     // allow creating a GitLab issue from a task
	FetchTemplates      bool              `json:"fetch_templates"` // offer repo issue templates when creating
	ColumnLabelBindings map[string]string `json:"column_label_bindings,omitempty"`

	// ── issue hierarchy (#2592) ──
	// Deliberately plain flags rather than rows in Bindings: a binding is "a FIELD of
	// an already-linked task changed → do something to its issue", and creating a
	// subtask fits neither half — the trigger is structural, and the child has no link
	// yet. Stretching the binding vocabulary for one case would cost more than it buys.

	// PushChildren allows a new subtask of a grouped, linked parent to be created as a
	// child work item in GitLab. Off by default, like every other push flag.
	PushChildren bool `json:"push_children"`
	// AutoGroupOnChild labels a linked parent as grouped by itself the first time a
	// subtask is pushed under it. Off by default on purpose: silently editing labels in
	// GitLab is not something to opt users into — the UI offers the button instead.
	AutoGroupOnChild bool `json:"auto_group_on_child"`
	// GroupLabel is the label that marks a grouped parent. Empty = DefaultGroupLabel.
	GroupLabel string `json:"group_label,omitempty"`

	// PushAttachments mirrors Tessera-hosted assets into the GitLab project's
	// upload store when a description/note is pushed (task #2713). A pointer, not a
	// plain bool, because this one defaults to ON: an absent key in an integration
	// configured before this field existed must mean "yes" (dead links in issues are
	// a bug, not a preference), and a plain bool would read that as "no". Turning it
	// off is for teams that don't want binaries duplicated into GitLab's storage —
	// links then degrade to a plain-text note instead of a broken image.
	PushAttachments *bool `json:"push_attachments,omitempty"`

	// Bindings is the customizable trigger→action table. Empty = fall back to the
	// legacy-flag defaults (effectiveBindings). Non-empty = admin took control.
	Bindings []Binding `json:"bindings,omitempty"`
}

// EffectiveGroupLabel is the label the "make this a grouped task" button writes.
func (w Writeback) EffectiveGroupLabel() string {
	if s := strings.TrimSpace(w.GroupLabel); s != "" {
		return s
	}
	return DefaultGroupLabel
}

// Trigger types — the Tessera-side event a binding reacts to. These are the
// vocabulary stored in the gitlab_writebacks.change_kind column.
const (
	TrigColumn     = "column"     // task moved to a column (any move)
	TrigCompletion = "completion" // the "completed" flag toggled
	TrigPriority   = "priority"
	TrigDue        = "due"
	TrigAssignees  = "assignees"
	TrigEstimate   = "estimate"
	TrigMilestone  = "milestone"
	TrigTitleDesc  = "title_desc"
	TrigLabels     = "labels" // a tag was added/removed
	TrigComment    = "comment"
)

// Structural child kinds (#2592). These share the gitlab_writebacks outbox with the
// triggers above but are deliberately NOT triggers: no binding resolves them, they
// describe a change to the task TREE rather than to a field, and for child_create the
// task has no link yet — so the worker branches on them before it looks one up.
const (
	KindChildCreate = "child_create" // subtask appeared under a grouped parent → open a child issue
	KindChildAttach = "child_attach" // an already-linked task became a subtask → re-parent it in GitLab
	KindChildDetach = "child_detach" // a linked subtask was detached → drop it to top-level in GitLab
)

// ChildIssueType is the GitLab issue_type given to a pushed subtask. An issue cannot
// hang under another issue in GitLab's hierarchy — only a work item of type Task can.
const ChildIssueType = "task"

// IsChildKind reports whether a change_kind is one of the structural child pushes,
// i.e. whether it must bypass the binding machinery entirely.
func IsChildKind(kind string) bool {
	return kind == KindChildCreate || kind == KindChildAttach || kind == KindChildDetach
}

// Action types — the GitLab-side effect of a matched binding.
const (
	ActSetLabel        = "set_label" // set one label, optionally clearing same-prefix siblings
	ActSetState        = "set_state" // close/reopen the issue
	ActSetDue          = "set_due"
	ActSetAssignees    = "set_assignees"
	ActSetEstimate     = "set_estimate"
	ActSetMilestone    = "set_milestone"
	ActSetTitleDesc    = "set_title_desc"
	ActReconcileLabels = "reconcile_labels" // diff task tags vs. issue tag-namespace labels
	ActPostComment     = "post_comment"
)

// Binding maps a Tessera-side trigger to a GitLab-side action. Bindings are the
// customizable replacement for the fixed write-back toggles: an admin declares,
// per integration, which GitLab action fires for which Tessera event.
type Binding struct {
	Enabled bool        `json:"enabled"`
	Trigger BindTrigger `json:"trigger"`
	Action  BindAction  `json:"action"`
}

// BindTrigger identifies a Tessera-side event plus optional qualifiers that
// narrow which occurrences match (e.g. only moves into a specific column, or only
// a specific priority level). Only the qualifier fields relevant to Type are read.
type BindTrigger struct {
	Type       string `json:"type"`
	ColumnID   string `json:"column_id,omitempty"`   // column: primary match key (stable across rename)
	ColumnName string `json:"column_name,omitempty"` // column: display + fallback match
	Priority   *int32 `json:"priority,omitempty"`    // priority: nil = any level
	Completed  *bool  `json:"completed,omitempty"`   // completion: nil = any
	DateKind   string `json:"date_kind,omitempty"`   // due: "due" | "start" (start dropped for issues)
}

// BindAction is the GitLab-side effect of a matched binding. ClearPrefix has no
// omitempty on purpose: the config re-marshals through this struct, so a deliberate
// false (keep sibling labels) must round-trip instead of defaulting back to true in
// the editor.
type BindAction struct {
	Type        string `json:"type"`
	Label       string `json:"label,omitempty"`      // set_label: full GL label title e.g. "S: In Progress"
	ClearPrefix bool   `json:"clear_prefix"`         // set_label: remove same-prefix sibling labels
	State       string `json:"state,omitempty"`      // set_state: "closed"|"opened"|"" (=derive from completed flag)
	DateKind    string `json:"date_kind,omitempty"`  // set_due: "due"; "start" dropped
	AddMarker   bool   `json:"add_marker,omitempty"` // post_comment: append the Tessera marker footer
}

// DefaultWriteback is the all-off config (write-back disabled).
func DefaultWriteback() Writeback { return Writeback{} }

// AttachmentsEnabled reports whether pushed bodies should mirror Tessera-hosted
// assets into GitLab. Absent means enabled — see PushAttachments.
func (w Writeback) AttachmentsEnabled() bool {
	return w.PushAttachments == nil || *w.PushAttachments
}

// effectiveBindings returns the bindings that actually drive write-back. When the
// admin has authored an explicit set, it wins verbatim. Otherwise the legacy
// boolean flags are synthesized into an equivalent binding set that reproduces the
// pre-bindings behavior exactly (so zero-migration back-compat holds). Returns nil
// when write-back is off and no bindings are set.
func (w Writeback) effectiveBindings(rules Rules) []Binding {
	if len(w.Bindings) > 0 {
		return w.Bindings
	}
	if !w.Enabled {
		return nil
	}
	var out []Binding
	add := func(b Binding) { b.Enabled = true; out = append(out, b) }

	if w.PushState {
		// state:"" — the worker derives open/closed from the live completed flag,
		// exactly as the old "state" case did.
		add(Binding{Trigger: BindTrigger{Type: TrigCompletion}, Action: BindAction{Type: ActSetState}})
	}
	if w.PushPriority {
		// The legacy priority push derived the "P:" label from the live priority via
		// the rule inverse. Reproduce it as one per-value set_label binding per level;
		// the same-prefix remove-set (+ AllPriorityLabels union at push time) swaps
		// stale labels just as before.
		if inv, ok := rules.InversePriority(); ok {
			levels := make([]int, 0, len(inv))
			for lvl := range inv {
				levels = append(levels, int(lvl))
			}
			sort.Ints(levels)
			for _, lvl := range levels {
				l := int32(lvl)
				add(Binding{
					Trigger: BindTrigger{Type: TrigPriority, Priority: &l},
					Action:  BindAction{Type: ActSetLabel, Label: inv[l], ClearPrefix: true},
				})
			}
		}
	}
	if w.PushComments {
		add(Binding{Trigger: BindTrigger{Type: TrigComment}, Action: BindAction{Type: ActPostComment}})
	}
	if w.PushLabels {
		add(Binding{Trigger: BindTrigger{Type: TrigLabels}, Action: BindAction{Type: ActReconcileLabels}})
	}
	if w.PushDue {
		add(Binding{Trigger: BindTrigger{Type: TrigDue, DateKind: "due"}, Action: BindAction{Type: ActSetDue, DateKind: "due"}})
	}
	if w.PushAssignees {
		add(Binding{Trigger: BindTrigger{Type: TrigAssignees}, Action: BindAction{Type: ActSetAssignees}})
	}
	if w.PushEstimate {
		add(Binding{Trigger: BindTrigger{Type: TrigEstimate}, Action: BindAction{Type: ActSetEstimate}})
	}
	if w.PushMilestone {
		add(Binding{Trigger: BindTrigger{Type: TrigMilestone}, Action: BindAction{Type: ActSetMilestone}})
	}
	if w.PushTitleDesc {
		add(Binding{Trigger: BindTrigger{Type: TrigTitleDesc}, Action: BindAction{Type: ActSetTitleDesc}})
	}
	// Legacy world has no column→label binding (column moves pushed only state),
	// so none is synthesized — column moves stay label-neutral by default.
	return out
}

// ResolveActions returns every action bound to a trigger occurrence t. The
// occurrence's Type + qualifier fields select matching, enabled bindings; multiple
// matches fan out. set_label actions sharing a namespace prefix are deduped
// keep-first, so two same-prefix bindings on one trigger don't churn labels.
func (w Writeback) ResolveActions(t BindTrigger, rules Rules) []BindAction {
	binds := w.effectiveBindings(rules)
	var out []BindAction
	seenPrefix := map[string]bool{}
	for _, b := range binds {
		if !b.Enabled || b.Trigger.Type != t.Type || !triggerMatches(b.Trigger, t) {
			continue
		}
		// Issues have no start date; a start binding is a silent no-op.
		if b.Trigger.Type == TrigDue && b.Trigger.DateKind == "start" {
			continue
		}
		if b.Action.Type == ActSetDue && b.Action.DateKind == "start" {
			continue
		}
		if b.Action.Type == ActSetLabel {
			if p := namespacePrefix(b.Action.Label); p != "" {
				if seenPrefix[p] {
					continue
				}
				seenPrefix[p] = true
			}
		}
		out = append(out, b.Action)
	}
	return out
}

// triggerMatches reports whether a binding's trigger (rule) matches an occurrence.
func triggerMatches(rule, occ BindTrigger) bool {
	switch rule.Type {
	case TrigColumn:
		if rule.ColumnID != "" && occ.ColumnID != "" {
			return rule.ColumnID == occ.ColumnID
		}
		return rule.ColumnName != "" && strings.EqualFold(rule.ColumnName, occ.ColumnName)
	case TrigPriority:
		return rule.Priority == nil || (occ.Priority != nil && *rule.Priority == *occ.Priority)
	case TrigCompletion:
		return rule.Completed == nil || (occ.Completed != nil && *rule.Completed == *occ.Completed)
	default:
		return true
	}
}

// SiblingLabels returns the same-namespace-prefix label titles that should be
// removed when setting `label`, so status labels are mutually exclusive. The set is
// drawn from the integration's OTHER set_label bindings sharing the prefix
// (deterministic and offline, mirroring the priority swap). When the prefix is the
// priority rule's namespace, it also unions rules.AllPriorityLabels() so a partial
// priority binding still clears stale "P:" labels. Returns nil when the label has no
// namespace prefix (nothing to mutually exclude).
func (w Writeback) SiblingLabels(label string, rules Rules) []string {
	prefix := namespacePrefix(label)
	if prefix == "" {
		return nil
	}
	set := map[string]bool{}
	for _, b := range w.effectiveBindings(rules) {
		if !b.Enabled || b.Action.Type != ActSetLabel {
			continue
		}
		l := strings.TrimSpace(b.Action.Label)
		if l == "" || l == label || namespacePrefix(l) != prefix {
			continue
		}
		set[l] = true
	}
	if pr := rules.priorityRule(); pr != nil && namespacePrefix(pr.Match) == prefix {
		for _, l := range rules.AllPriorityLabels() {
			if l != label {
				set[l] = true
			}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// namespacePrefix returns a label's namespace prefix — everything up to and
// including the first ":" (e.g. "S: In Progress" → "S:"), or "" if none. Two
// labels are siblings iff their prefixes are equal and non-empty.
func namespacePrefix(label string) string {
	label = strings.TrimSpace(label)
	if i := strings.Index(label, ":"); i >= 0 {
		return label[:i+1]
	}
	return ""
}

// Allows reports whether a change_kind may be pushed under this config. Always
// false when write-back is disabled.
func (w Writeback) Allows(kind string) bool {
	if !w.Enabled {
		return false
	}
	switch kind {
	case "state":
		return w.PushState
	case "priority":
		return w.PushPriority
	case "comment":
		return w.PushComments
	case "labels":
		return w.PushLabels
	case "due":
		return w.PushDue
	case "assignees":
		return w.PushAssignees
	case "estimate":
		return w.PushEstimate
	case "milestone":
		return w.PushMilestone
	case "title_desc":
		return w.PushTitleDesc
	default:
		return false
	}
}

// priorityRule returns the first "priority" action rule, or nil.
func (rs Rules) priorityRule() *Rule {
	for i := range rs.Rules {
		if rs.Rules[i].Action == "priority" {
			return &rs.Rules[i]
		}
	}
	return nil
}

// InversePriority builds the priority-level → full GitLab label title mapping
// from the priority rule (e.g. {4: "P: Critical", 3: "P: High", …}, reconstructing
// the title as prefix+value). The second return is false when there is no prefix
// priority rule or the inverse is ambiguous (two labels map to the same level —
// we couldn't know which to write back).
func (rs Rules) InversePriority() (map[int32]string, bool) {
	r := rs.priorityRule()
	if r == nil || r.MatchType == "regex" {
		return nil, false // can't reconstruct a label title from a regex match
	}
	out := make(map[int32]string, len(r.ValueMap))
	for value, lvl := range r.ValueMap {
		n, err := strconv.Atoi(lvl)
		if err != nil {
			continue
		}
		if _, dup := out[int32(n)]; dup {
			return nil, false // ambiguous inverse
		}
		out[int32(n)] = r.Match + value
	}
	return out, len(out) > 0
}

// AllPriorityLabels returns every full GitLab label title the priority rule knows
// (prefix+value), used as remove_labels when swapping the priority label so any
// stale priority label is cleared regardless of which one was set.
func (rs Rules) AllPriorityLabels() []string {
	r := rs.priorityRule()
	if r == nil || r.MatchType == "regex" {
		return nil
	}
	out := make([]string, 0, len(r.ValueMap))
	for value := range r.ValueMap {
		out = append(out, r.Match+value)
	}
	return out
}
