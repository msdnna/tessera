package gitlab

import (
	"regexp"
	"strconv"
	"strings"
)

// Rules is the per-integration label rule engine (stored as JSONB on
// gitlab_integrations.label_rules). It's a generic, ordered list of rules: each
// matches GitLab labels by a prefix or regex and applies an action. This
// replaces the old hardcoded status/priority/tag split.
type Rules struct {
	Rules         []Rule `json:"rules"`
	DefaultColumn string `json:"default_column"`  // column for issues with no status match
	DefaultAction string `json:"default_action"`  // "tag" (default) | "ignore" for unmatched labels
	TagKeepPrefix bool   `json:"tag_keep_prefix"` // keep the label prefix on tag names (default action + tag rules without their own setting)
}

// Rule matches labels and maps them to one action.
//
//	action "status"   → ValueMap[value] is a Tessera column name
//	action "priority" → ValueMap[value] is a priority level "0".."4"
//	action "board"    → ValueMap[value] is a target Tessera board id (uuid string)
//	action "tag"      → the label becomes a tag (KeepPrefix decides the name)
//	action "group"    → marks the task as a grouped parent (subtasks handled later)
//	action "ignore"   → the label is dropped
//
// "value" is the label title with the matched prefix trimmed (prefix match) or
// the full title (regex match).
type Rule struct {
	Match      string            `json:"match"`       // prefix string or regex pattern
	MatchType  string            `json:"match_type"`  // "prefix" (default) | "regex"
	Action     string            `json:"action"`      // status|priority|board|tag|group|ignore
	ValueMap   map[string]string `json:"value_map"`   // for status/priority/board
	KeepPrefix bool              `json:"keep_prefix"` // for tag action

	re *regexp.Regexp // compiled lazily for regex rules
}

// Label is a GitLab label reduced to what the rule engine needs.
type Label struct {
	Title string
	Color string // hex, e.g. "#428BCA" (may be empty)
}

// Tag is a label resolved to a Tessera tag.
type Tag struct {
	Name  string
	Color string
}

// Resolution is the board state derived from an issue's labels.
type Resolution struct {
	ColumnName string // target column name (DefaultColumn if no status matched)
	Priority   int32  // 0 when no priority matched
	Tags       []Tag  // labels to attach as tags (deduped, in order)
	BoardID    string // target board id when a board rule matched, else ""
	Group      bool   // a group rule matched (subtask grouping handled later)
}

// DefaultGroupLabel is the label put on an issue to mark it a grouped parent when the
// integration configures none of its own. It is spelled to match the "M: " prefix rule
// of DefaultRules — see ResolvesToGroup for why that has to hold.
const DefaultGroupLabel = "M: Сгруппированная задача"

// ResolvesToGroup reports whether a label title marks a grouped parent under these
// rules. This is the guard on the "make this a grouped task" button (#2592): the label
// it writes must be one the PULL will read back as grouping. Setting a label that no
// group rule matches would look like it worked and then quietly import as an ordinary
// tag, leaving the parent ungrouped and its children scattered.
func (rs Rules) ResolvesToGroup(label string) bool {
	if strings.TrimSpace(label) == "" {
		return false
	}
	return rs.Resolve([]Label{{Title: label}}).Group
}

// DefaultRules encodes the msdnna GitLab taxonomy as generic rules: S:→status,
// P:→priority, M:→group; everything else becomes a prefixed tag (default action).
func DefaultRules() Rules {
	return Rules{
		DefaultColumn: "К работе",
		DefaultAction: "tag",
		TagKeepPrefix: true,
		Rules: []Rule{
			{Match: "S: ", MatchType: "prefix", Action: "status", ValueMap: map[string]string{
				"To do": "К работе", "On hold": "К работе",
				"In progress": "В процессе", "Needs rework": "В процессе", "Needs tests": "В процессе",
				"In review": "На рассмотрении", "Tested": "На рассмотрении",
				"Done": "Готово",
			}},
			{Match: "P: ", MatchType: "prefix", Action: "priority", ValueMap: map[string]string{
				"Critical": "4", "High": "3", "Medium": "2", "Low": "1", "Nice to have": "0",
			}},
			{Match: "M: ", MatchType: "prefix", Action: "group"},
		},
	}
}

