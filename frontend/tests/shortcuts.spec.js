import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  handleKeydown,
  attach,
  resetShortcuts,
  pendingCaps,
  pushBlocker,
  SEQ_TIMEOUT,
} from '@/shortcuts/dispatcher'
import { parseKeys, matchChord } from '@/shortcuts/keymap'
import { chordsOf } from '@/shortcuts/labels'
import { CATALOG } from '@/shortcuts/catalog'
import { i18n } from '@/i18n'

// A keydown as the browser would send it. `code` is the physical key; `key`
// the character the active layout produced (Russian «п» for KeyG).
function press(code, opts = {}) {
  const e = new KeyboardEvent('keydown', {
    code,
    key: opts.key ?? '',
    bubbles: true,
    cancelable: true,
    ...opts,
  })
  if (opts.target) Object.defineProperty(e, 'target', { value: opts.target })
  handleKeydown(e)
  return e
}

beforeEach(() => {
  vi.useFakeTimers()
  resetShortcuts()
})
afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('keymap', () => {
  // Sentry TESSERA-FRONTEND-1C: <ShortcutKeys id="form.project"> crashed on
  // `undefined.trim()` — palette-only commands carry no keys.
  it('parses an entry without keys to no chords', () => {
    expect(parseKeys(undefined)).toEqual([])
    expect(parseKeys('')).toEqual([])
    for (const entry of CATALOG.filter((e) => !e.keys && !e.display)) {
      expect(chordsOf(entry.id)).toEqual([])
    }
  })

  it('matches letters by physical key, so the Russian layout still works', () => {
    const [g] = parseKeys('g')
    expect(matchChord(g, new KeyboardEvent('keydown', { code: 'KeyG', key: 'п' }))).toEqual({})
  })

  it('requires the exact modifiers', () => {
    const [n] = parseKeys('n')
    const [sn] = parseKeys('shift+n')
    const shifted = new KeyboardEvent('keydown', { code: 'KeyN', key: 'N', shiftKey: true })
    expect(matchChord(n, shifted)).toBeNull()
    expect(matchChord(sn, shifted)).toEqual({})
  })

  it('reads ? by character, whatever key produced it', () => {
    const [q] = parseKeys('?')
    expect(
      matchChord(q, new KeyboardEvent('keydown', { code: 'Digit7', key: '?', shiftKey: true })),
    ).toEqual({})
  })

  it('hands the digit of a digit chord to the handler', () => {
    const [d] = parseKeys('digit')
    expect(matchChord(d, new KeyboardEvent('keydown', { code: 'Digit3', key: '3' }))).toEqual({
      arg: 3,
    })
  })
})

describe('dispatcher', () => {
  it('runs a single-key shortcut', () => {
    const run = vi.fn()
    attach('card.complete', run)
    const e = press('KeyE', { key: 'e' })
    expect(run).toHaveBeenCalledOnce()
    expect(e.defaultPrevented).toBe(true)
  })

  it('walks a sequence and shows the pending key meanwhile', () => {
    const run = vi.fn()
    attach('nav.goBoard', run)
    press('KeyG', { key: 'g' })
    expect(pendingCaps.value).toEqual(['G'])
    expect(run).not.toHaveBeenCalled()
    press('KeyB', { key: 'b' })
    expect(run).toHaveBeenCalledOnce()
    expect(pendingCaps.value).toEqual([])
  })

  it('drops an unfinished sequence after the timeout', () => {
    const run = vi.fn()
    attach('nav.goBoard', run)
    press('KeyG')
    vi.advanceTimersByTime(SEQ_TIMEOUT + 1)
    press('KeyB')
    expect(run).not.toHaveBeenCalled()
  })

  it('fires the short binding on timeout when it is also a prefix (P vs P 0–4)', () => {
    const pick = vi.fn()
    const set = vi.fn()
    attach('card.priorityPick', pick)
    attach('card.priority', set)
    press('KeyP')
    expect(pick).not.toHaveBeenCalled()
    press('Digit2', { key: '2' })
    expect(set).toHaveBeenCalledWith(2, expect.anything())
    press('KeyP')
    vi.advanceTimersByTime(SEQ_TIMEOUT + 1)
    expect(pick).toHaveBeenCalledOnce()
  })

  it('fires an eager prefix at once and still completes the sequence', () => {
    const open = vi.fn()
    const task = vi.fn()
    attach('board.quick', open)
    attach('create.task', task)
    press('KeyN')
    expect(open).toHaveBeenCalledOnce()
    press('KeyT')
    expect(task).toHaveBeenCalledOnce()
  })

  it('leaves keys typed into a field alone, except input shortcuts', () => {
    const complete = vi.fn()
    const palette = vi.fn()
    attach('card.complete', complete)
    attach('nav.palette', palette)
    const input = document.createElement('input')
    press('KeyE', { target: input })
    expect(complete).not.toHaveBeenCalled()
    press('KeyK', { target: input, ctrlKey: true, metaKey: true })
    // ⌘ on Apple, Ctrl elsewhere — jsdom reports a non-Mac platform.
    expect(palette).not.toHaveBeenCalled()
    press('KeyK', { target: input, ctrlKey: true })
    expect(palette).toHaveBeenCalledOnce()
  })

  it('stays quiet under a blocking overlay, except modal shortcuts', () => {
    const complete = vi.fn()
    const palette = vi.fn()
    attach('card.complete', complete)
    attach('nav.palette', palette)
    const release = pushBlocker()
    press('KeyE')
    press('KeyK', { ctrlKey: true })
    expect(complete).not.toHaveBeenCalled()
    expect(palette).toHaveBeenCalledOnce()
    release()
    press('KeyE')
    expect(complete).toHaveBeenCalledOnce()
  })

  it('lets the latest live handler win and respects when()', () => {
    const board = vi.fn()
    const fab = vi.fn()
    let fabOpen = false
    attach('card.clear', board)
    attach('fab.close', fab, () => fabOpen)
    attach(
      'card.clear',
      () => {},
      () => false,
    ) // a dead one on top is skipped
    press('Escape')
    expect(board).toHaveBeenCalledOnce()
    fabOpen = true
    press('Escape')
    // card.clear and fab.close share Esc: the higher-prio entry wins.
    expect(fab).toHaveBeenCalledOnce()
    expect(board).toHaveBeenCalledOnce()
  })
})

describe('catalog', () => {
  it('labels every visible entry in both languages', () => {
    for (const e of CATALOG.filter((x) => x.group)) {
      expect(i18n.global.te(`shell.keys.${e.id}`, 'ru'), e.id).toBe(true)
    }
  })

  it('renders key caps for hints', () => {
    expect(chordsOf('nav.goBoard')).toEqual([['G'], ['B']])
    expect(chordsOf('card.priority')).toEqual([['P'], ['0–4']])
    expect(chordsOf('view.theme')).toEqual([['⇧', 'T']])
  })
})
