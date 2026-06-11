package gitlab

import "strings"

// Rules is the per-integration label rule engine config (stored as JSONB on
// gitlab_integrations.label_rules). It maps a GitLab issue's labels onto Tessera
// board state by label-namespace prefix:
//   - StatusPrefix labels  → a board column (kanban status; mutually exclusive)
//   - PriorityPrefix labels → the native task priority field
//   - everything else       → tags (optionally keeping the prefix)
//
// Column/priority targets are referenced by *name* / scalar (portable across
// boards), and resolved to ids by the caller against the integration's board.
type Rules struct {
	StatusPrefix   string            `json:"status_prefix"`
	StatusToColumn map[string]string `json:"status_to_column"` // GL status value (sans prefix) → column name
	DefaultColumn  string            `json:"default_column"`   // column for issues with no mapped status

	PriorityPrefix string           `json:"priority_prefix"`
	PriorityMap    map[string]int32 `json:"priority_map"` // GL priority value (sans prefix) → 0..4

	TagMode       string `json:"tag_mode"`        // "all" (default) | "ignore"
	TagKeepPrefix bool   `json:"tag_keep_prefix"` // keep the "T: " prefix on synced tag names
}

// Label is a GitLab label reduced to what the rule engine needs.
type Label struct {
	Title string
	Color string // hex, e.g. "#428BCA" (may be empty)
}

// Tag is a label resolved to a Tessera tag (name + the GitLab label colour,
// which may be empty — the caller picks a fallback colour then).
type Tag struct {
	Name  string
	Color string
}

// Resolution is the board state derived from an issue's labels.
type Resolution struct {
	ColumnName string // target column name (DefaultColumn if no status matched)
	Priority   int32  // 0 when no priority label matched
	Tags       []Tag  // labels to attach as tags (deduped, in input order)
}

// DefaultRules returns sensible defaults for the msdnna GitLab taxonomy
// (prefixes S:/P:/T:/C:/Scope:/B: …). Status collapses onto the 4 seeded board
// columns; non-status/priority labels become tags with their prefix kept, so
// they stay visually distinct from manually-applied scope tags.
func DefaultRules() Rules {
	return Rules{
		StatusPrefix: "S: ",
		StatusToColumn: map[string]string{
			"To do":        "К работе",
			"On hold":      "К работе",
			"In progress":  "В процессе",
			"Needs rework": "В процессе",
			"Needs tests":  "В процессе",
			"In review":    "На рассмотрении",
			"Tested":       "На рассмотрении",
			"Done":         "Готово",
		},
		DefaultColumn:  "К работе",
		PriorityPrefix: "P: ",
		PriorityMap: map[string]int32{
			"Critical":     4,
			"High":         3,
			"Medium":       2,
			"Low":          1,
			"Nice to have": 0,
		},
		TagMode:       "all",
		TagKeepPrefix: true,
	}
}

// Resolve maps an issue's labels to board state. Pure: no I/O, deterministic in
// input order. The first matched status label wins (statuses are meant to be
// mutually exclusive); likewise the first matched priority.
func (r Rules) Resolve(labels []Label) Resolution {
	res := Resolution{ColumnName: r.DefaultColumn}
	statusSet := false
	prioSet := false
	seen := map[string]struct{}{}

	for _, l := range labels {
		label := strings.TrimSpace(l.Title)
		if label == "" {
			continue
		}
		switch {
		case r.StatusPrefix != "" && strings.HasPrefix(label, r.StatusPrefix):
			if !statusSet {
				val := strings.TrimPrefix(label, r.StatusPrefix)
				if col, ok := r.StatusToColumn[val]; ok {
					res.ColumnName = col
					statusSet = true
				}
			}
		case r.PriorityPrefix != "" && strings.HasPrefix(label, r.PriorityPrefix):
			if !prioSet {
				val := strings.TrimPrefix(label, r.PriorityPrefix)
				if p, ok := r.PriorityMap[val]; ok {
					res.Priority = p
					prioSet = true
				}
			}
		default:
			if r.TagMode == "ignore" {
				continue
			}
			name := label
			if !r.TagKeepPrefix {
				name = stripNamespace(label)
			}
			if name == "" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			res.Tags = append(res.Tags, Tag{Name: name, Color: l.Color})
		}
	}
	return res
}

// stripNamespace removes a leading "prefix: " or "key::" namespace from a label
// title, e.g. "T: bug" → "bug", "effort::small" → "small".
func stripNamespace(label string) string {
	if i := strings.Index(label, "::"); i >= 0 {
		return strings.TrimSpace(label[i+2:])
	}
	if i := strings.Index(label, ": "); i >= 0 {
		return strings.TrimSpace(label[i+2:])
	}
	return label
}
