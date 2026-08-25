import { describe, it, expect, beforeEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { i18n, setI18nLocale } from '@/i18n'
import { notificationText, notificationTitle } from '@/utils/notificationText'
import { createFormatters } from '@/utils/format'

// Stage 5 of #2796 (#2801): the server stores the facts, the client writes the
// sentence. These specs pin both halves of that contract — the rendering itself,
// and the fallback that keeps pre-payload rows readable.

const N = (payload, text = 'старый текст') => ({ id: 'n1', kind: 'updated', payload, text })

const ru = () => ({
  t: i18n.global.t,
  te: i18n.global.te,
  formatters: createFormatters({ language: 'ru' }),
})
const en = () => ({
  t: i18n.global.t,
  te: i18n.global.te,
  formatters: createFormatters({ language: 'en' }),
})

describe('notificationText', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('renders each event in the active language', async () => {
    const cases = [
      [
        { event: 'task_assigned', actor: 'Иван', task_number: 42, title: 'Полить цветы' },
        'Иван назначил вам задачу #42 «Полить цветы»',
        'Иван assigned task #42 to you “Полить цветы”',
      ],
      [
        { event: 'task_moved', actor: 'Иван', task_number: 42, column: 'Готово' },
        'Иван переместил(а) задачу #42 → «Готово»',
        'Иван moved task #42 → “Готово”',
      ],
      [
        { event: 'task_mention', actor: 'Иван', task_number: 7, excerpt: 'глянь' },
        'Иван упомянул(а) вас в #7 «глянь»',
        'Иван mentioned you in #7 “глянь”',
      ],
      [
        { event: 'task_due_soon', task_number: 7, title: 'Полить цветы' },
        'Приближается срок задачи #7 «Полить цветы»',
        'Task #7 “Полить цветы” is due soon',
      ],
    ]
    for (const [payload, wantRu] of cases) {
      expect(notificationText(N(payload), ru())).toBe(wantRu)
    }
    await setI18nLocale('en')
    for (const [payload, , wantEn] of cases) {
      expect(notificationText(N(payload), en())).toBe(wantEn)
    }
  })

  // The changed fields arrive as machine names and are joined by Intl.ListFormat,
  // so the enumeration reads naturally in both languages instead of being a
  // comma-spliced list built on the server.
  it('renders a task_updated field list in the reader s language', () => {
    const payload = {
      event: 'task_updated',
      actor: 'Иван',
      task_number: 42,
      fields: ['title', 'due', 'estimate'],
    }
    expect(notificationText(N(payload), ru())).toBe(
      'Иван изменил(а) задачу #42: название, срок и оценка',
    )
  })

  it('drops a field name it does not know instead of showing it raw', () => {
    const payload = {
      event: 'task_updated',
      actor: 'Иван',
      task_number: 42,
      fields: ['title', 'colour'],
    }
    expect(notificationText(N(payload), ru())).toBe('Иван изменил(а) задачу #42: название')
  })

  it('renders a sync duration from seconds, not from a server-worded string', async () => {
    const sync = (seconds) =>
      notificationText(
        N({ event: 'integration_sync_ok', label: 'GitLab · a/b', created: 2, updated: 3, seconds }),
        ru(),
      )
    expect(sync(5)).toBe('GitLab · a/b: +2 новых, ~3 обновлено, за 5 с')
    expect(sync(80)).toBe('GitLab · a/b: +2 новых, ~3 обновлено, за 1 м 20 с')
    expect(sync(0)).toBe('GitLab · a/b: +2 новых, ~3 обновлено, за меньше секунды')
    await setI18nLocale('en')
    expect(
      notificationText(
        N({
          event: 'integration_sync_failed',
          label: 'GitLab · a/b',
          reason: '401',
          seconds: 3661,
        }),
        en(),
      ),
    ).toBe('GitLab · a/b: sync failed — 401 (after 1h 1m)')
  })

  // A reminder's body is the user's own text: content, never translated. Only
  // the empty-message default is a UI string.
  it('shows a reminder message verbatim and falls back to a translated default', async () => {
    expect(notificationText(N({ event: 'reminder', message: 'Позвонить' }), ru())).toBe('Позвонить')
    expect(notificationText(N({ event: 'reminder' }), ru())).toBe('Напоминание')
    await setI18nLocale('en')
    expect(notificationText(N({ event: 'reminder' }), en())).toBe('Reminder')
  })

  it('renders #? when the notification points at no task', () => {
    expect(notificationText(N({ event: 'task_comment', actor: 'Иван' }), ru())).toBe(
      'Иван прокомментировал #?',
    )
  })

  // Pitfall 2 of the #2796 plan: everything written before migration 0065 has an
  // empty payload and would otherwise render as a blank line.
  it('falls back to the stored text for a payload-less row', () => {
    expect(notificationText(N({}), ru())).toBe('старый текст')
    expect(notificationText({ text: 'без payload' }, ru())).toBe('без payload')
  })

  it('falls back to the stored text for an event newer than this bundle', () => {
    expect(notificationText(N({ event: 'task_teleported', actor: 'Иван' }), ru())).toBe(
      'старый текст',
    )
  })

  it('renders nothing rather than throwing on a missing notification', () => {
    expect(notificationText(null, ru())).toBe('')
    expect(notificationText(N({}, ''), ru())).toBe('')
  })
})

describe('notificationTitle', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('titles a native notification by kind', async () => {
    expect(notificationTitle('mention')).toBe('Вас упомянули')
    await setI18nLocale('en')
    expect(notificationTitle('mention')).toBe('You were mentioned')
  })

  it('falls back to the product name for an unknown kind', () => {
    expect(notificationTitle('teleported')).toBe('Tessera')
  })
})

// The event names are declared in Go and consumed here by key lookup, so nothing
// but a test connects the two: a builder added on the server without a locale key
// would silently degrade to the Russian `text` fallback in every language.
describe('locale coverage of the backend events', () => {
  it('has a message for every event the backend can emit', () => {
    const src = readFileSync(
      resolve(process.cwd(), '../backend/handlers/notification_content.go'),
      'utf8',
    )
    const events = [...src.matchAll(/^\s*ev\w+\s*=\s*"([a-z_]+)"/gm)].map((m) => m[1])
    expect(events.length).toBeGreaterThan(5)
    for (const ev of events) {
      expect(i18n.global.te(`notifications.event.${ev}`), `ru:${ev}`).toBe(true)
    }
  })

  it('has a Russian and English word for every field journalUpdate reports', () => {
    const src = readFileSync(resolve(process.cwd(), '../backend/handlers/tasks.go'), 'utf8')
    const fields = [...src.matchAll(/changed = append\(changed, "(\w+)"\)/g)].map((m) => m[1])
    expect(fields).toContain('title')
    for (const f of fields) {
      expect(i18n.global.te(`notifications.field.${f}`), `field:${f}`).toBe(true)
    }
  })
})
