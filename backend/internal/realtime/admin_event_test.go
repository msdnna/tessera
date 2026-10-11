package realtime

import "testing"

// An instance-level event (Event.Admin, the jobs window #3067) reaches the
// sockets of global admins and nobody else, whatever their workspaces.
func TestAdminEventsReachOnlyAdmins(t *testing.T) {
	member := &Client{scopes: map[string]struct{}{"ws": {}}}
	admin := (&Client{scopes: map[string]struct{}{}}).WithAdmin(true)
	ev := Event{Type: "job.updated", Admin: true}
	if member.shouldReceive(ev) {
		t.Fatal("a member received an admin-only event")
	}
	if !admin.shouldReceive(ev) {
		t.Fatal("an admin missed an admin-only event")
	}
	// The flag does not widen workspace delivery.
	if admin.shouldReceive(Event{Scope: "ws", Type: "task.created"}) {
		t.Fatal("admin flag leaked a foreign workspace's event")
	}
}
