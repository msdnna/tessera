// «Архив» is a scope of the board (?archived=1), and the control that opens it is
// the one that should close it. Neither of the two did.
//
//   * The desktop button (BoardActions) renders PRESSED inside the archive
//     (`type="primary"` + `secondary`) and then swallowed the click that would
//     un-press it: `@click="isArchive ? null : openArchive()"`.
//   * The phone menu (BoardMobileMenu) is the whole board toolbar there — Topbar
//     renders it *instead of* BoardActions — and its entry carried no state at
//     all: no tick, while every layout entry in the same dropdown gets one. From
//     inside the archive it re-pushed the identical query, which Vue Router
//     answers with a resolved NavigationFailure: a dead entry, silently.
//
// The only way back was the scope chip's «×» in the board composer — a different
// control, in a different place, from the one that got the reader in.
//
// Pinned here: the shared query maths (utils/boardScope), and that both
// affordances read AND clear the scope.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'

// One mutable route object so a spec can move the board in and out of the scope
// between mounts, and one router spy pair to read the navigation off.
const route = { query: {} }
const push = vi.fn()
const replace = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ push, replace }),
}))

const { default: BoardActions } = await import('@/components/BoardActions.vue')
const { default: BoardMobileMenu } = await import('@/components/BoardMobileMenu.vue')

const STUBS = { TagManager: true, MilestoneManager: true, TesseraIcon: true }

beforeEach(() => {
  setActivePinia(createPinia())
  route.query = {}
  push.mockClear()
  replace.mockClear()
})

describe('utils/boardScope', () => {
  it('entering keeps the board address and drops only the sprint scope', () => {
    // ?milestone= narrows the board to one sprint's live cards and the archive
    // lists archived ones — together they routinely resolve to an empty board.
    expect(archiveQuery({ milestone: 'sprint-7', tag: 'bug', layout: 'list' })).toEqual({
      archived: '1',
      tag: 'bug',
      layout: 'list',
    })
  })

  it('leaving drops only the scope key', () => {
    expect(withoutArchiveQuery({ archived: '1', tag: 'bug' })).toEqual({ tag: 'bug' })
  })

  it('does not mutate the query it was handed', () => {
    const q = { archived: '1', milestone: 'sprint-7' }
    archiveQuery(q)
    withoutArchiveQuery(q)
    expect(q).toEqual({ archived: '1', milestone: 'sprint-7' })
  })

  it('reads the flag as the string «1», not as anything truthy', () => {
    expect(isArchiveQuery({ archived: '1' })).toBe(true)
    expect(isArchiveQuery({ archived: '0' })).toBe(false)
    expect(isArchiveQuery({})).toBe(false)
    expect(isArchiveQuery(undefined)).toBe(false)
  })
})

describe('BoardActions «Архив» button', () => {
  const btn = (w) => w.find('[data-testid="board-archive"]')

  it('opens the archive from a live board', async () => {
    const w = mount(BoardActions, { global: { stubs: STUBS } })
    await btn(w).trigger('click')

    expect(push).toHaveBeenCalledWith({ query: { archived: '1' } })
    expect(replace).not.toHaveBeenCalled()
    w.unmount()
  })

  it('leaves the archive when pressed again — the pressed look is a toggle', async () => {
    route.query = { archived: '1', tag: 'bug' }
    const w = mount(BoardActions, { global: { stubs: STUBS } })
    await btn(w).trigger('click')

    // replace, not push: the archive must not become a step in the back history
    // of the board it overlays (same as the composer chip's «×»).
    expect(replace).toHaveBeenCalledWith({ query: { tag: 'bug' } })
    expect(push).not.toHaveBeenCalled()
    w.unmount()
  })
})

describe('BoardMobileMenu «Архив» entry', () => {
  const archiveOption = (w) => w.vm.options.find((o) => o.key === 'archive')

  it('is marked while the scope is on, plain while it is off', () => {
    const live = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    expect(live.vm.isActive(archiveOption(live))).toBe(false)
    expect(live.vm.items.find((i) => i.key === 'archive').active).toBe(false)
    live.unmount()

    route.query = { archived: '1' }
    const archived = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    expect(archived.vm.isActive(archiveOption(archived))).toBe(true)
    expect(archived.vm.items.find((i) => i.key === 'archive').active).toBe(true)
    archived.unmount()
  })

  it('opens the archive and drops the sprint scope', () => {
    route.query = { milestone: 'sprint-7' }
    const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    w.vm.onSelect('archive')

    expect(push).toHaveBeenCalledWith({ query: { archived: '1' } })
    w.unmount()
  })

  it('leaves the archive instead of re-pushing the same query', () => {
    route.query = { archived: '1', tag: 'bug' }
    const w = mount(BoardMobileMenu, { global: { stubs: STUBS } })
    w.vm.onSelect('archive')

    expect(replace).toHaveBeenCalledWith({ query: { tag: 'bug' } })
    expect(push).not.toHaveBeenCalled()
    w.unmount()
  })
})

// KanbanBoard is too heavy to mount (it loads a board over the API on create), so
// its two readers of the scope are read off the source — they must come from the
// same helper, or the chip and the buttons can drift apart again.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard reads the scope through the same helper', () => {
  it('uses isArchiveQuery for archivedMode and withoutArchiveQuery for the chip', () => {
    expect(kanban).toContain('const archivedMode = computed(() => isArchiveQuery(route.query))')
    expect(kanban).toContain('router.replace({ query: withoutArchiveQuery(route.query) })')
  })
})
