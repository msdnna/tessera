package gitlab

import (
	"math"
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
//	action "milestone"→ the task's stage (#3086): ValueMap[value] when listed, else
//	                    Template with {value} substituted ("Спринт {value}"), else
//	                    the value itself. A native GitLab milestone wins over it.
//	action "ignore"   → the label is dropped
//
// "value" is the label title with the matched prefix trimmed (prefix match) or
// the full title (regex match).
type Rule struct {
	Match      string            `json:"match"`       // prefix string or regex pattern
	MatchType  string            `json:"match_type"`  // "prefix" (default) | "regex"
	Action     string            `json:"action"`      // status|priority|board|tag|group|milestone|ignore
	ValueMap   map[string]string `json:"value_map"`   // for status/priority/board/milestone
	KeepPrefix bool              `json:"keep_prefix"` // for tag action
	// Template names the stage of a milestone rule; "{value}" is the label's value.
	Template string `json:"template,omitempty"`
	// Disabled switches the rule off without deleting it (#3087): the engine passes
	// over it as if it were not there. Stored inverted so every rule saved before
	// the switch existed stays on.
	Disabled bool `json:"disabled,omitempty"`

	re *regexp.Regexp // compiled lazily for regex rules
}

// ValuePlaceholder is what a milestone rule's Template substitutes.
const ValuePlaceholder = "{value}"

// stageTitle renders the stage a milestone rule gives a matched value: a value-map
// entry wins, then the template, then the bare value.
func (r *Rule) stageTitle(value string) string {
	if v, ok := r.lookup(value); ok {
		return strings.TrimSpace(v)
	}
	if tpl := strings.TrimSpace(r.Template); tpl != "" {
		if strings.Contains(tpl, ValuePlaceholder) {
			return strings.TrimSpace(strings.ReplaceAll(tpl, ValuePlaceholder, value))
		}
		return tpl
	}
	return strings.TrimSpace(value)
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
	StatusSet  bool   // a status label matched (ColumnName is the issue's own, not the default)
	Priority   int32  // 0 when no priority matched
	Tags       []Tag  // labels to attach as tags (deduped, in order)
	BoardID    string // target board id when a board rule matched, else ""
	Group      bool   // a group rule matched (subtask grouping handled later)
	Milestone  string // stage title a milestone rule gave (#3086), "" when none
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

// parsePriority turns a value-map entry ("2") into a task priority. Rules are
// admin-authored JSONB, so the number is untrusted: anything outside int32 (the
// DB column) is rejected rather than silently wrapped into a bogus priority.
func parsePriority(s string) (int32, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
		return 0, false
	}
	return int32(n), true // #nosec G109 -- range-checked on the line above
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

// LabelOutcome is what one label of an issue turned into — the dry-run answer of
// «Проверить лейбл» (#3087) and the unit the per-rule counters count.
type LabelOutcome struct {
	Label string `json:"label"`
	// Rule is the index of the rule that decided the label, -1 when no rule
	// matched and the default action applied.
	Rule   int    `json:"rule"`
	Action string `json:"action"` // the deciding action; for Rule == -1 the default ("tag" | "ignore")
	// Value is the label's value as the rule saw it (prefix trimmed).
	Value string `json:"value,omitempty"`
	// Result is what the action made of it: a column name, a priority level, a
	// board id, a tag name, a stage title. "" when the action produced nothing
	// (e.g. a status value the map does not list).
	Result string `json:"result,omitempty"`
	// Shadowed is set when the rule fired but an earlier label already took the
	// same first-match-wins slot (status, priority, board, stage), so this label
	// changed nothing.
	Shadowed bool `json:"shadowed,omitempty"`
}

// Explain walks an issue's labels through the rules exactly as Resolve does and
// reports each label's fate. Resolve is built on it, so the dry-run cannot drift
// from what the sync really applies.
func (rs Rules) Explain(labels []Label) []LabelOutcome {
	var out []LabelOutcome
	rs.walk(labels, func(o LabelOutcome) { out = append(out, o) })
	return out
}

// Resolve maps an issue's labels to board state. Pure & deterministic in input
// order. Each label takes the first enabled rule that matches;
// status/priority/board/stage are first-match-wins across all labels.
func (rs Rules) Resolve(labels []Label) Resolution {
	return rs.walk(labels, nil)
}

// walk is Resolve with a per-label report (nil = none wanted).
func (rs Rules) walk(labels []Label, report func(LabelOutcome)) Resolution {
	res := Resolution{ColumnName: rs.DefaultColumn}
	prioSet, boardSet, stageSet := false, false, false
	seen := map[string]struct{}{}

	addTag := func(title, color string, keepPrefix bool) string {
		name := title
		if !keepPrefix {
			name = stripNamespace(title)
		}
		if name == "" {
			return ""
		}
		if _, dup := seen[name]; dup {
			return name
		}
		seen[name] = struct{}{}
		res.Tags = append(res.Tags, Tag{Name: name, Color: color})
		return name
	}

	for _, l := range labels {
		title := strings.TrimSpace(l.Title)
		if title == "" {
			continue
		}
		o := LabelOutcome{Label: title, Rule: -1}
		matched := false
		for i := range rs.Rules {
			rule := &rs.Rules[i]
			if rule.Disabled {
				continue
			}
			value, ok := rule.matches(title)
			if !ok {
				continue
			}
			matched = true
			o.Rule, o.Action, o.Value = i, rule.Action, value
			switch rule.Action {
			case "status":
				if col, ok := rule.lookup(value); ok {
					o.Result = col
					if !res.StatusSet {
						res.ColumnName, res.StatusSet = col, true
					} else {
						o.Shadowed = true
					}
				}
			case "priority":
				if lvl, ok := rule.lookup(value); ok {
					if n, ok := parsePriority(lvl); ok {
						o.Result = lvl
						if !prioSet {
							res.Priority, prioSet = n, true
						} else {
							o.Shadowed = true
						}
					}
				}
			case "board":
				if bid, ok := rule.lookup(value); ok && bid != "" {
					o.Result = bid
					if !boardSet {
						res.BoardID, boardSet = bid, true
					} else {
						o.Shadowed = true
					}
				}
			case "milestone":
				if stage := rule.stageTitle(value); stage != "" {
					o.Result = stage
					if !stageSet {
						res.Milestone, stageSet = stage, true
					} else {
						o.Shadowed = true
					}
				}
			case "group":
				res.Group = true
			case "tag":
				o.Result = addTag(title, l.Color, rule.KeepPrefix)
			case "ignore":
				// dropped
			}
			break // first matching rule wins for this label
		}
		if !matched {
			o.Action = "ignore"
			if rs.DefaultAction != "ignore" {
				o.Action = "tag"
				o.Result = addTag(title, l.Color, rs.TagKeepPrefix)
			}
		}
		if report != nil {
			report(o)
		}
	}
	return res
}

// RuleHits counts, for each rule, how many of the given label sets it decided at
// least one label of — the «Задач» counter by a rule (#3087), when each set is the
// labels of one linked issue. Disabled rules count zero: they decide nothing.
func (rs Rules) RuleHits(sets [][]Label) []int {
	hits := make([]int, len(rs.Rules))
	for _, labels := range sets {
		fired := map[int]bool{}
		for _, o := range rs.Explain(labels) {
			if o.Rule >= 0 {
				fired[o.Rule] = true
			}
		}
		for i := range fired {
			hits[i]++
		}
	}
	return hits
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
		if rule.Disabled {
			continue
		}
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
		if !rs.Rules[i].Disabled && rs.Rules[i].Action == "tag" && !rs.Rules[i].KeepPrefix {
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
