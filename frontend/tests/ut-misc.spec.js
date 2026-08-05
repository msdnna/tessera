import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { humanizeError } from '@/utils/errors'
import { makeSlug } from '@/utils/slug'
import { initials } from '@/utils/initials'
import { BACKLOG_SCOPE, matchesScope, milestoneKey, milestoneRange } from '@/utils/milestones'
import { iconComponent, iconKind, sanitizeIconSvg, PROJECT_ICONS } from '@/utils/projectIcons'
import {
  isTauri,
  serverBase,
  setServerBase,
  apiBaseURL,
  absolutizeApiUrl,
  wsURL,
} from '@/utils/serverBase'

// ── slug.js ────────────────────────────────────────────────────────────
// The address preview must match the server byte for byte, otherwise the user
// approves one address and gets another. These are the cases from the Go side
// (internal/slug/slug_test.go) — keep both lists in step.
describe('makeSlug', () => {
  it('matches the server transliteration', () => {
    const cases = [
      ['Общие задачи', 'obshchie-zadachi'],
      ['Hello, World!', 'hello-world'],
      ['  Проект №1  ', 'proekt-1'],
      ['щётка-ёж', 'shchetka-ezh'],
      ['объём', 'obem'], // hard/soft signs drop
      ['---', ''],
      ['', ''],
      ['MiXeD Кейс 42', 'mixed-keys-42'],
    ]
    for (const [input, want] of cases) {
      expect(makeSlug(input), `makeSlug(${JSON.stringify(input)})`).toBe(want)
    }
  })

  it('collapses separator runs and trims the edges', () => {
    expect(makeSlug('a   b___c')).toBe('a-b-c')
    expect(makeSlug('!!!Проект!!!')).toBe('proekt')
  })

  it('treats nullish input as empty', () => {
    expect(makeSlug(null)).toBe('')
    expect(makeSlug(undefined)).toBe('')
  })
})

// ── errors.js ──────────────────────────────────────────────────────────
describe('humanizeError', () => {
  it('generic fallback for empty input', () => {
    expect(humanizeError('')).toBe('Что-то пошло не так')
    expect(humanizeError(null)).toBe('Что-то пошло не так')
  })

  it('maps known english sentinels (case-insensitive)', () => {
    expect(humanizeError('invalid credentials')).toBe('Неверный email или пароль')
    expect(humanizeError('FORBIDDEN')).toBe('Недостаточно прав')
    expect(humanizeError('  not found  ')).toBe('Не найдено')
  })

  it('collapses gin/validator errors by failed tag', () => {
    const email = "Key: 'Email' Error:Field validation for 'Email' failed on the 'email' tag"
    expect(humanizeError(email)).toBe('Введите корректный email')
    const req = "Key: 'Name' Error:Field validation for 'Name' failed on the 'required' tag"
    expect(humanizeError(req)).toBe('Заполните все обязательные поля')
    const min = "Key: 'Pw' Error:Field validation for 'Pw' failed on the 'min' tag"
    expect(humanizeError(min)).toBe('Значение слишком короткое')
  })

  it('recognises network-ish messages', () => {
    expect(humanizeError('Network Error')).toContain('Нет связи с сервером')
    expect(humanizeError('connect ECONNREFUSED')).toContain('Нет связи с сервером')
  })

  it('passes an unknown short message through unchanged', () => {
    expect(humanizeError('Своя ошибка')).toBe('Своя ошибка')
  })
})

// ── initials.js ────────────────────────────────────────────────────────
describe('initials', () => {
  it('two cyrillic words → two capitals', () => {
    expect(initials('Василий Соколов')).toBe('ВС')
  })

  it('dot-separated handle → first of each part', () => {
    expect(initials('a.fokin')).toBe('AF')
    expect(initials('v.sokolov')).toBe('VS')
  })

  it('single token → first two letters uppercased', () => {
    expect(initials('msdnna')).toBe('MS')
  })

  it('empty / null → placeholder', () => {
    expect(initials('')).toBe('?')
    expect(initials(null)).toBe('?')
    expect(initials('   ')).toBe('?')
  })
})

// ── milestones.js ──────────────────────────────────────────────────────
describe('milestoneRange', () => {
  it('both dates → a range', () => {
    expect(milestoneRange({ start_date: '2026-06-01', due_date: '2026-06-30' })).toContain('–')
  })

  it('due-only → "до …", start-only → "с …"', () => {
    expect(milestoneRange({ due_date: '2026-06-30' }).startsWith('до ')).toBe(true)
    expect(milestoneRange({ start_date: '2026-06-01' }).startsWith('с ')).toBe(true)
  })

  it('empty for no milestone or no dates', () => {
    expect(milestoneRange(null)).toBe('')
    expect(milestoneRange({})).toBe('')
  })
})

