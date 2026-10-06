/**
 * TaskCard — the kanban card, exactly as the board renders it: priority bar on the
 * left edge, two-line title, property pills (priority, due date, tags, estimate,
 * milestone, assignees), subtask list. Board-level data (tags, members, columns,
 * milestones) is passed alongside the task and resolved by id, like on the board.
 * @category Board
 */
import TaskCardVue from '@/components/TaskCard.vue'
import { vueIsland } from '../runtime/island'
import { BOARD_KEYS, seedBoard } from '../runtime/board'

export const TaskCard = vueIsland(TaskCardVue, {
  name: 'TaskCard',
  seed: seedBoard,
  events: { onOpen: 'open', onChanged: 'changed', onRestore: 'restore' },
  toProps: (p) => ({
    task: p.task,
    subtasks: p.subtasks || [],
    subtasksTotal: p.subtasksTotal || 0,
    subtasksExpanded: !!p.subtasksExpanded,
    nested: !!p.nested,
    readonly: !!p.readonly,
    selected: !!p.selected,
    parentColumnId: p.parentColumnId ?? null,
  }),
})
TaskCard.boardKeys = BOARD_KEYS
