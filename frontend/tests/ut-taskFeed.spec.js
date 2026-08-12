import { describe, it, expect } from 'vitest'
import { fmtWhen, eventText } from '@/utils/taskFeed'

// The journal wording used to live inside TaskModal, where it could only be checked
// by reading it. It is user-facing text on every task, so pin it down.
describe('taskFeed.eventText', () => {
  it('renders every journal kind the backend emits', () => {
    const cases = [
      [{ kind: 'created' }, 'создал(а) задачу'],
      [{ kind: 'renamed', data: { to: 'Новое имя' } }, 'переименовал(а) → «Новое имя»'],
      [{ kind: 'description' }, 'изменил(а) описание'],
      [{ kind: 'due', data: { set: true } }, 'установил(а) срок'],
      [{ kind: 'due', data: { set: false } }, 'убрал(а) срок'],
      [{ kind: 'completed' }, 'отметил(а) выполненной'],
      [{ kind: 'reopened' }, 'вернул(а) в работу'],
      [{ kind: 'recurred' }, 'перенёс(ла) повтор задачи'],
      [{ kind: 'moved', data: { to: 'Готово' } }, 'переместил(а) → «Готово»'],
      [{ kind: 'assigned' }, 'назначил(а) исполнителя'],
      [{ kind: 'unassigned' }, 'снял(а) исполнителя'],
      [{ kind: 'archived' }, 'отправил(а) в архив'],
      [{ kind: 'restored' }, 'восстановил(а) из архива'],
      [{ kind: 'comment' }, 'оставил(а) комментарий'],
      [{ kind: 'relation', data: { related: 42 } }, 'добавил(а) связь с #42'],
      [{ kind: 'attachment', data: { filename: 'a.pdf' } }, 'прикрепил(а) файл «a.pdf»'],
    ]
    for (const [e, want] of cases) expect(eventText(e), e.kind).toBe(want)
  })

  it('degrades to the raw kind for an event it does not know', () => {
    expect(eventText({ kind: 'teleported' })).toBe('teleported')
  })

  it('survives events with no data payload', () => {
    // `moved` and `renamed` read e.data — an event without one must not throw.
    expect(eventText({ kind: 'moved' })).toBe('переместил(а)')
    expect(eventText({ kind: 'renamed' })).toBe('переименовал(а) → «»')
    expect(eventText({ kind: 'attachment' })).toBe('прикрепил(а) файл')
  })

  it('names the priority instead of printing its number', () => {
    expect(eventText({ kind: 'priority', data: { to: 3 } })).not.toMatch(/→ 3$/)
    // An out-of-range level falls back to the raw value rather than "undefined".
    expect(eventText({ kind: 'priority', data: { to: 99 } })).toBe('изменил(а) приоритет → 99')
  })
})

describe('taskFeed.fmtWhen', () => {
  it('formats a timestamp as day, short month, hh:mm', () => {
    expect(fmtWhen('2026-03-07T09:05:00Z')).toMatch(/^\d{2} .+, \d{2}:\d{2}$/)
  })
})
