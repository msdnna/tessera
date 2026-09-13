package quickact

import (
	"testing"
	"time"
)

func TestCanonKeyAndLookup(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/close", "close"},
		{"  /Close  ", "close"},
		{"//close", "close"},
		{"DONE", "done"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := CanonKey(tc.in); got != tc.want {
			t.Errorf("CanonKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Aliases resolve to the canonical key.
	if cmd, ok := Lookup("/DONE"); !ok || cmd.Key != "close" {
		t.Errorf("Lookup(/DONE) = %v, %v; want close", cmd, ok)
	}
	if !IsBuiltin("blockedby") {
		t.Error("blockedby alias should be builtin")
	}
	if IsBuiltin("approve") {
		t.Error("approve is a custom key, not builtin")
	}
}

func TestRegistryIsWellFormed(t *testing.T) {
	seen := map[string]string{}
	for _, cmd := range Registry {
		for _, key := range append([]string{cmd.Key}, cmd.Aliases...) {
			if prev, dup := seen[key]; dup {
				t.Errorf("key %q declared by both %q and %q", key, prev, cmd.Key)
			}
			seen[key] = cmd.Key
			if CanonKey(key) != key {
				t.Errorf("key %q is not canonical", key)
			}
		}
		if cmd.Description == "" || cmd.Example == "" {
			t.Errorf("%q: description and example are shown in the popup, both required", cmd.Key)
		}
	}
}

func TestParse(t *testing.T) {
	custom := []string{"approve", "hold"}
	cases := []struct {
		name       string
		body       string
		wantCmds   []string // canonical keys, in order
		wantCustom []string
		wantRest   string
	}{
		{name: "plain text", body: "просто комментарий", wantRest: "просто комментарий"},
		{name: "single command", body: "/close", wantCmds: []string{"close"}},
		{
			name: "command then text", body: "/assign @msdnna\nпосмотри, пожалуйста",
			wantCmds: []string{"assign"}, wantRest: "посмотри, пожалуйста",
		},
		{
			name: "alias resolves", body: "/done",
			wantCmds: []string{"close"},
		},
		{
			name: "several commands keep order", body: "/priority высокий\n/due завтра\n/close",
			wantCmds: []string{"priority", "due", "close"},
		},
		{
			name: "custom command stays in text", body: "/approve\nвсё ок",
			wantCustom: []string{"approve"}, wantRest: "/approve\nвсё ок",
		},
		{
			name: "unknown command stays in text", body: "/frobnicate now",
			wantRest: "/frobnicate now",
		},
		{
			name: "slash mid-line is not a command", body: "cd /home && ls\nсмотри src/utils\nработаем 24/7",
			wantRest: "cd /home && ls\nсмотри src/utils\nработаем 24/7",
		},
		{
			name:     "command inside a fenced block is text",
			body:     "пример:\n```\n/close\n```\nвот так",
			wantRest: "пример:\n```\n/close\n```\nвот так",
		},
		{
			name:     "fence closes, command after it executes",
			body:     "```\n/close\n```\n/archive",
			wantCmds: []string{"archive"}, wantRest: "```\n/close\n```",
		},
		{
			name: "indented four spaces is a code block, not a command",
			body: "    /close", wantRest: "/close",
		},
		{
			name: "up to three spaces still parses", body: "   /close",
			wantCmds: []string{"close"},
		},
		{
			name: "mixed builtin and custom", body: "/close\n/hold\nпояснение",
			wantCmds: []string{"close"}, wantCustom: []string{"hold"},
			wantRest: "/hold\nпояснение",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Parse(tc.body, custom)
			if len(res.Cmds) != len(tc.wantCmds) {
				t.Fatalf("cmds = %v, want %v", keys(res.Cmds), tc.wantCmds)
			}
			for i, want := range tc.wantCmds {
				if res.Cmds[i].Key != want {
					t.Errorf("cmd[%d] = %q, want %q", i, res.Cmds[i].Key, want)
				}
			}
			if len(res.Custom) != len(tc.wantCustom) {
				t.Fatalf("custom = %v, want %v", keys(res.Custom), tc.wantCustom)
			}
			for i, want := range tc.wantCustom {
				if res.Custom[i].Key != want {
					t.Errorf("custom[%d] = %q, want %q", i, res.Custom[i].Key, want)
				}
			}
			if res.Rest != tc.wantRest {
				t.Errorf("rest = %q, want %q", res.Rest, tc.wantRest)
			}
		})
	}
}

func TestParseArgAndLine(t *testing.T) {
	res := Parse("текст\n/title  Новый  заголовок  ", nil)
	if len(res.Cmds) != 1 {
		t.Fatalf("cmds = %v, want one", keys(res.Cmds))
	}
	if got := res.Cmds[0].Arg; got != "Новый  заголовок" {
		t.Errorf("arg = %q, want inner spacing preserved and edges trimmed", got)
	}
	if res.Cmds[0].Line != 1 {
		t.Errorf("line = %d, want 1", res.Cmds[0].Line)
	}
}

func keys(cmds []Cmd) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Key
	}
	return out
}

