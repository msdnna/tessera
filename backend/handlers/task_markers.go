package handlers

import (
	"regexp"
	"strings"
)

// Which lines of a markdown document carry a checkbox the reader actually sees.
//
// Three places answer this question about the same text and have to agree: the
// web preview (`utils/markdown.js`, which asks marked itself), the Android
// preview (`util/Markdown.kt`, which walks the block rules because a tap must be
// answered synchronously without a Kotlin parser), and this file, which the
// recurrence uses to hand the next occurrence an empty checklist. A line scan
// that only knows about a fence drifts from the rendering in both directions —
// see taskMarkerLines — so the walk below is the port of the Kotlin one, pinned
// by the same table measured off marked (ut-task-markers_test.go here,
// MiscUtilTest there).

// taskMarkerRe matches a GFM task-list marker at line start: `- [ ]`, `* [x]`,
// `1. [X]`, … Mirrors the frontend's TASK_MARKER_RE (utils/markdown.js) and the
// Android one (util/Markdown.kt), blockquote prefix included — «> - [x] …» is
// drawn with a real box.
var taskMarkerRe = regexp.MustCompile(`^([ \t>]*(?:[-*+]|[0-9]{1,9}[.)])[ \t]+\[)([ xX])(\])`)

// listMarkerRe matches any list marker; a task item is one of these with a box
// behind it. Its groups give the columns the item's content starts at, which is
// what an indent inside a list is measured against.
var listMarkerRe = regexp.MustCompile(`^([ \t]*)([-*+]|[0-9]{1,9}[.)])([ \t]+)`)

// quoteRe matches one nesting level of blockquote prefix.
var quoteRe = regexp.MustCompile(`^ {0,3}>[ \t]?`)

// codeIndent is the column count that opens an indented code block, and the tab
// stop width indentWidth expands a tab to.
const codeIndent = 4

// taskMarkerLines lists the source lines that draw a checkbox, in document order.
//
// Counting every line that *looks* like a marker is not the same question, and
// the gap is not academic:
//
//   - «- [ ] …» inside a ``` fence or an indented code block is a code SAMPLE,
//     drawn as text with no box — and a description that documents a checklist is
//     exactly where such a sample lives;
//   - «> - [ ] …» in a blockquote IS drawn as a box, but does not start its line
//     with a bullet;
//   - «- [ ]x», with no space after the bracket, and an item with nothing after
//     the box, are literal text to GFM.
//
// The approximation left standing is the Kotlin one's: an indented code block is
// recognised relative to the innermost open list item, which is where
// CommonMark's container rules are subtlest.
func taskMarkerLines(src string) []int {
	var out []int
	scan := newBlockScan()
	for i, raw := range strings.Split(src, "\n") {
		if scan.step(stripQuotes(raw)) {
			out = append(out, i)
		}
	}
	return out
}

// blockScan walks a document line by line, tracking the block context a checkbox
// needs to survive: an open code fence, an indented code block, and the innermost
// open list item.
type blockScan struct {
	fence       string
	listContent int
	inCode      bool
	prevBlank   bool
	wasBlank    bool
}

func newBlockScan() *blockScan {
	return &blockScan{listContent: -1, prevBlank: true, wasBlank: true}
}

// step feeds one blockquote-stripped line; true when that line draws a checkbox.
func (s *blockScan) step(body string) bool {
	if s.consumedByCode(body) {
		return false
	}
	trimmed := strings.TrimLeft(body, " \t")
	if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
		s.fence = fenceMarker(trimmed)
		return false
	}
	indent := indentWidth(body)
	m := listMarkerRe.FindStringSubmatch(body)
	if m == nil {
		// A block of its own at a column the open item cannot contain ends the list.
		if s.wasBlank && indent < max(s.listContent, 0) {
			s.listContent = -1
		}
		return false
	}
	s.listContent = indent + len(m[2]) + len(m[3])
	return isTaskItem(body)
}

// consumedByCode reports a line that carries no box whatever it looks like:
// blank, inside a fence (either delimiter included), or indented code.
func (s *blockScan) consumedByCode(body string) bool {
	if s.fence != "" {
		if closesFence(strings.TrimLeft(body, " \t"), s.fence) {
			s.fence = ""
		}
		s.prevBlank = false
		return true
	}
	if strings.TrimSpace(body) == "" {
		s.prevBlank = true
		return true
	}
	// Inside a list an indent is measured from the open item's content column:
	// four spaces under «- » is a nested item, six of them are code.
	codeAt := max(s.listContent, 0) + codeIndent
	indent := indentWidth(body)
	s.wasBlank = s.prevBlank
	s.prevBlank = false
	if s.inCode && indent >= codeAt {
		return true
	}
	s.inCode = indent >= codeAt && s.wasBlank
	return s.inCode
}

// isTaskItem reports whether the line's list marker carries a box GFM actually
// renders: the bracket has to be followed by a space, and the item needs content
// of its own.
func isTaskItem(body string) bool {
	m := taskMarkerRe.FindString(body)
	if m == "" {
		return false
	}
	after := body[len(m):]
	if !strings.HasPrefix(after, " ") && !strings.HasPrefix(after, "\t") {
		return false
	}
	return strings.TrimSpace(after) != ""
}

// stripQuotes removes every level of blockquote prefix from a line.
func stripQuotes(line string) string {
	for {
		m := quoteRe.FindString(line)
		if m == "" {
			return line
		}
		line = line[len(m):]
	}
}

// indentWidth is the line's leading indentation in columns, a tab advancing to
// the next multiple of codeIndent.
func indentWidth(s string) int {
	n := 0
	for _, ch := range s {
		switch ch {
		case ' ':
			n++
		case '\t':
			n += codeIndent - n%codeIndent
		default:
			return n
		}
	}
	return n
}
