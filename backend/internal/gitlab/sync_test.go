package gitlab

import (
	"testing"
	"time"
)

func TestLabelsKey(t *testing.T) {
	if got := LabelsKey(nil); got != "" {
		t.Fatalf("empty labels: want %q, got %q", "", got)
	}
	if got := LabelsKey(labels("bug", "p::high")); got != "bug\np::high" {
		t.Fatalf("joined labels: got %q", got)
	}
	// Order is part of the key on purpose: a reordered label list from GitLab
	// hashes differently and re-syncs. Cheap, and never misses a real change.
	if LabelsKey(labels("a", "b")) == LabelsKey(labels("b", "a")) {
		t.Fatal("expected label order to change the key")
	}
}

func TestHashStrIsStableAndDistinct(t *testing.T) {
	// The stored *_hash columns are compared against a hash computed on a later
	// run, so the same input must hash the same across calls.
	first := HashStr("x")
	if again := HashStr("x"); again != first {
		t.Fatalf("expected a stable hash, got %q then %q", first, again)
	}
	if HashStr("x") == HashStr("y") {
		t.Fatal("expected different hashes for different inputs")
	}
	if len(HashStr("")) != 64 {
		t.Fatalf("expected sha256 hex, got %d chars", len(HashStr("")))
	}
}

func TestMergeIssuesDedupesByGlobalIDFirstWins(t *testing.T) {
	a := []Issue{{GlobalID: "gid/1", Title: "from a"}, {GlobalID: "gid/2"}}
	b := []Issue{{GlobalID: "gid/1", Title: "from b"}, {GlobalID: "gid/3"}}

	out := MergeIssues(a, b)

	if len(out) != 3 {
		t.Fatalf("want 3 issues, got %d", len(out))
	}
	if out[0].Title != "from a" {
		t.Fatalf("first list must win on a duplicate, got %q", out[0].Title)
	}
	for i, want := range []string{"gid/1", "gid/2", "gid/3"} {
		if out[i].GlobalID != want {
			t.Fatalf("position %d: want %s, got %s", i, want, out[i].GlobalID)
		}
	}
}

func TestMergeIssuesEmptyInputs(t *testing.T) {
	if got := MergeIssues(nil, nil); len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
	only := []Issue{{GlobalID: "gid/1"}}
	if got := MergeIssues(nil, only); len(got) != 1 || got[0].GlobalID != "gid/1" {
		t.Fatalf("want the single issue through, got %+v", got)
	}
}

// issueAt builds an issue with the snapshot fields DropUnchanged looks at.
func issueAt(gid, title string, at time.Time, labelTitles ...string) Issue {
	t := at
	return Issue{GlobalID: gid, Title: title, UpdatedAt: &t, Labels: labels(labelTitles...)}
}

// keyOf is the snapshot a link would carry after syncing is.
func keyOf(is Issue) SyncKey {
	return SyncKey{UpdatedAt: is.UpdatedAt, TitleHash: HashStr(is.Title), LabelsHash: HashStr(LabelsKey(is.Labels))}
}

func TestDropUnchanged(t *testing.T) {
	at := time.Date(2026, 8, 11, 1, 2, 3, 0, time.UTC)
	synced := issueAt("gid/1", "same", at, "bug")

	cases := []struct {
		name  string
		issue Issue
		known map[string]SyncKey
		keep  bool
	}{
		{
			name:  "linked and unchanged is dropped",
			issue: synced,
			known: map[string]SyncKey{"gid/1": keyOf(synced)},
		},
		{
			name:  "not linked yet is kept",
			issue: synced,
			known: map[string]SyncKey{"gid/other": keyOf(synced)},
			keep:  true,
		},
		{
			name:  "newer updatedAt is kept",
			issue: issueAt("gid/1", "same", at.Add(time.Second), "bug"),
			known: map[string]SyncKey{"gid/1": keyOf(synced)},
			keep:  true,
		},
		{
			// The whole point of the hash check: same-second edits share a timestamp.
			name:  "same second, retitled is kept",
			issue: issueAt("gid/1", "renamed", at, "bug"),
			known: map[string]SyncKey{"gid/1": keyOf(synced)},
			keep:  true,
		},
		{
			name:  "same second, labels changed is kept",
			issue: issueAt("gid/1", "same", at, "bug", "p::high"),
			known: map[string]SyncKey{"gid/1": keyOf(synced)},
			keep:  true,
		},
		{
			name:  "unknown updatedAt on the issue is kept",
			issue: Issue{GlobalID: "gid/1", Title: "same", Labels: labels("bug")},
			known: map[string]SyncKey{"gid/1": keyOf(synced)},
			keep:  true,
		},
		{
			name:  "unknown updatedAt on the stored key is kept",
			issue: synced,
			known: map[string]SyncKey{"gid/1": {TitleHash: HashStr("same"), LabelsHash: HashStr("bug")}},
			keep:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DropUnchanged([]Issue{tc.issue}, tc.known)
			if tc.keep && len(got) != 1 {
				t.Fatalf("want the issue kept, got %d issues", len(got))
			}
			if !tc.keep && len(got) != 0 {
				t.Fatalf("want the issue dropped, got %d issues", len(got))
			}
		})
	}
}

func TestDropUnchangedNoKnownKeysKeepsEverything(t *testing.T) {
	issues := []Issue{{GlobalID: "gid/1"}, {GlobalID: "gid/2"}}
	if got := DropUnchanged(issues, nil); len(got) != 2 {
		t.Fatalf("want 2 issues, got %d", len(got))
	}
}

func TestDropUnchangedKeepsOrderOfSurvivors(t *testing.T) {
	at := time.Date(2026, 8, 11, 1, 2, 3, 0, time.UTC)
	stale := issueAt("gid/2", "unchanged", at)
	issues := []Issue{
		issueAt("gid/1", "new", at),
		stale,
		issueAt("gid/3", "also new", at),
	}

	got := DropUnchanged(issues, map[string]SyncKey{"gid/2": keyOf(stale)})

	if len(got) != 2 || got[0].GlobalID != "gid/1" || got[1].GlobalID != "gid/3" {
		t.Fatalf("want gid/1 then gid/3, got %+v", got)
	}
}