describe('milestoneKey / matchesScope', () => {
  const ms = { id: 'a1b2', slug: 'sprint-1' }

  it('milestoneKey prefers the slug, falls back to the id', () => {
    expect(milestoneKey(ms)).toBe('sprint-1')
    expect(milestoneKey({ id: 'a1b2', slug: '' })).toBe('a1b2')
    expect(milestoneKey(null)).toBe(BACKLOG_SCOPE)
  })

  it('matchesScope accepts both the slug and the legacy uuid', () => {
    expect(matchesScope(ms, 'sprint-1')).toBe(true)
    expect(matchesScope(ms, 'a1b2')).toBe(true)
    expect(matchesScope(ms, 'sprint-2')).toBe(false)
    expect(matchesScope(ms, '')).toBe(false)
  })

  it('the backlog scope matches the no-milestone node only', () => {
    expect(matchesScope(null, BACKLOG_SCOPE)).toBe(true)
    expect(matchesScope(ms, BACKLOG_SCOPE)).toBe(false)
  })
})

// ── projectIcons.js ────────────────────────────────────────────────────
describe('projectIcons', () => {
  it('iconComponent resolves curated keys, null otherwise', () => {
    expect(iconComponent('home')).toBe(PROJECT_ICONS.find((i) => i.key === 'home').component)
    expect(iconComponent('nope')).toBeNull()
    expect(iconComponent('')).toBeNull()
  })

  it('iconKind classifies the stored value', () => {
    expect(iconKind('')).toBe('none')
    expect(iconKind(null)).toBe('none')
    expect(iconKind('data:image/png;base64,AAA')).toBe('img')
    expect(iconKind('  <svg viewBox="0 0 1 1"></svg>')).toBe('svg')
    expect(iconKind('rocket')).toBe('curated')
    expect(iconKind('unknownkey')).toBe('none')
  })

  it('sanitizeIconSvg strips scripts but keeps svg markup', () => {
    const out = sanitizeIconSvg('<svg><script>alert(1)</script><rect/></svg>')
    expect(out.toLowerCase()).not.toContain('<script')
    expect(out).toContain('<svg')
  })
})

// ── serverBase.js ──────────────────────────────────────────────────────
describe('serverBase (web / no Tauri)', () => {
  beforeEach(() => {
    delete window.__TAURI__
    delete window.__TAURI_INTERNALS__
    localStorage.clear()
  })

  it('reports not-Tauri and same-origin defaults', () => {
    expect(isTauri()).toBe(false)
    expect(serverBase()).toBe('')
    expect(apiBaseURL()).toBe('/api')
  })

  it('absolutizeApiUrl is a no-op on web', () => {
    expect(absolutizeApiUrl('/api/x')).toBe('/api/x')
    expect(absolutizeApiUrl(null)).toBeNull()
  })

  it('wsURL derives from location on web', () => {
    expect(wsURL()).toMatch(/^wss?:\/\/.+\/api\/ws$/)
  })
})

describe('serverBase (Tauri desktop)', () => {
  beforeEach(() => {
    window.__TAURI_INTERNALS__ = {}
    localStorage.clear()
  })
  afterEach(() => {
    delete window.__TAURI_INTERNALS__
    localStorage.clear()
  })

  it('defaults to the production origin, editable via setServerBase', () => {
    expect(isTauri()).toBe(true)
    expect(serverBase()).toBe('https://tessera.msdnna.website')
    setServerBase('https://my.host/')
    expect(serverBase()).toBe('https://my.host') // trailing slash trimmed
    expect(apiBaseURL()).toBe('https://my.host/api')
  })

  it('setServerBase("") clears back to default', () => {
    setServerBase('https://x.y')
    setServerBase('')
    expect(serverBase()).toBe('https://tessera.msdnna.website')
  })

  it('absolutizeApiUrl prefixes root-relative /api urls', () => {
    setServerBase('https://my.host')
    expect(absolutizeApiUrl('/api/avatar/1')).toBe('https://my.host/api/avatar/1')
    expect(absolutizeApiUrl('https://cdn/x')).toBe('https://cdn/x') // absolute untouched
  })

  it('wsURL derives scheme/host from the stored base', () => {
    setServerBase('https://my.host')
    expect(wsURL()).toBe('wss://my.host/api/ws')
    setServerBase('http://local:8080')
    expect(wsURL()).toBe('ws://local:8080/api/ws')
  })
})

// ── dnd.js (module side-effects: window touch listeners) ────────────────
describe('dnd pressMoved', () => {
  it('is false initially and set by a touch move beyond threshold', async () => {
    const { pressMoved } = await import('@/utils/dnd')
    expect(pressMoved()).toBe(false)
    window.dispatchEvent(new TouchEvent('touchstart', { touches: [{ clientX: 0, clientY: 0 }] }))
    window.dispatchEvent(new TouchEvent('touchmove', { touches: [{ clientX: 50, clientY: 0 }] }))
    expect(pressMoved()).toBe(true)
    // a mousedown clears the flag
    window.dispatchEvent(new MouseEvent('mousedown'))
    expect(pressMoved()).toBe(false)
  })
})

