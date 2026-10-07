// The archive view (?archived=1) is advertised as read-only, and everything else
// on the board honours that: the column draggable, the card draggable and TaskCard
// all take `archivedMode`. The column HEADER did not — its `editable` prop only
// asked whether the lane is a status column — so the archive still offered rename,
// the colour palette and the delete button.
//
// Delete is the one that hurts. The server resolves the column's cards with
// `SELECT id FROM tasks WHERE column_id = $1` (TaskIDsInColumn) — no archived_at
// filter — then cascades and purges their uploads. The archive, meanwhile, shows
// and counts only the archived cards: a header reading «3» over a column holding
// 40 live ones, and a confirm that says «delete the column and its tasks». No undo.
//
// Pinned here: with `readonly`, the header opens neither menu nor rename, and the
// API is never reached. (Since #2969 the delete no longer takes the cards — they
// move to another column — but the archive stays a place to look, not to edit.)
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const remove = vi.fn()
const update = vi.fn()
vi.mock('@/api', () => ({
  columns: { remove: (...a) => remove(...a), update: (...a) => update(...a) },
}))

const { default: ColumnHeader } = await import('@/components/ColumnHeader.vue')

const DCOL = { key: 'c1', name: 'В процессе', rawName: 'В процессе' }
const mountHead = (readonly) =>
  mount(ColumnHeader, {
    props: { dcol: DCOL, editable: true, readonly, count: 3, total: 3 },
    global: { stubs: { TesseraIcon: true } },
  })

describe('ColumnHeader in the archive view', () => {
  beforeEach(() => {
    remove.mockClear()
    update.mockClear()
  })

  // The «…» trigger is what opens the settings popover — the palette, the
  // done-column toggle and the delete button all live in its (lazily rendered)
  // body, so the trigger's absence is what puts them out of reach.
  it('offers no «…» menu, so the delete button is out of reach', () => {
    const live = mountHead(false)
    expect(live.find('.col-menu').exists()).toBe(true)
    live.unmount()

    const archived = mountHead(true)
    expect(archived.find('.col-menu').exists()).toBe(false)
    archived.unmount()
  })

  it('ignores the right-click menu', async () => {
    const w = mountHead(true)
    await w.get('.col-head').trigger('contextmenu')

    expect(document.body.querySelector('[data-testid="column-actions"]')).toBeNull()
    expect(w.emitted('delete')).toBeUndefined()
    expect(remove).not.toHaveBeenCalled()
    w.unmount()
  })

  it('does not rename on double-click', async () => {
    const w = mountHead(true)
    await w.get('.col-title').trigger('dblclick')

    expect(w.find('input').exists()).toBe(false)
    expect(update).not.toHaveBeenCalled()
    w.unmount()
  })

  // The grab cursor and the Sortable handle class are a promise the archive can't
  // keep: KanbanBoard disables the column draggable there.
  it('drops the drag handle class', () => {
    expect(mountHead(false).get('.col-title').classes()).toContain('col-drag')
    expect(mountHead(true).get('.col-title').classes()).not.toContain('col-drag')
  })
})
