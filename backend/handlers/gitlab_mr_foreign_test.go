package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
)

func TestChangeRef(t *testing.T) {
	for _, c := range []struct {
		repo, own string
		want      string
	}{
		{"scrum/board", "scrum/board", "!418"},
		{"Scrum/Board", "scrum/board", "!418"},
		{"", "scrum/board", "!418"},
		{"grp/api", "scrum/board", "grp/api!418"},
	} {
		if got := changeRef(c.repo, c.own, 418); got != c.want {
			t.Errorf("changeRef(%q, %q) = %q, want %q", c.repo, c.own, got, c.want)
		}
	}
}

func TestNeedsMRRead(t *testing.T) {
	at := time.Date(2026, 10, 11, 7, 0, 0, 0, time.UTC)
	later := at.Add(time.Minute)
	merged := gitlab.MergeRequest{State: "merged", UpdatedAt: &at}
	done := db.TaskCodeLink{State: "merged", CiStatus: "success", RemoteUpdatedAt: &at}
	if needsMRRead(merged, done, true) {
		t.Error("a settled merged MR is read again")
	}
	if !needsMRRead(merged, db.TaskCodeLink{}, false) {
		t.Error("a new MR is not read for its pipeline")
	}
	if !needsMRRead(gitlab.MergeRequest{State: "opened", UpdatedAt: &at}, done, true) {
		t.Error("an open MR is not read (its pipeline moves without it)")
	}
	running := done
	running.CiStatus = "running"
	if !needsMRRead(merged, running, true) {
		t.Error("a running pipeline is not re-read")
	}
	if !needsMRRead(gitlab.MergeRequest{State: "closed", UpdatedAt: &later}, done, true) {
		t.Error("a changed MR is not re-read")
	}
}

func TestMRRefreshGate(t *testing.T) {
	var g mrRefreshGate
	id := uuid.New()
	now := time.Now()
	if !g.claim(id, now, time.Minute) {
		t.Fatal("first claim refused")
	}
	if g.claim(id, now, 0) {
		t.Fatal("a running refresh was claimed twice")
	}
	g.done(id, now, true)
	if g.claim(id, now.Add(30*time.Second), time.Minute) {
		t.Fatal("claimed inside the ttl")
	}
	if !g.claim(id, now.Add(30*time.Second), 0) {
		t.Fatal("ttl 0 (the pull) must only wait for a running refresh")
	}
	g.done(id, now, false) // never ran: the stamp stays
	if !g.claim(id, now.Add(2*time.Minute), time.Minute) {
		t.Fatal("not claimable after the ttl")
	}
}
