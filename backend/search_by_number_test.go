// A task number is how this product spells a task everywhere: `#2932` on the
// card, `?task=2932` in the URL, «#2932» typed into the board's own search box
// (utils/taskFilter.matchesQuery, TaskFilter.kt), workspace_id + number in the
// MCP tools. Workspace search was the one place it meant nothing — SearchTasks
// matched `title ILIKE` and `description ILIKE` and nothing else.
//
// That made the miss a quiet one rather than an empty one. Typing «2932» in a
// tracker returns every task whose *description* mentions #2932 — a plausible
// list, minus the one task the reader was addressing. And workspace search is
// the only way to reach a task on a board you are not looking at, so there was
// no second route to it.
package main

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// searchNumbers pulls the task numbers out of a search response, in the order
// the server returned them — the ordering is half of what is pinned here.
func searchNumbers(t *testing.T, res map[string]any) []int64 {
	t.Helper()
	rows, _ := res["tasks"].([]any)
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("tasks: unexpected row %#v", r)
		}
		n, _ := m["number"].(float64)
		out = append(out, int64(n))
	}
	return out
}

func searchTasksBy(t *testing.T, c *client, ws, q string) map[string]any {
	t.Helper()
	return c.expect(t, c.get("/workspaces/"+ws+"/search?q="+url.QueryEscape(q)), http.StatusOK)
}

// createdNumber reads the number the server assigned to a freshly created task.
func createdNumber(t *testing.T, task map[string]any) int64 {
	t.Helper()
	n, ok := task["number"].(float64)
	if !ok || n < 1 {
		t.Fatalf("task has no number: %#v", task["number"])
	}
	return int64(n)
}

func TestSearchFindsTaskByNumber(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	mk := s.WS[:8]

	// The wanted task's title carries no digits at all: the only thing that can
	// match a numeric query here is the number itself.
	want := mkTask(t, c, s.Board, s.col(t, 0), "Совсем другое название "+mk)
	num := createdNumber(t, want)

	t.Run("#N addresses the task", func(t *testing.T) {
		got := searchNumbers(t, searchTasksBy(t, c, s.WS, fmt.Sprintf("#%d", num)))
		if len(got) != 1 || got[0] != num {
			t.Fatalf("«#%d» did not find task #%d: %v", num, num, got)
		}
	})

	t.Run("a bare N addresses it too", func(t *testing.T) {
		got := searchNumbers(t, searchTasksBy(t, c, s.WS, fmt.Sprintf("%d", num)))
		if len(got) == 0 || got[0] != num {
			t.Fatalf("«%d» did not find task #%d: %v", num, num, got)
		}
	})
}

// The exact task has to come first. Every task that *mentions* #N in its
// description carries a higher number (it was written later), so by `number
// DESC` alone the addressed task sorts under its own mentions — and past the
// page cut it drops out of the answer entirely.
func TestSearchRanksTheAddressedTaskFirst(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	mk := s.WS[:8]

	want := mkTask(t, c, s.Board, s.col(t, 0), "Адресуемая задача "+mk)
	num := createdNumber(t, want)

	// Two later tasks quoting the number the way a comment or a plan does.
	for i := 0; i < 2; i++ {
		decoy := mkTask(t, c, s.Board, s.col(t, 0), fmt.Sprintf("Ссылается %d %s", i, mk))
		c.expect(t, c.patch("/tasks/"+decoy["id"].(string),
			map[string]any{"description": fmt.Sprintf("см. #%d %s", num, mk)}), http.StatusOK)
	}

	got := searchNumbers(t, searchTasksBy(t, c, s.WS, fmt.Sprintf("#%d", num)))
	if len(got) != 3 {
		t.Fatalf("expected the task and its two mentions, got %v", got)
	}
	if got[0] != num {
		t.Fatalf("task #%d did not lead its own mentions: %v", num, got)
	}
}

// A number is only an address when it is the whole query. `12` inside «отчёт 12»
// is a word in a sentence, and the text match already covers it — reading it as
// a number would pull an unrelated task to the top of every such search.
func TestSearchNumberInsideAPhraseIsText(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	mk := s.WS[:8]

	other := mkTask(t, c, s.Board, s.col(t, 0), "Ни при чём "+mk)
	num := createdNumber(t, other)
	phrase := fmt.Sprintf("Отчёт %d %s", num, mk)
	wanted := mkTask(t, c, s.Board, s.col(t, 0), phrase)

	got := searchNumbers(t, searchTasksBy(t, c, s.WS, phrase))
	if len(got) != 1 || got[0] != createdNumber(t, wanted) {
		t.Fatalf("«%s» should match on text alone, got %v", phrase, got)
	}
}

// The controls: what a numeric query must NOT reach.
func TestSearchByNumberStaysInScope(t *testing.T) {
	t.Parallel()

	t.Run("an archived task is not addressable", func(t *testing.T) {
		t.Parallel()
		c := signup(t)
		s := mkStack(t, c)
		mk := s.WS[:8]
		task := mkTask(t, c, s.Board, s.col(t, 0), "Уехала в архив "+mk)
		num := createdNumber(t, task)
		c.expect(t, c.patch("/tasks/"+task["id"].(string)+"/archive",
			map[string]any{}), http.StatusNoContent)

		if got := searchNumbers(t, searchTasksBy(t, c, s.WS, fmt.Sprintf("#%d", num))); len(got) != 0 {
			t.Fatalf("archived task came back for «#%d»: %v", num, got)
		}
	})

	t.Run("another workspace's number is not visible", func(t *testing.T) {
		t.Parallel()
		mine := signup(t)
		ms := mkStack(t, mine)
		theirs := signup(t)
		ts := mkStack(t, theirs)
		// Numbering restarts per workspace (migration 0005), so the same number
		// exists on both sides — exactly the collision a scope leak would show.
		strangerNum := createdNumber(t, mkTask(t, theirs, ts.Board, ts.col(t, 0), "Чужая задача"))

		if got := searchNumbers(t, searchTasksBy(t, mine, ms.WS, fmt.Sprintf("#%d", strangerNum))); len(got) != 0 {
			t.Fatalf("a stranger's #%d leaked into my search: %v", strangerNum, got)
		}
	})

	t.Run("a non-number query is still a text search", func(t *testing.T) {
		t.Parallel()
		c := signup(t)
		s := mkStack(t, c)
		mk := s.WS[:8]
		mkTask(t, c, s.Board, s.col(t, 0), "Решётка # в названии "+mk)

		// A bare `#`, a number glued to letters and a number too large to be one
		// all have to behave like the text they are — and none of them may turn
		// the search into an error.
		for _, q := range []string{"#", "#" + mk, "2932abc" + mk, "99999999999999999999"} {
			searchTasksBy(t, c, s.WS, q)
		}
		got := searchTitles(t, searchTasksBy(t, c, s.WS, "Решётка # в названии "+mk), "tasks")
		if len(got) != 1 {
			t.Fatalf("a `#` in the text stopped matching as text: %v", got)
		}
	})
}
