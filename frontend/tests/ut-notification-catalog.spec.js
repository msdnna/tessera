import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import ru from '@/locales/ru'
import en from '@/locales/en'

// The notification catalog exists twice: once in the bundles the web app renders
// the bell feed from, once in `backend/internal/notify/locale.go`, which words the
// same event for a channel that leaves the app (email, telegram, push). The header
// of that file says they are twins — same keys, same placeholders, same wording —
// but nothing enforced it, and the two are edited in different commits by whoever
// touches one side.
//
// Reading the Go source here is the same trick `ut-command-labels.spec.js` plays on
// the quick-action registry: the backend stays the source of truth and the test
// fails the moment the client's copy drifts.
const SOURCE = readFileSync(resolve(__dirname, '../../backend/internal/notify/locale.go'), 'utf8')

// The Go literal for one language, from `"<lang>": {` to the line that closes it.
function goBundle(lang) {
  const start = SOURCE.indexOf(`\t"${lang}": {`)
  expect(start, `bundle ${lang} in locale.go`).toBeGreaterThan(-1)
  return SOURCE.slice(start, SOURCE.indexOf('\n\t},\n', start))
}

// One `name: map[string]string{ … }` inside a bundle, as a plain object.
function goMap(bundle, name) {
  const start = bundle.indexOf(`${name}: map[string]string{`)
  expect(start, `${name} map`).toBeGreaterThan(-1)
  const body = bundle.slice(start, bundle.indexOf('\n\t\t},', start))
  return Object.fromEntries(
    [...body.matchAll(/"([a-z_]+)":\s*"([^"]*)"/g)].map((m) => [m[1], m[2]]),
  )
}

// One `name: "…"` field of a bundle.
function goString(bundle, name) {
  return bundle.match(new RegExp(`${name}:\\s*"([^"]*)"`))?.[1]
}

const BUNDLES = [
  ['ru', ru],
  ['en', en],
]

describe('the notification bundles are twins of the backend catalog', () => {
  it('reads a plausible catalog off the backend source', () => {
    const b = goBundle('ru')
    expect(Object.keys(goMap(b, 'events')).length).toBeGreaterThan(10)
    expect(goMap(b, 'events')).toHaveProperty('task_assigned')
    expect(goMap(b, 'kinds')).toHaveProperty('assigned')
  })

  for (const [lang, bundle] of BUNDLES) {
    // `test` is the subject of the probe a user sends to check a channel they just
    // set up: it is never stored as a notification, so it has no feed row to word
    // and the client has no use for it. Every other kind reaches both sides.
    const SERVER_ONLY_KINDS = ['test']

    it(`${lang}: every event, field and kind matches word for word`, () => {
      const go = goBundle(lang)
      for (const [goName, feName, serverOnly] of [
        ['events', 'event', []],
        ['fields', 'field', []],
        ['kinds', 'kind', SERVER_ONLY_KINDS],
      ]) {
        const server = goMap(go, goName)
        const client = bundle.notifications[feName]
        const expected = Object.keys(server).filter((k) => !serverOnly.includes(k))
        expect(Object.keys(client).sort(), `${lang}.${feName} key set`).toEqual(expected.sort())
        for (const key of expected) {
          expect(client[key], `${lang}.${feName}.${key}`).toBe(server[key])
        }
      }
    })

    it(`${lang}: the inline-title wrapper, the durations and the fallback reason match`, () => {
      const go = goBundle(lang)
      const n = bundle.notifications
      expect(n.ctx, `${lang}.ctx`).toBe(goString(go, 'ctx'))
      expect(n.duration.instant, `${lang}.duration.instant`).toBe(goString(go, 'instant'))
      expect(n.duration.hm, `${lang}.duration.hm`).toBe(goString(go, 'durationHM'))
      expect(n.duration.ms, `${lang}.duration.ms`).toBe(goString(go, 'durationMS'))
      expect(n.duration.s, `${lang}.duration.s`).toBe(goString(go, 'durationS'))
      expect(n.reason.unknown, `${lang}.reason.unknown`).toBe(goString(go, 'unknownReason'))
    })
  }
})

// `common.duration` (the background-jobs and GitLab-journal ticker, utils/duration.js)
// and `notifications.duration` (the bell feed, utils/notificationText.js) spell the
// same elapsed time, and a sync run shows both: the journal row says how long it took,
// the notification about that same run says "in {took}". They were written in separate
// commits and the English pair drifted apart — "1 h 3 m" in the journal against "1h 3m"
// in the notification, the email and the Android feed. Russian never diverged, which is
// what gave it away as an oversight rather than a choice.
describe('both duration catalogs spell an elapsed time the same way', () => {
  for (const [lang, bundle] of BUNDLES) {
    it(`${lang}: the journal ticker and the notification feed agree`, () => {
      for (const key of ['hm', 'ms', 's']) {
        expect(bundle.common.duration[key], `${lang}.common.duration.${key}`).toBe(
          bundle.notifications.duration[key],
        )
      }
    })
  }
})
