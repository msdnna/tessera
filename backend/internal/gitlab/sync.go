package gitlab

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// HashStr is a content snapshot helper (sha256 hex) for a link's *_hash columns:
// the pull records them and the write-back loop-guard compares against them.
func HashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// LabelsKey joins label titles for a link's snapshot hash.
func LabelsKey(labels []Label) string {
	titles := make([]string, len(labels))
	for i, l := range labels {
		titles[i] = l.Title
	}
	return strings.Join(titles, "\n")
}

// MergeIssues concatenates two issue lists, deduped by global id (first wins).
func MergeIssues(a, b []Issue) []Issue {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]Issue, 0, len(a)+len(b))
	for _, set := range [][]Issue{a, b} {
		for _, is := range set {
			if seen[is.GlobalID] {
				continue
			}
			seen[is.GlobalID] = true
			out = append(out, is)
		}
	}
	return out
}

// SyncKey is the change-detection snapshot stored on a link at the last sync:
// the issue's GitLab updatedAt plus content hashes of its title and labels.
type SyncKey struct {
	UpdatedAt  *time.Time
	TitleHash  string
	LabelsHash string
}

// Unchanged reports whether an issue matches this stored snapshot, i.e. nothing
// worth re-syncing happened to it. The hash check guards the second-precision
// updatedAt: two edits in the same second share a timestamp, so a same-second
// retitle is still caught. An unknown updatedAt on either side counts as changed.
func (k SyncKey) Unchanged(is Issue) bool {
	return k.UpdatedAt != nil && is.UpdatedAt != nil && is.UpdatedAt.Equal(*k.UpdatedAt) &&
		k.TitleHash == HashStr(is.Title) && k.LabelsHash == HashStr(LabelsKey(is.Labels))
}

// NeedNotes picks, out of the light list of a full sweep (#3105), the issues whose
// discussions have to be fetched: the ones with no link yet (known has no entry)
// and the ones GitLab touched since the last sync (updatedAt differs from the
// stored one, or either side doesn't know it). skip leaves out issues the run will
// not keep anyway — e.g. a closed issue the closed-issue policy sends straight to
// the archive. Returns the iids, in input order.
//
// The updatedAt comparison is enough because GitLab touches the issue when a note
// is added or edited on it (a note save touches its issue) — the same property the
// incremental pull's `updatedAfter` filter already relies on to pick up comments.
func NeedNotes(issues []Issue, known map[string]SyncKey, skip func(Issue) bool) []string {
	var out []string
	for _, is := range issues {
		if is.NotesLoaded || (skip != nil && skip(is)) {
			continue
		}
		k, linked := known[is.GlobalID]
		if linked && k.UpdatedAt != nil && is.UpdatedAt != nil && is.UpdatedAt.Equal(*k.UpdatedAt) {
			continue
		}
		out = append(out, strconv.FormatInt(is.IID, 10))
	}
	return out
}

// WithNotes swaps the issues of a light list for their fully fetched copies
// (matched by global id), keeping the light list's order and membership: an issue
// the second phase did not return keeps its light version.
func WithNotes(light, full []Issue) []Issue {
	if len(full) == 0 {
		return light
	}
	byID := make(map[string]Issue, len(full))
	for _, is := range full {
		byID[is.GlobalID] = is
	}
	out := make([]Issue, len(light))
	for i, is := range light {
		if f, ok := byID[is.GlobalID]; ok {
			out[i] = f
		} else {
			out[i] = is
		}
	}
	return out
}

// DropUnchanged removes issues whose stored SyncKey says nothing changed since the
// last sync — the incremental overlap window re-delivers already-synced issues, and
// reconciling them again is wasted work. Issues absent from known (no link yet) are
// always kept. known is keyed by issue global id; an empty map keeps everything.
func DropUnchanged(issues []Issue, known map[string]SyncKey) []Issue {
	if len(known) == 0 {
		return issues
	}
	kept := issues[:0]
	for _, is := range issues {
		if k, linked := known[is.GlobalID]; linked && k.Unchanged(is) {
			continue
		}
		kept = append(kept, is)
	}
	return kept
}
