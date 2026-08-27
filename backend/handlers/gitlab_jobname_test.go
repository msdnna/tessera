package handlers

import (
	"strings"
	"testing"

	"tessera/internal/db"
)

// The live run's name must read the same as its journal entry (syncRunToDTO), or the
// same sync shows up under two different names while it runs and after it finishes.
// That now holds on two levels: the rendered fallback and the key+argument pair the
// client translates.
func TestGitlabSyncName(t *testing.T) {
	cases := []struct {
		title     string
		integ     db.GitlabIntegration
		want      string
		wantLabel string
	}{
		{"name set", db.GitlabIntegration{Name: "Pamir Scrum", ProjectPath: "pamir/scrum"}, "Синхронизация GitLab · Pamir Scrum", "Pamir Scrum"},
		{"name empty falls back to path", db.GitlabIntegration{ProjectPath: "pamir/scrum"}, "Синхронизация GitLab · pamir/scrum", "pamir/scrum"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			got := gitlabSyncName(c.integ)
			if got.Text != c.want {
				t.Errorf("gitlabSyncName().Text = %q, want %q", got.Text, c.want)
			}
			if got.Key != gitlabSyncNameKey || got.Arg != c.wantLabel {
				t.Errorf("gitlabSyncName() key/arg = %q/%q, want %q/%q", got.Key, got.Arg, gitlabSyncNameKey, c.wantLabel)
			}
			if !strings.HasPrefix(c.want, gitlabSyncJournalPrefix) {
				t.Errorf("name %q drifted from the journal prefix %q", c.want, gitlabSyncJournalPrefix)
			}
			// The journal side must produce the same key and argument, so a run doesn't
			// change its name in the panel the moment it lands in the journal.
			if got.Arg != gitlabSyncLabel(c.integ) {
				t.Errorf("live label %q differs from gitlabSyncLabel %q", got.Arg, gitlabSyncLabel(c.integ))
			}
		})
	}
}

func TestSyncOpLabel(t *testing.T) {
	cases := map[string]struct{ key, label string }{
		"full":        {"sync_full", "полная синхронизация"},
		"incremental": {"sync_incremental", "инкрементальная синхронизация"},
		"":            {"sync_incremental", "инкрементальная синхронизация"},
	}
	for in, want := range cases {
		if got := syncOpLabel(in); got != want.label {
			t.Errorf("syncOpLabel(%q) = %q, want %q", in, got, want.label)
		}
		if got := syncOpKey(in); got != want.key {
			t.Errorf("syncOpKey(%q) = %q, want %q", in, got, want.key)
		}
	}
}

// Every worker op key a tick loop can pass must have rendered wording behind it —
// otherwise the fallback silently becomes an empty string for clients (and the
// supervisor log) that don't know the key.
func TestWorkerOpsCoverEveryKey(t *testing.T) {
	for _, key := range []string{opSyncScan, opWriteback, opDelivery, opDueScan, opRecurrence} {
		if workerOps[key] == "" {
			t.Errorf("worker op %q has no rendered fallback", key)
		}
	}
}
