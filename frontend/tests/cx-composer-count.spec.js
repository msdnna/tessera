// #2851: the composer bar shows «показано: N (M)» — how much the current facet
// set leaves on the board, cards first and then the total with every subtask
// folded in. Two things are pinned here.
//
// 1. The counter is derived from the filtered task list, NOT by summing the
//    column headers. With tag grouping a task carrying two column tags sits in
//    both lists, so the arithmetic sum double-counts it while the board shows
//    one card per column — the number under the search field would exceed the
//    number of distinct tasks on screen.
// 2. The message itself: brackets appear only when there are subtasks to fold
//    in, matching the column chip from #2850, and the hover title spells both
//    numbers out.
import { describe, it, expect, afterEach } from 'vitest'
import { i18n, setI18nLocale } from '@/i18n'
import { countWithSubtasks } from '@/utils/boardCounts'

afterEach(async () => {
  await setI18nLocale('ru')
})

const task = (id) => ({ id, title: id })
const t = (key, args) => i18n.global.t(`board.composer.${key}`, args)

// The component picks the message the same way; keep the choice in one place so
// the test exercises the rule rather than restating the strings.
const label = (c) =>
  c.total > c.tasks ? t('shown', { tasks: c.tasks, total: c.total }) : t('shownFlat', c)
const title = (c) =>
  c.total > c.tasks ? t('shownTitle', { tasks: c.tasks, total: c.total }) : t('shownTitleFlat', c)

describe('composer «showing» count', () => {
  it('counts the filtered cards and their whole tree', () => {
    // a → a1 → a11, plus a2; b alone. Cards: 2, tree: 5.
    const subs = { a: [task('a1'), task('a2')], a1: [task('a11')] }
    expect(countWithSubtasks([task('a'), task('b')], subs)).toEqual({ tasks: 2, total: 5 })
  })

  it('counts a task once even though tag grouping puts it in two columns', () => {
    // The board with tag columns: `a` carries both tags, so it renders as a card
    // in each. Summing the two headers gives 3; the bar must say 2.
    const cols = { design: [task('a'), task('b')], backend: [task('a')] }
    const sumOfHeaders = Object.values(cols)
      .map((list) => countWithSubtasks(list, {}).tasks)
      .reduce((x, y) => x + y, 0)
    expect(sumOfHeaders).toBe(3)

    const filtered = [task('a'), task('b')] // what filteredTasks actually holds
    expect(countWithSubtasks(filtered, {}).tasks).toBe(2)
  })

  it('is zero on an empty board', () => {
    const c = countWithSubtasks([], {})
    expect(c).toEqual({ tasks: 0, total: 0 })
    expect(label(c)).toBe('показано: 0')
  })

  it('renders the total in brackets after the card count', () => {
    expect(label({ tasks: 12, total: 34 })).toBe('показано: 12 (34)')
  })

  it('drops the brackets when nothing shown has subtasks', () => {
    // `показано: 12 (12)` would be noise, not information.
    expect(label({ tasks: 12, total: 12 })).toBe('показано: 12')
  })

  it('explains both numbers in the hover title', () => {
    const withSubs = title({ tasks: 12, total: 34 })
    expect(withSubs).toContain('12')
    expect(withSubs).toContain('34')

    const flat = title({ tasks: 12, total: 12 })
    expect(flat).toContain('12')
    expect(flat).not.toContain('34')
  })

  it('follows the interface language', async () => {
    await setI18nLocale('en')
    expect(label({ tasks: 12, total: 34 })).toBe('showing: 12 (34)')
    expect(label({ tasks: 12, total: 12 })).toBe('showing: 12')
  })
})
