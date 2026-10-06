// What the shell re-reads after a gap in the realtime stream — a reconnect
// (sleep, backend restart, flapping link) or a server-sent `resync` marker.
//
// Everything listed here is state the socket is the *only* live source for: it
// is fetched once at mount and then kept current by events alone. So a gap does
// not merely delay it — it drops whatever happened inside the gap on the floor,
// and the shell stays at its pre-outage frame until a full page reload.
//
// The bell was the case that bit (#2932): the layout's resync rebuilt the
// sidebar tree and nothing else, so every notification delivered while the
// socket was down — mentions, assignments, reminders — never arrived. The
// unread count read as yesterday's, which is worse than a missing badge: it
// says "nothing new" with authority.
//
// Takes the stores as arguments so the ordering below can be tested without
// mounting AppLayout (same rationale as utils/treeEvents.js).
export async function resyncShell({ workspaces, notifications, conflicts }) {
  // The workspace set first, and awaited: the server cuts these sockets on
  // every membership change (Hub.DropUser), so a reconnect may be the only
  // notice that our membership moved. The conflicts feed below is scoped to
  // whichever workspace survives that, and asking for it against the old one
  // would refetch a workspace we may no longer read.
  await workspaces.resync()
  // Independent of each other — the feed is per-user, the conflicts are
  // per-workspace — so neither has to wait for the other's request.
  await Promise.all([notifications.load(), conflicts.load(workspaces.currentId)])
}
