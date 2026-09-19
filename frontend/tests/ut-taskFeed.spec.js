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
      // Any member may drop any member's file, and the bytes go with the row —
      // so the removal is the half of the pair that most needs a name on it.
      [{ kind: 'attachment_removed', data: { filename: 'a.pdf' } }, 'удалил(а) файл «a.pdf»'],
      // The tag travels by name: a board grouped by tags turns a drag into an
      // attach + a detach, and those two lines are the only trace it leaves.
      [{ kind: 'tagged', data: { tag: 'срочно' } }, 'навесил(а) тег «срочно»'],
      [{ kind: 'untagged', data: { tag: 'срочно' } }, 'снял(а) тег «срочно»'],
      // The stage is named for the tag's reason: «Этапы» recomputes its progress
      // from this pointer, and the milestone may be renamed or deleted later.
      [{ kind: 'milestone', data: { title: 'Этап 1' } }, 'привязал(а) к этапу «Этап 1»'],
      [{ kind: 'milestone_removed', data: { title: 'Этап 1' } }, 'отвязал(а) от этапа «Этап 1»'],
    ]
    for (const [e, want] of cases) expect(eventText(e), e.kind).toBe(want)
  })

  // The `default:` branch prints the raw kind, which is a machine token in
  // English — fine for an event this build has never heard of, a bug for one the
  // backend writes today. These are the kinds `logEvent`/`logEventActor` emit
  // (handlers/collab.go, tasks.go, task_ops.go, attachments.go, recurrence.go,
  // gitlab*.go); every one of them has to come out as a sentence in both languages.
  const EMITTED_KINDS = [
    'created',
    'renamed',
    'description',
    'priority',
    'due',
    'start',
    'estimate',
    'completed',
    'reopened',
    'recurred',
    'moved',
    'assigned',
    'unassigned',
    'archived',
    'restored',
    'comment',
    'commands',
    'relation',
    'relation_removed',
    'tagged',
    'untagged',
    'attachment',
    'attachment_removed',
    'milestone',
    'milestone_removed',
    'synced',
  ]

  it('never falls through to the raw kind for a kind the backend emits', async () => {
    const { setI18nLocale } = await import('@/i18n')
    for (const lang of ['ru', 'en']) {
      await setI18nLocale(lang)
      for (const kind of EMITTED_KINDS) {
        const out = eventText({ kind })
        expect(out, `${lang}/${kind}`).toBeTruthy()
        expect(out, `${lang}/${kind}`).not.toBe(kind)
        // A missing catalog entry renders as the key path, not as the token.
        expect(out, `${lang}/${kind}`).not.toMatch(/^task\.journal\./)
      }
    }
    await setI18nLocale('ru')
  })

  it('words the kinds that were missing from the feed', () => {
    expect(eventText({ kind: 'start', data: { set: true } })).toBe('установил(а) дату начала')
    expect(eventText({ kind: 'start', data: { set: false } })).toBe('убрал(а) дату начала')
    expect(eventText({ kind: 'estimate', data: { set: true } })).toBe('установил(а) оценку')
    expect(eventText({ kind: 'estimate', data: { set: false } })).toBe('убрал(а) оценку')
    expect(eventText({ kind: 'relation_removed', data: { related: 42 } })).toBe(
      'удалил(а) связь с #42',
    )
    // The source is captioned the way the relation badges caption it, so the
    // journal doesn't invent a second spelling of "GitLab".
    expect(eventText({ kind: 'synced', data: { source: 'gitlab' } })).toBe(
      'синхронизировал(а) с GitLab',
    )
  })

  // One aggregated entry per comment (collab.go), so the line has to name which
  // commands ran — "ran quick actions" would hide that /close fired.
  it('names the quick actions a comment ran, and the ones that failed', () => {
    expect(eventText({ kind: 'commands', data: { applied: ['due', 'assign'], failed: [] } })).toBe(
      'выполнил(а) команды: /due, /assign',
    )
    expect(eventText({ kind: 'commands', data: { applied: [], failed: ['assign'] } })).toBe(
      'не смог(ла) выполнить команды: /assign',
    )
    expect(eventText({ kind: 'commands', data: { applied: ['due'], failed: ['assign'] } })).toBe(
      'выполнил(а) команды: /due (не удалось: /assign)',
    )
  })

  it('degrades to the raw kind for an event it does not know', () => {
    expect(eventText({ kind: 'teleported' })).toBe('teleported')
  })

  it('survives events with no data payload', () => {
    // `moved` and `renamed` read e.data — an event without one must not throw.
    expect(eventText({ kind: 'moved' })).toBe('переместил(а)')
    expect(eventText({ kind: 'renamed' })).toBe('переименовал(а) → «»')
    expect(eventText({ kind: 'attachment' })).toBe('прикрепил(а) файл')
    expect(eventText({ kind: 'attachment_removed' })).toBe('удалил(а) файл')
  })

  it('names the priority instead of printing its number', () => {
    expect(eventText({ kind: 'priority', data: { to: 3 } })).not.toMatch(/→ 3$/)
    // An out-of-range level falls back to the raw value rather than "undefined".
    expect(eventText({ kind: 'priority', data: { to: 99 } })).toBe('изменил(а) приоритет → 99')
  })

  // A move lands on a seeded column as often as not, and those carry a name_key
  // (#2800) — the journal must caption it the way the board does, while a column
  // the user named (no key) reads verbatim in every language.
  it('captions a seeded target column in the active language', async () => {
    const { setI18nLocale } = await import('@/i18n')
    const moved = { kind: 'moved', data: { to: 'Готово', to_key: 'done' } }
    expect(eventText(moved)).toBe('переместил(а) → «Готово»')
    await setI18nLocale('en')
    expect(eventText(moved)).toBe('moved it to “Done”')
    expect(eventText({ kind: 'moved', data: { to: 'Протестировано' } })).toBe(
      'moved it to “Протестировано”',
    )
    await setI18nLocale('ru')
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
