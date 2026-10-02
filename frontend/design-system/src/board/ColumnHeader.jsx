/**
 * ColumnHeader — the kanban column title row: status glyph tinted with the
 * column colour (todo / in progress / review / done), name, card count
 * (`5 (12)` with subtasks), optional Σ estimate and the column menu.
 * @category Board
 */
import ColumnHeaderVue from '@/components/ColumnHeader.vue'
import { vueIsland } from '../runtime/island'

export const ColumnHeader = vueIsland(ColumnHeaderVue, {
  name: 'ColumnHeader',
  events: { onSetDone: 'set-done', onToggleCollapse: 'toggle-collapse', onChanged: 'changed' },
  toProps: (p) => ({
    dcol: { key: p.column?.id ?? p.column?.key, ...p.column },
    count: p.count ?? 0,
    total: p.total ?? 0,
    estimate: p.estimate ?? '',
    editable: !!p.editable,
    isDone: !!p.isDone,
    first: !!p.first,
    collapsed: !!p.collapsed,
  }),
})
