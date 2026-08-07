package handlers

import (
	"strings"
	"testing"

	"tessera/internal/db"
)

// The live run's name must read the same as its journal entry (syncRunToDTO), or the
// same sync shows up under two different names while it runs and after it finishes.
func TestGitlabSyncName(t *testing.T) {
	cases := []struct {
		title string
		integ db.GitlabIntegration
		want  string
	}{
		{"name set", db.GitlabIntegration{Name: "Pamir Scrum", ProjectPath: "pamir/scrum"}, "Синхронизация GitLab · Pamir Scrum"},
		{"name empty falls back to path", db.GitlabIntegration{ProjectPath: "pamir/scrum"}, "Синхронизация GitLab · pamir/scrum"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := gitlabSyncName(c.integ); got != c.want {
				t.Errorf("gitlabSyncName() = %q, want %q", got, c.want)
			}
			if !strings.HasPrefix(c.want, gitlabSyncJournalPrefix) {
				t.Errorf("name %q drifted from the journal prefix %q", c.want, gitlabSyncJournalPrefix)
			}
		})
	}
}

func TestSyncOpLabel(t *testing.T) {
	cases := map[string]string{
		"full":        "полная синхронизация",
		"incremental": "инкрементальная синхронизация",
		"":            "инкрементальная синхронизация",
	}
	for in, want := range cases {
		if got := syncOpLabel(in); got != want {
			t.Errorf("syncOpLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
