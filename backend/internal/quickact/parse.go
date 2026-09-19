package quickact

import (
	"regexp"
	"strings"
)

// Cmd is one command occurrence found in a comment body.
type Cmd struct {
	Key  string `json:"key"`           // canonical key (aliases resolved)
	Arg  string `json:"arg,omitempty"` // argument text, trimmed
	Line int    `json:"line"`          // 0-based line index in the source body
	Raw  string `json:"raw"`           // the source line, verbatim
}

// Result is what Parse extracted from a comment body.
type Result struct {
	// Cmds are built-in commands, in source order. Their lines are stripped
	// from Rest — the comment must not repeat what was executed.
	Cmds []Cmd
	// Custom are workspace-dictionary commands. They are never executed and
	// their lines stay in Rest: "/approve" is a note to a bot, not an action.
	Custom []Cmd
	// Rest is the comment body with built-in command lines removed.
	Rest string
}

// cmdLine matches a command line: at most three leading spaces (four would be
// an indented markdown code block), a slash, the key, then the argument.
var cmdLine = regexp.MustCompile(`^ {0,3}/([A-Za-z][A-Za-z0-9_-]*)[ \t]*(.*)$`)

// fenceLine matches a markdown code fence: the marker run, then whatever
// follows it on the line (the info string of an opening fence).
var fenceLine = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

// fence is the code block currently open, if any. The zero value means none.
//
// Tracking the marker rather than flipping a boolean on every fence line is what
// makes quoting markdown inside markdown safe: a block opened with ``` is closed
// by ``` and by nothing else, so the ~~~ and the ```go inside it are its content.
// A plain toggle read them as the closing fence, handed the rest of the block to
// the executor, and then deleted the command line it found there from the
// comment — the author's snippet came back mangled and the task moved.
type fence struct {
	char  byte // '`' or '~'
	width int  // how many of them opened this block
}

func (f fence) on() bool { return f.width > 0 }

// closedBy reports whether a fence line ends this block: the same character, at
// least as long, and nothing but whitespace after it. The last clause is why
// ```go inside a ``` block is content — a closing fence carries no info string
// (CommonMark §4.5), and a language tag is how one quotes a code sample.
func (f fence) closedBy(marker, rest string) bool {
	return marker[0] == f.char && len(marker) >= f.width && strings.TrimSpace(rest) == ""
}

// Parse splits a comment body into executable commands and the remaining text.
//
// A command must occupy a whole line and start it — "cd /home" and "src/utils"
// are ordinary text, and so is a "/close" inside a fenced code block (otherwise
// the examples in this repository's own documentation would start executing).
//
// custom holds the workspace's dictionary keys; they are reported separately so
// the preview can say "recognised, does nothing" instead of staying silent.
func Parse(body string, custom []string) Result {
	customSet := make(map[string]bool, len(custom))
	for _, k := range custom {
		if k = CanonKey(k); k != "" {
			customSet[k] = true
		}
	}

	lines := strings.Split(body, "\n")
	res := Result{}
	kept := make([]string, 0, len(lines))
	var open fence

	for i, line := range lines {
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			switch {
			case !open.on():
				open = fence{char: m[1][0], width: len(m[1])}
			case open.closedBy(m[1], m[2]):
				open = fence{}
			}
			kept = append(kept, line)
			continue
		}
		if open.on() {
			kept = append(kept, line)
			continue
		}
		m := cmdLine.FindStringSubmatch(line)
		if m == nil {
			kept = append(kept, line)
			continue
		}
		key := CanonKey(m[1])
		arg := strings.TrimSpace(m[2])
		if cmd, ok := Lookup(key); ok {
			res.Cmds = append(res.Cmds, Cmd{Key: cmd.Key, Arg: arg, Line: i, Raw: line})
			continue // executed → line does not survive into the comment
		}
		if customSet[key] {
			res.Custom = append(res.Custom, Cmd{Key: key, Arg: arg, Line: i, Raw: line})
		}
		kept = append(kept, line) // custom and unknown alike stay as text
	}

	res.Rest = strings.TrimSpace(strings.Join(kept, "\n"))
	return res
}
