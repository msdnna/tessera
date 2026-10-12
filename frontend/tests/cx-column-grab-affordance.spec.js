// Regression test for #2932 — the column caption promised a drag the board had
// disabled.
//
// KanbanBoard's column draggable takes `handle: '.col-drag'` and is `:disabled`
// unless the board is grouped by status outside the archive; ColumnHeader stamps
// `.col-drag` on the status glyph and the caption under exactly that condition
// (`canEdit = editable && !readonly`). The grab cursor, though, sat on
// `.col-stat`/`.col-title` unconditionally, with a `.col-head.ro` rule taking it
// back in the archive — one of the two halves of `canEdit`. The other half is tag
// and milestone grouping, where the lanes aren't columns at all: nothing there
// reorders, dblclick-to-rename returns early and the «…» menu isn't rendered, yet
// every caption still showed a grab hand over the widest target in the header.
//
// Pinned here: the grab cursor is declared on the handle class itself, never on an
// element that also renders without it. The cursor lives in a scoped <style>, which
// jsdom doesn't apply, so that half is read off the source (same trick as
// cx-collapsed-strip-tap.spec.js); the class half is asserted by mounting. Vitest
// runs with frontend/ as cwd.
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const { default: ColumnHeader } = await import('@/components/ColumnHeader.vue')

const src = readFileSync(resolve(process.cwd(), 'src/components/ColumnHeader.vue'), 'utf8')
const style = src.slice(src.indexOf('<style'))

// Selectors of every rule in the scoped stylesheet that declares `cursor: grab`.
function grabSelectors(css) {
  const out = []
  const re = /([^{}]+)\{([^{}]*)\}/g
  let m
  while ((m = re.exec(css))) {
    if (/cursor:\s*grab/.test(m[2])) out.push(m[1].trim())
  }
  return out
}

const DCOL = { key: 'c1', name: 'В процессе', rawName: 'В процессе' }
const mountHead = (props) =>
  mount(ColumnHeader, {
    props: { dcol: DCOL, count: 3, total: 3, ...props },
    global: { stubs: { TesseraIcon: true } },
  })

describe('column header grab affordance (#2932)', () => {
  it('declares the grab cursor only on the drag handle class', () => {
    const sels = grabSelectors(style)
    expect(sels.length).toBeGreaterThan(0)
    for (const s of sels) expect(s).toContain('.col-drag')
  })

  it('no longer hangs it on the caption or the glyph themselves', () => {
    // Both render in lanes that cannot be dragged, so a rule of their own is what
    // put the hand where nothing honours it.
    for (const s of grabSelectors(style)) {
      expect(s).not.toMatch(/(^|,)\s*\.col-title\s*(,|$)/)
      expect(s).not.toMatch(/(^|,)\s*\.col-stat\s*(,|$)/)
    }
  })

  it('needs no archive-only cursor override any more', () => {
    // `.col-head.ro …{cursor: default}` existed solely to undo the unconditional
    // grab. Left in place it would be dead weight hiding a future regression.
    expect(style).not.toMatch(/\.col-head\.ro[^{]*\{[^}]*cursor:\s*default/)
  })

  it('stamps the handle class on a real status column', () => {
    const w = mountHead({ editable: true, readonly: false })
    expect(w.find('.col-title').classes()).toContain('col-drag')
    expect(w.find('.col-stat').classes()).toContain('col-drag')
    w.unmount()
  })

  it('withholds it from a tag / milestone lane', () => {
    // groupMode !== 'status' → `editable: false`: the column draggable is
    // :disabled there, and the glyph isn't rendered at all.
    const w = mountHead({ editable: false, readonly: false })
    expect(w.find('.col-title').classes()).not.toContain('col-drag')
    expect(w.find('.col-stat').exists()).toBe(false)
    w.unmount()
  })

  it('withholds it in the archive', () => {
    const w = mountHead({ editable: true, readonly: true })
    expect(w.find('.col-title').classes()).not.toContain('col-drag')
    expect(w.find('.col-stat').classes()).not.toContain('col-drag')
    w.unmount()
  })

  it('leaves no rename behind on a lane with no handle', async () => {
    // Renaming is gated by the same `canEdit`, so the caption of a handle-less
    // lane offers nothing at all — which is what the cursor now says.
    const w = mountHead({ editable: false, readonly: false })
    await w.get('.col-title').trigger('dblclick')
    expect(w.find('.col-title').exists()).toBe(true) // still the caption, no input
    w.unmount()
  })
})