func TestParseDate(t *testing.T) {
	now := time.Date(2026, 8, 4, 23, 45, 0, 0, time.UTC)
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{in: "2026-08-14", want: day(2026, 8, 14)},
		{in: "14.08.2026", want: day(2026, 8, 14)},
		{in: "14.08", want: day(2026, 8, 14)},
		{in: "сегодня", want: day(2026, 8, 4)},
		{in: "TOMORROW", want: day(2026, 8, 5)},
		{in: "вчера", want: day(2026, 8, 3)},
		{in: "+3d", want: day(2026, 8, 7)},
		{in: "+2w", want: day(2026, 8, 18)},
		{in: "+1m", want: day(2026, 9, 4)},
		{in: "-1д", want: day(2026, 8, 3)},
		{in: "", wantErr: true},
		{in: "когда-нибудь", wantErr: true},
		{in: "2026-13-01", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseDate(tc.in, now)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDate(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDate(%q): %v", tc.in, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("ParseDate(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParsePriority(t *testing.T) {
	cases := []struct {
		in      string
		want    int32
		wantErr bool
	}{
		{in: "нет", want: 0},
		{in: "низкий", want: 1},
		{in: "Обычный", want: 2},
		{in: " высокий ", want: 3},
		{in: "срочный", want: 4},
		{in: "urgent", want: 4},
		{in: "3", want: 3},
		{in: "9", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParsePriority(tc.in)
		if tc.wantErr != (err != nil) {
			t.Errorf("ParsePriority(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("ParsePriority(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseEstimate(t *testing.T) {
	timeCfg := Estimation{Unit: "time", HoursPerDay: 8, DaysPerWeek: 5}
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		// A bare number is HOURS, like the estimate field in both clients
		// (frontend utils/estimation.js, android util/Estimation.kt).
		{in: "90", want: 5400},
		{in: "90m", want: 90},
		{in: "2h", want: 120},
		{in: "1h30m", want: 90},
		{in: "1,5h", want: 90},
		{in: "2ч", want: 120},
		{in: "1d", want: 480},  // 8h day
		{in: "1w", want: 2400}, // 5d week
		{in: "1d 4h", want: 720},
		{in: "1d 4", want: 720}, // the tail-less number is hours too
		// A unit spelled out as a word, read by its first letter — the estimate
		// field on both clients matches `[a-zа-яё]*` there, and the UI language
		// here is Russian, so "3 дня" is the ordinary way to type it.
		{in: "3 дня", want: 1440},
		{in: "30 мин", want: 30},
		{in: "2 hours", want: 120},
		{in: "1 неделя", want: 2400},
		{in: "2 часа 30 минут", want: 150},
		{in: "1 день 4 часа", want: 720},
		{in: "0", wantErr: true},
		{in: "", wantErr: true},
		{in: "скоро", wantErr: true},
		{in: "2h кое-что", wantErr: true},
		{in: "2 3", wantErr: true},
		// A word that starts with no unit letter stays an error rather than
		// silently becoming hours the way the clients' field would read it: a
		// command that stored "5 попугаев" as five hours would be worse than one
		// that says it did not understand.
		{in: "5 попугаев", wantErr: true},
		{in: "2 часа кое-что", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseEstimate(tc.in, timeCfg)
		if tc.wantErr != (err != nil) {
			t.Errorf("ParseEstimate(%q) = %v, err = %v, wantErr %v", tc.in, got, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("ParseEstimate(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// Workspace settings drive day/week arithmetic.
	if got, err := ParseEstimate("1d", Estimation{Unit: "time", HoursPerDay: 6, DaysPerWeek: 4}); err != nil || got != 360 {
		t.Errorf("ParseEstimate(1d, 6h/day) = %v, %v; want 360", got, err)
	}
	// An absent unit inherits the built-in default: time, 8h/day, 5d/week.
	if got, err := ParseEstimate("1w", Estimation{}); err != nil || got != 2400 {
		t.Errorf("ParseEstimate(1w, zero cfg) = %v, %v; want 2400", got, err)
	}
}

// A points / custom workspace counts dimensionless numbers, so the argument is
// read the way the clients read their estimate field: a plain number (or a
// T-shirt size), and nothing that looks like a duration.
func TestParseEstimateNonTimeUnits(t *testing.T) {
	points := Estimation{Unit: "points", PointsScale: "fibonacci"}
	tshirt := Estimation{Unit: "points", PointsScale: "tshirt"}
	custom := Estimation{Unit: "custom"}

	cases := []struct {
		name    string
		cfg     Estimation
		in      string
		want    float64
		wantErr bool
	}{
		{name: "points number", cfg: points, in: "5", want: 5},
		{name: "points decimal comma", cfg: points, in: "2,5", want: 2.5},
		{name: "points reject duration", cfg: points, in: "2h", wantErr: true},
		{name: "points reject word", cfg: points, in: "много", wantErr: true},
		{name: "points reject zero", cfg: points, in: "0", wantErr: true},
		{name: "tshirt size", cfg: tshirt, in: "M", want: 3},
		{name: "tshirt size lowercase", cfg: tshirt, in: "xxl", want: 13},
		{name: "tshirt number still works", cfg: tshirt, in: "8", want: 8},
		{name: "tshirt unknown size", cfg: tshirt, in: "XXXL", wantErr: true},
		{name: "custom number", cfg: custom, in: "12", want: 12},
		{name: "custom reject duration", cfg: custom, in: "1d", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseEstimate(tc.in, tc.cfg)
		if tc.wantErr != (err != nil) {
			t.Errorf("%s: ParseEstimate(%q) = %v, err = %v, wantErr %v", tc.name, tc.in, got, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: ParseEstimate(%q) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestParseRefsMentionsTags(t *testing.T) {
	refs, err := ParseRefs("#2591 и #2605, ещё #2591")
	if err != nil {
		t.Fatalf("ParseRefs: %v", err)
	}
	if len(refs) != 2 || refs[0] != 2591 || refs[1] != 2605 {
		t.Errorf("ParseRefs = %v, want [2591 2605] deduped", refs)
	}
	if _, err := ParseRefs("нет номера"); err == nil {
		t.Error("ParseRefs without a number should fail")
	}

	if got := ParseMentions("@msdnna, @bot @msdnna"); len(got) != 2 || got[0] != "msdnna" || got[1] != "bot" {
		t.Errorf("ParseMentions = %v, want [msdnna bot]", got)
	}
	if got := ParseMentions("никого"); len(got) != 0 {
		t.Errorf("ParseMentions = %v, want empty", got)
	}

	if got := ParseTags("backend, очень срочно ,backend"); len(got) != 2 ||
		got[0] != "backend" || got[1] != "очень срочно" {
		t.Errorf("ParseTags = %v, want [backend, очень срочно]", got)
	}
}
