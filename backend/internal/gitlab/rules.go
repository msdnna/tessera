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
					if col, ok := rule.ValueMap[value]; ok {
						res.ColumnName, statusSet = col, true
					}
				}
			case "priority":
				if !prioSet {
					if lvl, ok := rule.ValueMap[value]; ok {
						if n, err := strconv.Atoi(lvl); err == nil {
							res.Priority, prioSet = int32(n), true
						}
					}
				}
			case "board":
				if !boardSet {
					if bid, ok := rule.ValueMap[value]; ok && bid != "" {
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
