package gitlab

import "strconv"

// Writeback is the per-integration write-back config (stored as JSONB on
// gitlab_integrations.writeback). All flags default false, so write-back is
// strictly opt-in. ColumnLabelBindings is a forward seam for an explicit
// column→"S:" label mapping; it is empty in the MVP (status write-back is
// state-only because the column→label inverse is otherwise lossy).
type Writeback struct {
	Enabled             bool              `json:"enabled"`
	PushState           bool              `json:"push_state"`
	PushPriority        bool              `json:"push_priority"`
	PushComments        bool              `json:"push_comments"`
	PushLabels          bool              `json:"push_labels"`     // reconcile tag-namespace labels
	PushDue             bool              `json:"push_due"`        // push the issue's own due_date
	PushAssignees       bool              `json:"push_assignees"`  // push the resolved assignee set
	PushEstimate        bool              `json:"push_estimate"`   // push timeEstimate (time unit only)
	PushTitleDesc       bool              `json:"push_title_desc"`
	ColumnLabelBindings map[string]string `json:"column_label_bindings,omitempty"`
}

// DefaultWriteback is the all-off config (write-back disabled).
func DefaultWriteback() Writeback { return Writeback{} }

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
