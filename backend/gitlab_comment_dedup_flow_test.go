// Comment de-duplication across the push/pull boundary (task #2865).
//
// A comment written in Tessera is pushed to GitLab as a discussion; the next pull
// reads it back and must recognise it as the same comment instead of importing a
// second, GitLab-sourced copy the user cannot edit. Two independent mechanisms do
// that — the note gid, and (while the async push has not tagged the gid yet) the
// comment body — and both were broken in a way the suite could not see.
package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// taskCommentRow is one task_comments row, read straight from the database: the
// duplicate the user sees is a row-level fact (a second row with a NULL author),
// and the API view flattens exactly the distinction under test.
type taskCommentRow struct {
	Body     string
	HasLocal bool // author_id IS NOT NULL — a comment the user can still edit
	GlNoteID string
}

func taskCommentRows(t *testing.T, taskID string) []taskCommentRow {
	t.Helper()
	rows, err := testPool.Query(context.Background(),
		`SELECT body, author_id IS NOT NULL, coalesce(gl_note_id, '')
		   FROM task_comments WHERE task_id = $1::uuid ORDER BY created_at`, taskID)
	if err != nil {
		t.Fatalf("query comments: %v", err)
	}
	defer rows.Close()
	var out []taskCommentRow
	for rows.Next() {
		var r taskCommentRow
		if err := rows.Scan(&r.Body, &r.HasLocal, &r.GlNoteID); err != nil {
			t.Fatalf("scan comment: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// addFakeNote plants a note on fake issue !1 the way GitLab would hold it after a
// push, and bumps updatedAt so an incremental pull picks the issue up.
func addFakeNote(t *testing.T, w *wbFake, n glNote) {
	t.Helper()
	is := w.findIssue(1)
	if is == nil {
		t.Fatal("fake issue !1 missing")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	is.Notes = append(is.Notes, n)
	is.UpdatedAt = time.Now().UTC()
}

// finishedPullRuns counts completed pull runs. The push runs a drain produces
// share the journal, so counting every run would be ambiguous.
func finishedPullRuns(t *testing.T, c *client, wsID string) int {
	t.Helper()
	done := 0
	for _, r := range c.get("/workspaces/" + wsID + "/gitlab/sync-runs").listBody(t) {
		if r["kind"] == "pull" && r["finished_at"] != nil {
			done++
		}
	}
	return done
}

// syncAndWait triggers a pull and blocks until one more pull run has finished.
// Counting from a baseline matters: the stand already performed pulls of its own,
// so waiting for a fixed total would return before this pull ran and assert on
// state the sync had not touched yet.
// The pull is a full one on purpose: an incremental run only fetches issues
// assigned to the token's user, and the stand's issue !1 has no assignee — an
// incremental sync would return nothing and the assertions below would pass
// without the pull ever having looked at the comment.
func syncAndWait(t *testing.T, c *client, wsID, integID string) {
	t.Helper()
	before := finishedPullRuns(t, c, wsID)
	if r := c.post("/workspaces/"+wsID+"/gitlab/integrations/"+integID+"/sync?mode=full", nil); r.Status != http.StatusAccepted {
		t.Fatalf("full sync: status %d\n%s", r.Status, r.Body)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if finishedPullRuns(t, c, wsID) > before {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("pull run never finished (baseline %d)", before)
}

// The gid path. The push stores "gid://gitlab/Note/<id>"; GraphQL hands the same
// note back as "gid://gitlab/DiscussionNote/<id>" because a note inside a
// discussion is a DiscussionNote. gl_note_id is unique and upserted with
// ON CONFLICT, so the two spellings used to miss each other and the pull inserted
// a GitLab-sourced copy — the duplicate in the task's screenshot.
func TestGitlabPulledCommentDoesNotDuplicatePushedOne(t *testing.T) {
	t.Parallel()
	st := newWritebackStand(t, "gl-dedupgid-user", "grp-dedupgid")
	c, s, w := st.c, st.s, st.w

	st.putBindings(t, []map[string]any{{
		"enabled": true,
		"trigger": map[string]any{"type": "comment"},
		"action":  map[string]any{"type": "post_comment"},
	}})

	const body = "Комментарий из Tessera"
	if r := c.post("/tasks/"+st.taskID+"/comments", map[string]any{"body": body}); r.Status != http.StatusCreated {
		t.Fatalf("comment: status %d\n%s", r.Status, r.Body)
	}
	drainOutboxUntil(t, func() bool { return writebackRows(t, st.taskID)["comment"].Status == "sent" })

	// The fake now holds the note the push created (it registers it on the issue),
	// so this pull reads back our own comment — the exact round trip that used to
	// produce a second, uneditable copy.
	before := taskCommentRows(t, st.taskID)
	if len(before) != 1 || before[0].GlNoteID == "" {
		t.Fatalf("after push: want one comment tagged with its note gid, got %+v", before)
	}

	// A note from somebody else, so the assertions can tell "de-duplicated" from
	// "the pull never read any notes" — without it a sync that fetched nothing at
	// all would look identical to a correct one.
	const foreign = "Ответ коллеги"
	addFakeNote(t, w, glNote{ID: w.nextNoteID(), Body: foreign, AuthorLogin: "colleague"})

	syncAndWait(t, c, s.WS, st.integID)

	rows := taskCommentRows(t, st.taskID)
	var mine, theirs []taskCommentRow
	for _, r := range rows {
		if r.HasLocal {
			mine = append(mine, r)
		} else {
			theirs = append(theirs, r)
		}
	}
	sawForeign := false
	for _, r := range theirs {
		if r.Body == foreign {
			sawForeign = true
		}
	}
	if !sawForeign {
		t.Fatalf("the pull did not import the colleague's note — it read nothing: %+v", rows)
	}
	if len(theirs) != 1 {
		t.Fatalf("the pull imported a GitLab-sourced copy of our own comment: %+v", rows)
	}
	if len(mine) != 1 {
		t.Fatalf("own comments = %d, want 1: %+v", len(mine), rows)
	}
	if mine[0].Body != body || mine[0].GlNoteID != before[0].GlNoteID {
		t.Fatalf("comment changed identity across the pull: %+v → %+v", before[0], mine[0])
	}
}

// The body path. When a pull races the async push the gid is not tagged yet, and
// the only handle left is the body — but our copy holds "/api/uploads/…" links
// while GitLab's holds the mirrored "/uploads/…" ones. The comparison used to run
// on the raw GitLab body, so a comment with an attachment could never be claimed
// and was imported as a duplicate. No comment binding here on purpose: nothing is
// pushed, so gl_note_id stays NULL and this is the only mechanism in play.
func TestGitlabPullClaimsPushedCommentWithAttachment(t *testing.T) {
	t.Parallel()
	st := newWritebackStand(t, "gl-dedupbody-user", "grp-dedupbody")
	c, s, w := st.c, st.s, st.w

	// The asset map a real push would have written when it mirrored the image.
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO gitlab_uploads (integration_id, source_key, gl_url, gl_markdown)
		 VALUES ($1::uuid, '/api/uploads/pic.png', '/uploads/a1b2c3/pic.png', '')`, st.integID); err != nil {
		t.Fatalf("insert upload map: %v", err)
	}

	const localBody = "Смотри ![pic](/api/uploads/pic.png)"
	const gitlabBody = "Смотри ![pic](/uploads/a1b2c3/pic.png)"
	if r := c.post("/tasks/"+st.taskID+"/comments", map[string]any{"body": localBody}); r.Status != http.StatusCreated {
		t.Fatalf("comment: status %d\n%s", r.Status, r.Body)
	}

	noteID := w.nextNoteID()
	addFakeNote(t, w, glNote{ID: noteID, Body: gitlabBody, AuthorLogin: w.username, Discussion: noteID})

	syncAndWait(t, c, s.WS, st.integID)

	rows := taskCommentRows(t, st.taskID)
	if len(rows) != 1 {
		t.Fatalf("comments = %d, want 1 (the note was imported instead of claimed): %+v", len(rows), rows)
	}
	if !rows[0].HasLocal || rows[0].Body != localBody {
		t.Fatalf("the claimed comment is not the user's own: %+v", rows[0])
	}
	if want := fmt.Sprintf("gid://gitlab/Note/%d", noteID); rows[0].GlNoteID != want {
		t.Fatalf("gl_note_id = %q, want the claim to tag %s", rows[0].GlNoteID, want)
	}
}
