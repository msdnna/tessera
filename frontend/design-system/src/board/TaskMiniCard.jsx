/**
 * TaskMiniCard — the compact task preview shown in hover cards, timeline/calendar
 * popovers and link lists: number, title, priority, due date, tags and assignees.
 * @category Board
 */
import TaskMiniCardVue from '@/components/TaskMiniCard.vue'
import { vueIsland } from '../runtime/island'

const byKey = (list, key) =>
  Object.fromEntries((list || []).filter(Boolean).map((x) => [x[key], x]))

export const TaskMiniCard = vueIsland(TaskMiniCardVue, {
  name: 'TaskMiniCard',
  toProps: (p) => ({
    task: p.task,
    tagsMap: byKey(p.tags, 'id'),
    membersMap: byKey(p.members, 'user_id'),
    tagPrefixNames: p.prefixNames || {},
    column: p.column ?? null,
  }),
})
