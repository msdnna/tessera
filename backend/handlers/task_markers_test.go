package handlers

import (
	"reflect"
	"testing"
)

// The table below is the mapping marked v18.0.4 implies — the copy the web
// preview and the Android WebView both render with — measured by flipping each
// candidate line and observing which rendered box moved. It is the same table
// MiscUtilTest pins the Kotlin walk with (android/.../util/MiscUtilTest.kt), and
// it is here for the same reason: the three answers to "which line draws this
// box" must not drift apart, since the recurrence empties a checklist by them.
var markedTruth = []struct {
	name string
	src  string
	want []int
}{
	{"simple", "- [ ] a\n- [x] b\n- [ ] c", []int{0, 1, 2}},
	{"fenced sample is text, not a box", "- [ ] real\n\n```\n- [ ] sample\n```\n\n- [ ] second", []int{0, 6}},
	{"tilde fence", "~~~\n- [ ] sample\n~~~\n- [x] real", []int{3}},
	{"fence keeps its info string", "```js\n- [ ] s\n```\n- [ ] r", []int{3}},
	{"unclosed fence swallows the rest", "- [ ] a\n```\n- [ ] b", []int{0}},
	{"fence indented inside a list", "- [ ] parent\n  ```\n  - [ ] sample\n  ```\n- [ ] second", []int{0, 4}},
	{"four-space block is code", "text\n\n    - [ ] sample\n\n- [ ] real", []int{4}},
	{"a leading tab is code too", "\t- [ ] tabbed", nil},
	{"four spaces under a list is a nested item", "- [ ] a\n\n    - [ ] four", []int{0, 2}},
	{"six spaces under a list is code", "- [ ] parent\n\n      code here\n\n- [ ] second", []int{0, 4}},
	{"quoted item is drawn as a box", "> - [ ] quoted\n\n- [ ] plain", []int{0, 2}},
	{"quoted list", "> - [ ] q1\n> - [x] q2", []int{0, 1}},
	{"no space after the bracket is literal text", "- [ ]x\n- [ ] real", []int{1}},
	{"an empty item is no item", "- [ ]", nil},
	{"nested item follows its parent", "- [ ] parent\n  - [ ] child\n- [x] sibling", []int{0, 1, 2}},
	{"deep indent is still an item", "- [ ] a\n    - [ ] deep", []int{0, 1}},
	{"numbered markers", "1. [ ] one\n2) [x] two", []int{0, 1}},
	{"star and plus markers", "* [ ] s\n+ [x] p", []int{0, 1}},
	{"upper-case tick", "- [X] up", []int{0}},
	// A literal `- [ ]x` still opens a *list*, so four spaces under it is a
	// nested item and not code — the indent that starts code is measured from the
	// open item's content column, not from the left margin.
	{"four spaces under a plain item", "- item\n\n    - [ ] under", []int{2}},
	{"code between literal brackets and real boxes", "- [ ]x\n\ntext\n\n    - [ ] indented\n\n- [ ] first\n- [x] second", []int{6, 7}},
}

func TestTaskMarkerLinesMatchesMarked(t *testing.T) {
	for _, tc := range markedTruth {
		if got := taskMarkerLines(tc.src); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: taskMarkerLines = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUntickDescriptionKeepsCodeSamples(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			// The reason this exists: a routine whose description documents its own
			// checklist. The sample is code on every client that draws it.
			name: "fence indented inside a list item",
			src:  "1. Прогнать чек-лист:\n\n    ```\n    - [x] образец\n    ```\n\n2. [x] Отправить отчёт",
			want: "1. Прогнать чек-лист:\n\n    ```\n    - [x] образец\n    ```\n\n2. [ ] Отправить отчёт",
		},
		{
			name: "indented code block",
			src:  "Шаблон:\n\n    - [x] образец\n\n- [x] реальный пункт",
			want: "Шаблон:\n\n    - [x] образец\n\n- [ ] реальный пункт",
		},
		{
			// A ~~~ line inside a ``` block is content, not a second fence: the
			// old scan read it as one and stepped back out of the block.
			name: "tilde line inside a backtick fence",
			src:  "```\n~~~\n- [x] образец\n```\n\n- [x] реальный",
			want: "```\n~~~\n- [x] образец\n```\n\n- [ ] реальный",
		},
		{
			name: "fence at the margin is still skipped",
			src:  "```\n- [x] образец\n```\n\n- [x] реальный",
			want: "```\n- [x] образец\n```\n\n- [ ] реальный",
		},
		{
			name: "quoted item is a box and is cleared",
			src:  "> - [x] цитата\n\n- [X] пункт",
			want: "> - [ ] цитата\n\n- [ ] пункт",
		},
		{
			name: "description with no boxes comes back byte for byte",
			src:  "просто текст [ссылка](http://x) и [скобки]",
			want: "просто текст [ссылка](http://x) и [скобки]",
		},
		{
			name: "already empty boxes are left alone",
			src:  "- [ ] раз\n- [ ] два",
			want: "- [ ] раз\n- [ ] два",
		},
	}
	for _, tc := range cases {
		if got := untickDescription(tc.src); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}
