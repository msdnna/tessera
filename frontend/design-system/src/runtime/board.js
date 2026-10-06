// Board context for the card components. In the app, KanbanBoard loads the board's
// reference data (tags, members, milestones, columns) and display settings into the
// boardView store, and every card reads it from there. An island gets the same data
// as React props and seeds its own (per-island) store instance with it.
import { defaultFieldVis, useBoardViewStore } from '@/stores/boardView'

const byKey = (list, key) =>
  Object.fromEntries((list || []).filter(Boolean).map((x) => [x[key], x]))

export function seedBoard(pinia, p) {
  const bv = useBoardViewStore(pinia)
  bv.projectId = 'design'
  bv.columns = p.columns || []
  bv.refill(bv.tagsMap, byKey(p.tags, 'id'))
  bv.refill(bv.membersMap, byKey(p.members, 'user_id'))
  bv.refill(bv.milestonesMap, byKey(p.milestones, 'id'))
  bv.refill(bv.prefixNames, p.prefixNames || {})
  bv.cardSize = p.cardSize || 'medium'
  bv.showEmpty = p.showEmpty ?? true
  bv.stackFields = !!p.stackFields
  bv.refill(bv.fieldVis, { ...defaultFieldVis(), ...(p.fields || {}) })
}

// React props that feed the store rather than the Vue component.
export const BOARD_KEYS = [
  'columns',
  'tags',
  'members',
  'milestones',
  'prefixNames',
  'cardSize',
  'showEmpty',
  'stackFields',
  'fields',
]
