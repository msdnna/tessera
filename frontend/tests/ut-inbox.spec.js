import { describe, it, expect } from 'vitest'
import { parseCapture } from '@/utils/quickParse'
import {
  groupInbox,
  recordState,
  snoozeOptions,
  quickRemindOptions,
  dueMoment,
  distance,
  firstLine,
  firedCount,
  doneTodayCount,
  remindersByTask,
} from '@/utils/inbox'

// «Входящие» (#2972): the capture line and the feed's grouping, on a fixed clock.
const now = new Date(2026, 9, 7, 9, 40) // Wed 7 Oct 2026, 09:40
const iso = (...a) => new Date(...a).toISOString()

describe('parseCapture', () => {
  it('reads the mockup line: tomorrow 10:00 → a reminder tied to a task', () => {
    const r = parseCapture('завтра 10:00 созвон по релизу #frontend /2951', { now })
    expect(r.kind).toBe('reminder')
    expect(r.remindAt).toBe(iso(2026, 9, 8, 10, 0))
    expect(r.title).toBe('Созвон по релизу')
    expect(r.taskRef).toEqual({ number: 2951 })
    expect(r.tags.map((t) => t.name)).toEqual(['frontend'])
  })

  it('a line without a moment is a note', () => {
    const r = parseCapture('идеи для онбординга', { now })
    expect(r.kind).toBe('note')
    expect(r.remindAt).toBeNull()
    expect(r.title).toBe('Идеи для онбординга')
  })

  it('a day without a time means 09:00', () => {
    const r = parseCapture('пт отчёт', { now })
    expect(r.remindAt).toBe(iso(2026, 9, 9, 9, 0))
  })

  it('a bare time already past today is tomorrow', () => {
    expect(parseCapture('08:00 зарядка', { now }).remindAt).toBe(iso(2026, 9, 8, 8, 0))
    expect(parseCapture('18:00 зарядка', { now }).remindAt).toBe(iso(2026, 9, 7, 18, 0))
  })

  it('counts «через 2 ч» and «через 30мин» from now', () => {
    expect(parseCapture('через 2 ч проверить', { now }).remindAt).toBe(iso(2026, 9, 7, 11, 40))
    expect(parseCapture('через 30мин проверить', { now }).remindAt).toBe(iso(2026, 9, 7, 10, 10))
    expect(parseCapture('через 3 дня', { now }).remindAt).toBe(iso(2026, 9, 10, 9, 40))
  })

  it('ties a task by word, keeps @handles out of the title', () => {
    const r = parseCapture('спросить @илья про /webhook', { now })
    expect(r.taskRef).toEqual({ word: 'webhook' })
    expect(r.mentions).toEqual(['илья'])
    expect(r.title).toBe('Спросить про')
  })
})

describe('groupInbox', () => {
  const rem = (id, at, extra = {}) => ({
    id,
    kind: 'reminder',
    remind_at: at,
    done: false,
    ...extra,
  })
  const reminders = [
    rem('fired', iso(2026, 9, 7, 9, 0)),
    rem('soon', iso(2026, 9, 7, 11, 0), { task: { board_id: 'b1' } }),
    rem('snoozed', iso(2026, 9, 7, 18, 0), { snoozed_from: iso(2026, 9, 7, 12, 0) }),
    rem('doneToday', iso(2026, 9, 7, 9, 15), { done: true, done_at: iso(2026, 9, 7, 9, 20) }),
    rem('doneYesterday', iso(2026, 9, 6, 9, 15), { done: true, done_at: iso(2026, 9, 6, 9, 20) }),
    rem('later', iso(2026, 9, 9, 14, 0)),
  ]
  const notes = [
    { id: 'n1', kind: 'note', updated_at: iso(2026, 9, 5) },
    { id: 'n2', kind: 'note', updated_at: iso(2026, 9, 6), task: { board_id: 'b1' } },
  ]

  it('splits into fired / today (done last) / later / notes', () => {
    const g = groupInbox({ reminders, notes }, { now })
    expect(g.map((x) => [x.key, x.items.map((i) => i.id)])).toEqual([
      ['fired', ['fired']],
      ['today', ['soon', 'snoozed', 'doneToday']],
      ['later', ['later']],
      ['notes', ['n2', 'n1']],
    ])
  })

  it('filters by kind and by the open board', () => {
    expect(groupInbox({ reminders, notes }, { now, filter: 'notes' }).map((g) => g.key)).toEqual([
      'notes',
    ])
    const b = groupInbox({ reminders, notes }, { now, filter: 'board', boardId: 'b1' })
    expect(b.flatMap((g) => g.items.map((i) => i.id))).toEqual(['soon', 'n2'])
    expect(groupInbox({ reminders, notes }, { now, filter: 'board' })).toEqual([])
  })

  it('counts fired and done today', () => {
    expect(firedCount(reminders, now)).toBe(1)
    expect(doneTodayCount(reminders, now)).toBe(1)
  })

  it('knows each state', () => {
    expect(reminders.map((r) => recordState(r, now))).toEqual([
      'fired',
      'upcoming',
      'snoozed',
      'done',
      'done',
      'upcoming',
    ])
  })

  it('maps open reminders by task, soonest first', () => {
    const m = remindersByTask([
      rem('a', iso(2026, 9, 8), { task_id: 't' }),
      rem('b', iso(2026, 9, 7), { task_id: 't' }),
      rem('c', iso(2026, 9, 7), { task_id: 't', done: true }),
    ])
    expect(m.get('t').map((r) => r.id)).toEqual(['b', 'a'])
  })
})

describe('snooze options', () => {
  it('counts from now; the evening only while ahead; due only when tied and ahead', () => {
    const task = { due_date: '2026-10-09T00:00:00Z' }
    const o = snoozeOptions(now, task)
    expect(o.map((x) => x.key)).toEqual(['hour', 'evening', 'morning', 'monday', 'due'])
    expect(o[0].at.getTime()).toBe(now.getTime() + 3600000)
    expect(o[3].at).toEqual(new Date(2026, 9, 12, 9, 0))
    expect(o[4].at).toEqual(new Date(2026, 9, 9, 18, 0))
    const late = new Date(2026, 9, 7, 20, 0)
    expect(snoozeOptions(late).map((x) => x.key)).toEqual(['hour', 'morning', 'monday'])
    // A Monday morning snoozes to next week's Monday.
    expect(snoozeOptions(new Date(2026, 9, 12, 8, 0))[3].at).toEqual(new Date(2026, 9, 19, 9, 0))
  })

  it('task modal offers hour / tomorrow / due', () => {
    expect(quickRemindOptions(now, { due_date: iso(2026, 9, 9, 15, 0) }).map((x) => x.key)).toEqual(
      ['hour', 'morning', 'due'],
    )
    expect(dueMoment(iso(2026, 9, 9, 15, 0))).toEqual(new Date(2026, 9, 9, 15, 0))
  })
})

describe('labels', () => {
  it('distance', () => {
    expect(distance(iso(2026, 9, 7, 9, 0), now)).toEqual({ key: 'min', past: true, n: 40 })
    expect(distance(iso(2026, 9, 7, 11, 0), now)).toEqual({ key: 'hm', past: false, h: 1, m: 20 })
    expect(distance(iso(2026, 9, 7, 19, 40), now)).toEqual({ key: 'h', past: false, n: 10 })
  })

  it('firstLine strips Markdown marks', () => {
    expect(firstLine('\n- [ ] **спринт** · баг-трекер\nещё')).toBe('спринт · баг-трекер')
    expect(firstLine('## Повестка')).toBe('Повестка')
    expect(firstLine('')).toBe('')
  })
})