// ── localeOptions.js ───────────────────────────────────────────────────
describe('localeOptions', () => {
  it('timezoneOptions returns {label,value} pairs with label === value', async () => {
    const { timezoneOptions } = await import('@/utils/localeOptions')
    const opts = timezoneOptions()
    expect(opts.length).toBeGreaterThan(0)
    expect(opts[0]).toHaveProperty('label')
    expect(opts[0]).toHaveProperty('value')
    expect(opts.every((o) => o.label === o.value)).toBe(true)
    expect(opts.some((o) => /\//.test(o.value))).toBe(true) // e.g. Europe/Moscow
  })

  it('countryOptions localises regions and drops unassigned codes', async () => {
    const { countryOptions } = await import('@/utils/localeOptions')
    const ru = countryOptions('ru')
    expect(ru.length).toBeGreaterThan(50)
    expect(ru.every((o) => o.value !== o.label)).toBe(true) // code === label rows dropped
    expect(ru.some((o) => o.value === 'RU')).toBe(true)
  })
})

// ── device.js ──────────────────────────────────────────────────────────
describe('device', () => {
  beforeEach(() => {
    delete window.__TAURI__
    delete window.__TAURI_INTERNALS__
    localStorage.clear()
  })

  it('getDeviceId persists a stable id across calls', async () => {
    vi.resetModules()
    const { getDeviceId } = await import('@/utils/device')
    const a = getDeviceId()
    const b = getDeviceId()
    expect(a).toBe(b)
    expect(localStorage.getItem('tessera_device_id')).toBe(a)
  })

  it('deviceLabel names the browser from the UA on web', async () => {
    const { deviceLabel } = await import('@/utils/device')
    const spy = vi.spyOn(navigator, 'userAgent', 'get')
    spy.mockReturnValue('Mozilla/5.0 Firefox/128.0')
    expect(deviceLabel()).toBe('Браузер (Firefox)')
    spy.mockReturnValue('Mozilla/5.0 Chrome/120 Safari/537')
    expect(deviceLabel()).toBe('Браузер (Chrome)')
    spy.mockReturnValue('Mozilla/5.0 Edg/120')
    expect(deviceLabel()).toBe('Браузер (Edge)')
    spy.mockReturnValue('unknown-agent')
    expect(deviceLabel()).toBe('Браузер (браузер)')
    spy.mockRestore()
  })

  it('deviceLabel names the OS in the desktop (Tauri) app', async () => {
    window.__TAURI_INTERNALS__ = {}
    const { deviceLabel } = await import('@/utils/device')
    const spy = vi.spyOn(navigator, 'userAgent', 'get')
    spy.mockReturnValue('X11; Linux x86_64')
    expect(deviceLabel()).toBe('Настольное приложение (Linux)')
    spy.mockReturnValue('Windows NT 10.0')
    expect(deviceLabel()).toBe('Настольное приложение (Windows)')
    spy.mockRestore()
    delete window.__TAURI_INTERNALS__
  })

  it('notificationsSupported is always true in the desktop app', async () => {
    window.__TAURI_INTERNALS__ = {}
    const { notificationsSupported } = await import('@/utils/device')
    expect(notificationsSupported()).toBe(true)
    delete window.__TAURI_INTERNALS__
  })

  it('getDeviceId falls back to a non-crypto id when randomUUID is absent', async () => {
    localStorage.clear()
    const orig = globalThis.crypto
    // crypto without randomUUID → fallback branch (web-<ts>-<rand>)
    Object.defineProperty(globalThis, 'crypto', { value: {}, configurable: true })
    vi.resetModules()
    const { getDeviceId } = await import('@/utils/device')
    const id = getDeviceId()
    expect(id).toMatch(/^web-/)
    Object.defineProperty(globalThis, 'crypto', { value: orig, configurable: true })
  })

  it('notificationsSupported reflects window.Notification on web', async () => {
    const { notificationsSupported } = await import('@/utils/device')
    const expected = 'Notification' in window
    expect(notificationsSupported()).toBe(expected)
  })
})

// ── sources.js ─────────────────────────────────────────────────────────
describe('sources', () => {
  it('sourceMeta labels known sources; only an integration carries an icon', async () => {
    const { sourceMeta } = await import('@/utils/sources')
    expect(sourceMeta('user').label).toBe('Tessera')
    expect(sourceMeta('user').icon).toBe(null)
    expect(sourceMeta('gitlab').label).toBe('GitLab')
    expect(sourceMeta('gitlab').icon).toBeTruthy()
  })

  it('sourceMeta echoes an unknown source and dashes an empty one', async () => {
    const { sourceMeta } = await import('@/utils/sources')
    expect(sourceMeta('jira').label).toBe('jira')
    expect(sourceMeta('jira').icon).toBe(null)
    expect(sourceMeta(null).label).toBe('—')
    expect(sourceMeta(undefined).label).toBe('—')
  })

  it('isExternalSource is true only for a non-user provider', async () => {
    const { isExternalSource } = await import('@/utils/sources')
    expect(isExternalSource('gitlab')).toBe(true)
    expect(isExternalSource('jira')).toBe(true)
    expect(isExternalSource('user')).toBe(false)
    expect(isExternalSource('')).toBe(false)
    expect(isExternalSource(null)).toBe(false)
  })
})
