package gitlab

import (
	"crypto/sha256"
	"encoding/hex"
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
