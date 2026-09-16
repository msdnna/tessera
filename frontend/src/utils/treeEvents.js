// Routing for realtime events that change the sidebar's project tree.
//
// The tree (groups + projects) is workspace state, so the layout owns it — the
// same place that already applies somebody else's board CRUD to it
// (stores/workspaces.js onBoardEvent, #2932). Its project/group half had no such
// handler: `refresh()` was reached only from onProjectGone (a delete) and from
// onProjectSlugChanged (a rename that also changed the address). Everything else
// a colleague did to the tree stayed invisible until a reload or a socket
// reconnect — a project or group they added never appeared, a rename stayed
// stale, a project dragged into another group kept its old parent, and a deleted
// group kept its projects nested under it (the server leaves those projects
// ungrouped — DeleteProjectGroup).
//
// Pure function on purpose: the routing table is the part worth testing, and it
// must not need a mounted AppLayout to be checked (same rationale as
// utils/boardEvents.js).

// Events that rewrite the tree. Every one of them is workspace-scoped and
// carries nothing the client can merge cheaply (a group edit can re-parent a
// whole subtree), so the answer is one re-read of groups + projects — two
// requests that deliberately leave the expanded boards alone.
const TREE_EVENTS = new Set([
  'group.created',
  'group.updated',
  'group.deleted',
  'group.moved',
  'project.created',
  'project.updated',
  'project.deleted',
  'project.moved',
])

// Should this event make the layout re-read the sidebar tree?
//
// `project.estimation` is deliberately absent: it names a project but changes
// only its estimation config, which the store patches in place
// (onEstimationEvent). Re-reading the tree for it would cost two requests to
// arrive at the same rows.
export function treeEventNeedsRefresh(ev, currentWorkspaceId) {
  if (!currentWorkspaceId || ev?.scope !== currentWorkspaceId) return false
  return TREE_EVENTS.has(ev?.type)
}
