import { useBoardViewStore } from '@/stores/boardView'

// Seed the board-view store with the context a TaskCard / TaskModal needs.
//
// Those components take the open board's context (columns, tags, members,
// milestones, GitLab flags, card display settings) from the store rather than
// from props, so a test that mounts them has to fill it — this keeps every such
// test from hand-rolling the same setup. Call it after setActivePinia().
//
// Keys map 1:1 onto the store; lists are turned into the keyed maps the store
// owns, so a test can pass the plain arrays it already has.
export function seedBoardStore({
  boardId = 'b1',
  wsId = 'w1',
  projectId = 'p1',
  board = null,
  columns = [],
  tags = [],
  members = [],
  gitlabMembers = [],
  milestones = [],
  prefixNames = {},
  metaTagPrefixes = null,
  fieldVis = null,
  cardSize = 'medium',
  stackFields = false,
  showEmpty = true,
  gitlabCanCreate = false,
  gitlabFetchTemplates = false,
  gitlabIntegrationId = null,
} = {}) {
  const s = useBoardViewStore()
  s.setContext(boardId, wsId, projectId)
  s.board = board
  s.columns = columns
  s.refill(s.tagsMap, Object.fromEntries(tags.map((t) => [t.id, t])))
  s.refill(s.membersMap, Object.fromEntries(members.map((m) => [m.user_id, m])))
  s.refill(s.gitlabMembersMap, Object.fromEntries(gitlabMembers.map((g) => [g.gl_user_id, g])))
  s.refill(s.milestonesMap, Object.fromEntries(milestones.map((m) => [m.id, m])))
  s.refill(s.prefixNames, prefixNames)
  if (metaTagPrefixes) s.metaTagPrefixes = new Set(metaTagPrefixes)
  if (fieldVis) Object.assign(s.fieldVis, fieldVis)
  s.cardSize = cardSize
  s.stackFields = stackFields
  s.showEmpty = showEmpty
  s.gitlabCanCreate = gitlabCanCreate
  s.gitlabFetchTemplates = gitlabFetchTemplates
  s.gitlabIntegrationId = gitlabIntegrationId
  return s
}
