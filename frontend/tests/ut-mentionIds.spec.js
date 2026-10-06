// The set of members a comment notifies must be the set of mention chips the
// comment renders — no more, no less. The composer used to report only the rows
// picked from its own @-popup (matched by a bare substring), so a hand-typed or
// seeded "@Name" notified nobody while showing a resolved chip, and an "@name"
// inside a fenced code block notified someone for a mention that is never drawn.
// Every case below is stated as that invariant: mentionIdsIn(src) === the ids
// renderRich puts on the chips.
import { describe, it, expect } from 'vitest'
import { mentionIdsIn } from '@/utils/mentions'
import { renderRich } from '@/utils/markdown'

const ITEMS = [
  { id: 'u1', label: 'Ann Lee', display: 'Ann Lee' },
  // GitLab-linked member: inserted by login, still written by name in older text.
  { id: 'u2', label: 'e.polyansky', display: 'Евгений Полянский', username: 'e.polyansky' },
  { id: 'u3', label: 'Иван', display: 'Иван' },
  { id: 'u4', label: 'Иванов', display: 'Иванов' },
  // GitLab-only row: no Tessera account, so no Tessera notification.
  { id: null, label: 'v.sokolov', display: 'Виктор Соколов', gitlab: true },
]

// The ids actually drawn as resolved chips, read back off the rendered HTML.
function renderedIds(src) {
  const doc = new DOMParser().parseFromString(renderRich(src, ITEMS), 'text/html')
  const out = []
  for (const el of doc.querySelectorAll('span.mention[data-id]')) {
    const id = el.getAttribute('data-id')
    if (id && !out.includes(id)) out.push(id)
  }
  return out
}

function bothWays(src) {
  return { sent: mentionIdsIn(src, ITEMS), drawn: renderedIds(src) }
}

describe('mentionIdsIn', () => {
  it('notifies a hand-typed mention — the popup is a convenience, not the record', () => {
    const { sent, drawn } = bothWays('@Ann Lee, глянь пожалуйста')
    expect(sent).toEqual(['u1'])
    expect(sent).toEqual(drawn)
  })

  it('notifies the display name of a GitLab-linked member', () => {
    // What startReply seeds: "@" + author_name, never the login.
    const { sent, drawn } = bothWays('@Евгений Полянский, согласен')
    expect(sent).toEqual(['u2'])
    expect(sent).toEqual(drawn)
  })

  it('notifies the login spelling of the same member', () => {
    expect(mentionIdsIn('@e.polyansky ок', ITEMS)).toEqual(['u2'])
  })

  it('stays silent for a mention inside a fenced code block', () => {
    const src = ['Пример:', '', '```', 'curl -u @Иван http://x', '```', ''].join('\n')
    const { sent, drawn } = bothWays(src)
    expect(drawn).toEqual([])
    expect(sent).toEqual([])
  })

  it('stays silent for a mention inside inline code', () => {
    const { sent, drawn } = bothWays('поле `@Ann Lee` это литерал')
    expect(drawn).toEqual([])
    expect(sent).toEqual([])
  })

  it('does not notify a shorter member whose name is a prefix of the written handle', () => {
    const { sent, drawn } = bothWays('@Иванов посмотри')
    expect(sent).toEqual(['u4'])
    expect(sent).toEqual(drawn)
  })

  // The written handle is nobody's: the roster has "Иван" and "Иванов", not
  // "Ивановна". A prefix match used to hand it to "Иван" — a resolved chip reading
  // "@Иван" followed by the stray "овна", and a "you were mentioned" for someone
  // the author never named (#2932).
  it('notifies nobody when the written handle merely starts with a member name', () => {
    const { sent, drawn } = bothWays('@Ивановна посмотри')
    expect(sent).toEqual([])
    expect(sent).toEqual(drawn)
  })

  it('draws an unknown ASCII handle as itself rather than as the member it starts with', () => {
    // "e.polyansky" is a member; "e.polyanskyy" is not, and the generic token has
    // to claim the whole of it.
    const { sent, drawn } = bothWays('@e.polyanskyy посмотри')
    expect(sent).toEqual([])
    expect(sent).toEqual(drawn)
    expect(renderRich('@e.polyanskyy посмотри', ITEMS)).toContain('>@e.polyanskyy<')
  })

  it('keeps a mention that ends the sentence on a full stop', () => {
    // The boundary must not swallow punctuation: a dot closes a handle unless a
    // word follows it, so "@Иван." is still Иван and "@Иван.Петров" is neither.
    expect(mentionIdsIn('спроси @Иван.', ITEMS)).toEqual(['u3'])
    expect(mentionIdsIn('спроси @Иван.Петрова', ITEMS)).toEqual([])
  })

  it('leaves a GitLab-only handle without a Tessera notification', () => {
    const { sent, drawn } = bothWays('@v.sokolov привет')
    expect(drawn).toEqual([])
    expect(sent).toEqual([])
  })

  it('ignores an unknown handle', () => {
    expect(mentionIdsIn('@nobody.here привет', ITEMS)).toEqual([])
  })

  it('reports each member once however many times they are written', () => {
    expect(mentionIdsIn('@Иван и ещё раз @Иван', ITEMS)).toEqual(['u3'])
  })

  it('collects several mentions in written order', () => {
    const { sent, drawn } = bothWays('@Ann Lee и @Иван — на вас')
    expect(sent).toEqual(['u1', 'u3'])
    expect(sent).toEqual(drawn)
  })

  it('is empty for blank content and for no roster', () => {
    expect(mentionIdsIn('', ITEMS)).toEqual([])
    expect(mentionIdsIn('@Ann Lee', [])).toEqual([])
    expect(mentionIdsIn('@Ann Lee', null)).toEqual([])
  })

  it('does not treat an e-mail address as a mention of its user part', () => {
    // "ann@t.io" — the "@" is mid-token, so nothing is drawn and nobody is told.
    const { sent, drawn } = bothWays('пиши на Иван@t.io')
    expect(drawn).toEqual([])
    expect(sent).toEqual([])
  })
})