// matches reports whether the rule matches a label title, returning the extracted
// value (title minus prefix, or full title for regex).
func (r *Rule) matches(title string) (string, bool) {
	if r.MatchType == "regex" {
		if r.re == nil {
			re, err := regexp.Compile(r.Match)
			if err != nil {
				return "", false
			}
			r.re = re
		}
		if r.re.MatchString(title) {
			return title, true
		}
		return "", false
	}
	// prefix (default)
	if r.Match == "" || strings.HasPrefix(title, r.Match) {
		return strings.TrimPrefix(title, r.Match), true
	}
	return "", false
}

// lookup resolves a matched value against the rule's ValueMap. The match is
// case-insensitive: GitLab label capitalisation ("S: In Progress") should still
// hit a value-map key written in another case ("In progress"). Exact match wins;
// otherwise the first case-insensitive key match.
func (r *Rule) lookup(value string) (string, bool) {
	if v, ok := r.ValueMap[value]; ok {
		return v, true
	}
	for k, v := range r.ValueMap {
		if strings.EqualFold(k, value) {
			return v, true
		}
	}
	return "", false
}

// Resolve maps an issue's labels to board state. Pure & deterministic in input
// order. Each label takes the first rule that matches; status/priority/board are
// first-match-wins across all labels.
func (rs Rules) Resolve(labels []Label) Resolution {
	res := Resolution{ColumnName: rs.DefaultColumn}
	statusSet, prioSet, boardSet := false, false, false
	seen := map[string]struct{}{}

	addTag := func(title, color string, keepPrefix bool) {
		name := title
		if !keepPrefix {
			name = stripNamespace(title)
		}
		if name == "" {
			return
		}
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		res.Tags = append(res.Tags, Tag{Name: name, Color: color})
	}

	for _, l := range labels {
		title := strings.TrimSpace(l.Title)
		if title == "" {
			continue
		}
		matched := false
		for i := range rs.Rules {
			rule := &rs.Rules[i]
			value, ok := rule.matches(title)
			if !ok {
				continue
			}
			matched = true
			switch rule.Action {
			case "status":
				if !statusSet {
					if col, ok := rule.lookup(value); ok {
						res.ColumnName, statusSet = col, true
					}
				}
			case "priority":
				if !prioSet {
					if lvl, ok := rule.lookup(value); ok {
						if n, err := strconv.Atoi(lvl); err == nil {
							res.Priority, prioSet = int32(n), true
						}
					}
				}
			case "board":
				if !boardSet {
					if bid, ok := rule.lookup(value); ok && bid != "" {
						res.BoardID, boardSet = bid, true
					}
				}
			case "group":
				res.Group = true
			case "tag":
				addTag(title, l.Color, rule.KeepPrefix)
			case "ignore":
				// dropped
			}
			break // first matching rule wins for this label
		}
		if !matched && rs.DefaultAction != "ignore" {
			addTag(title, l.Color, rs.TagKeepPrefix)
		}
	}
	return res
}

// TagLabelClass reports whether a GitLab label title would resolve to a Tessera
// *tag* (as opposed to status/priority/board/group/ignore). Used by label
// write-back to reconcile ONLY tag-namespace labels — status (S:) and priority
// (P:) labels are owned by the state/priority write-back paths and never touched.
func (rs Rules) TagLabelClass(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	for i := range rs.Rules {
		rule := &rs.Rules[i]
		if _, ok := rule.matches(title); !ok {
			continue
		}
		return rule.Action == "tag" // first matching rule wins
	}
	// no rule matched → default action decides
	return rs.DefaultAction != "ignore"
}

// TagsInvertible reports whether tag names round-trip to full GitLab label titles
// (i.e. the prefix is kept), so a Tessera tag name can be pushed verbatim as a
// label. False when any tag-producing path strips the namespace — then we can't
// reconstruct the label title and label write-back must be skipped.
func (rs Rules) TagsInvertible() bool {
	if !rs.TagKeepPrefix {
		return false
	}
	for i := range rs.Rules {
		if rs.Rules[i].Action == "tag" && !rs.Rules[i].KeepPrefix {
			return false
		}
	}
	return true
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
