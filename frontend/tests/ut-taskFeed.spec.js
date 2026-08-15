import { describe, it, expect } from 'vitest'
import { fmtWhen, eventText, groupThreads } from '@/utils/taskFeed'

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

// The API returns a flat array (Android and the MCP server read it that way);
// the tree is assembled on the client, so that assembly is what needs pinning.
describe('taskFeed.groupThreads', () => {
  it('hangs replies off their root and keeps roots in order', () => {
    const threads = groupThreads([
      { id: 'a', parent_id: null },
      { id: 'a1', parent_id: 'a' },
      { id: 'a2', parent_id: 'a' },
      { id: 'b', parent_id: null },
    ])
    expect(threads.map((t) => t.root.id)).toEqual(['a', 'b'])
    expect(threads[0].replies.map((r) => r.id)).toEqual(['a1', 'a2'])
    expect(threads[1].replies).toEqual([])
  })

  it('promotes an orphaned reply to a root instead of dropping it', () => {
    // The parent was deleted (or filtered out): losing the text entirely is
    // worse than showing it unindented.
    const threads = groupThreads([
      { id: 'a', parent_id: null },
      { id: 'x', parent_id: 'gone' },
    ])
    expect(threads.map((t) => t.root.id)).toEqual(['a', 'x'])
  })

  it('keeps a reply visible when it precedes its root in the list', () => {
    const threads = groupThreads([
      { id: 'a1', parent_id: 'a' },
      { id: 'a', parent_id: null },
    ])
    expect(threads.map((t) => t.root.id)).toEqual(['a1', 'a'])
  })

  it('handles an empty or missing list', () => {
    expect(groupThreads([])).toEqual([])
    expect(groupThreads(undefined)).toEqual([])
  })
})
