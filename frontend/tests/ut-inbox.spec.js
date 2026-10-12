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
  recordTags,
  recordPeople,
  parseTagLine,
  tagLine,
  tagsByProject,
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

  const people = [
    { user_id: 'u1', name: 'Анна Ким', email: 'anna@x.dev' },
    { user_id: 'u2', name: 'Илья Петров', email: 'ilya@x.dev' },
  ]

  it('ties a task by word, takes a member out of the title as a mention (#3040)', () => {
    const r = parseCapture('спросить @илья про /webhook', { now, people })
    expect(r.taskRef).toEqual({ word: 'webhook' })
    expect(r.people.map((p) => p.user_id)).toEqual(['u2'])
    expect(r.title).toBe('Спросить про')
  })

  it('keeps a handle that names nobody in the title', () => {
    const r = parseCapture('спросить @борис', { now, people })
    expect(r.people).toEqual([])
    expect(r.title).toBe('Спросить @борис')
  })

  it('reads «Созвон #релиз @Анна завтра 10:00»', () => {
    const tags = [{ id: 't1', name: 'Релиз', color: '#7c5cff' }]
    const r = parseCapture('Созвон #релиз @Анна завтра 10:00', { now, people, tags })
    expect(r.kind).toBe('reminder')
    expect(r.title).toBe('Созвон')
    expect(r.remindAt).toBe(iso(2026, 9, 8, 10, 0))
    expect(r.tags).toEqual([{ name: 'релиз', tag: tags[0] }])
    expect(r.people.map((p) => p.user_id)).toEqual(['u1'])
  })
})

describe('record tags and mentions (#3040)', () => {
  const wsTags = [
    { id: 't1', project_id: 'p1', name: 'Frontend', color: '#7c5cff' },
    { id: 't2', project_id: 'p2', name: 'релиз', color: '#e0533d' },
  ]
  const byProject = tagsByProject(wsTags)

  it('paints a tag of the record’s project, leaves the rest free labels', () => {
    const rec = { kind: 'note', tags: ['frontend', 'релиз'], task: { project_id: 'p1' } }
    expect(recordTags(rec, byProject)).toEqual([
      { name: 'frontend', tag: wsTags[0] },
      { name: 'релиз', tag: null },
    ])
    // A note written in a project, with no task, wears that project's tags.
    expect(recordTags({ tags: ['РЕЛИЗ'], project_id: 'p2' }, byProject)[0].tag).toBe(wsTags[1])
    // Without any project every tag is a free label.
    expect(recordTags({ tags: ['frontend'] }, byProject)[0].tag).toBe(null)
    expect(recordTags({}, byProject)).toEqual([])
  })

  it('lists the mentioned members in order, dropping the ones who left', () => {
    const members = [
      { user_id: 'u1', name: 'Анна Ким' },
      { user_id: 'u2', name: 'Илья Петров' },
    ]
    const got = recordPeople({ mentions: ['u2', 'gone', 'u1', 'u2'] }, members)
    expect(got.map((m) => m.user_id)).toEqual(['u2', 'u1'])
  })

  it('reads and writes the tag line of an opened record', () => {
    expect(parseTagLine('#релиз, frontend  #Релиз #идеи')).toEqual(['релиз', 'frontend', 'идеи'])
    expect(parseTagLine('')).toEqual([])
    expect(tagLine(['релиз', 'frontend'])).toBe('#релиз #frontend')
  })

  it('narrows the feed to a tag', () => {
    const later = new Date(2026, 9, 7, 15, 0).toISOString()
    const feed = {
      reminders: [
        { id: 'r1', kind: 'reminder', remind_at: later, tags: ['Релиз'] },
        { id: 'r2', kind: 'reminder', remind_at: later, tags: [] },
      ],
      notes: [
        { id: 'n1', kind: 'note', updated_at: later, tags: ['релиз', 'идеи'] },
        { id: 'n2', kind: 'note', updated_at: later },
      ],
    }
    const ids = (groups) => groups.flatMap((g) => g.items.map((r) => r.id))
    expect(ids(groupInbox(feed, { now, tag: 'релиз' }))).toEqual(['r1', 'n1'])
    expect(ids(groupInbox(feed, { now, tag: 'релиз', filter: 'notes' }))).toEqual(['n1'])
    expect(ids(groupInbox(feed, { now }))).toHaveLength(4)
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
