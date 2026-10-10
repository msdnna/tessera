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
  // The archive (#3005): the project leaves the tree for «Архив проектов» and back.
  'project.archived',
  'project.restored',
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

// Sprint (milestone) nodes are the tree's other half: a project on `tree_mode`
// `milestones`/`both` draws one row per milestone, and ProjectRow fetches that
// list exactly twice — when the project expands and when the mode flips to
// include sprints. Nothing else ever re-reads it, because the only listener for
// `milestone.*` is the OPEN BOARD (utils/boardEvents.js keeps them for its own
// reloadMilestones branch) and the tree table above covers groups/projects only.
//
// So every sprint edit — anyone's, including the reader's own from the board or
// from MilestoneManager — left the sidebar lying until a collapse+expand: a new
// sprint never appeared, a rename stayed stale, a closed one kept hanging in the
// list, and a deleted one stayed as a dead node whose click opens the board
// scoped to `?milestone=<gone id>` — an empty board with no hint why (#2932).
const MILESTONE_EVENTS = new Set(['milestone.created', 'milestone.updated', 'milestone.deleted'])

// Which project's sprint list does this event invalidate?
//
//   undefined — not a sprint event of the current workspace, do nothing
//   <id>      — that project's list (created/updated carry the milestone row)
//   null      — unknown project, so every loaded list has to be re-read:
//               `milestone.deleted` carries a bare {id}, and so does the GitLab
//               sync's `milestone.updated` (handlers/gitlab_milestone.go).
export function milestoneTreeTarget(ev, currentWorkspaceId) {
  if (!currentWorkspaceId || ev?.scope !== currentWorkspaceId) return undefined
  if (!MILESTONE_EVENTS.has(ev?.type)) return undefined
  const pid = ev?.data?.project_id
  return typeof pid === 'string' && pid ? pid : null
}
