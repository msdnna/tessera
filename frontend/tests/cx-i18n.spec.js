import { describe, it, expect, beforeEach } from 'vitest'
import { h } from 'vue'
import { mount } from '@vue/test-utils'
import { NConfigProvider, NEmpty } from 'naive-ui'
import ru from '@/locales/ru'
import en from '@/locales/en'
import { i18n, setI18nLocale, loadLocaleMessages, normalizeLocale, SUPPORTED_LOCALES } from '@/i18n'
import { naivePack } from '@/i18n/naive'

// Flattens a message bundle to dotted leaf paths, so a missing *or* extra key
// shows up by name instead of as an opaque object diff.
function leafKeys(obj, prefix = '') {
  return Object.entries(obj).flatMap(([k, v]) => {
    const path = prefix ? `${prefix}.${k}` : k
    return v && typeof v === 'object' && !Array.isArray(v) ? leafKeys(v, path) : [path]
  })
}

describe('locale bundles', () => {
  it('ru and en carry exactly the same keys', () => {
    // Without this gate a key missing from en silently renders Russian text
    // through the fallback, and nobody notices until a user does.
    const r = leafKeys(ru).sort()
    const e = leafKeys(en).sort()
    expect(e.filter((k) => !r.includes(k))).toEqual([])
    expect(r.filter((k) => !e.includes(k))).toEqual([])
  })

  it('every namespace exists in both locales', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(ru).sort())
  })

  it('leaf values are non-empty strings', () => {
    for (const [locale, bundle] of [
      ['ru', ru],
      ['en', en],
    ]) {
      for (const key of leafKeys(bundle)) {
        const value = key.split('.').reduce((acc, part) => acc[part], bundle)
        expect(typeof value, `${locale}:${key}`).toBe('string')
        expect(value.trim(), `${locale}:${key}`).not.toBe('')
      }
    }
  })

  // vue-i18n compiles a message the first time something renders it, so a
  // message with `@` or `|` in it stays silent through build, lint and every
  // spec that does not happen to show that exact string — and then throws
  // `SyntaxError` mid-render in front of a user, leaving a blank hole where the
  // element should be. That is how `you@example.com` took the email field off
  // the login and register forms (#2799): `@` opens a linked-key reference and
  // `|` separates plural forms, so both have to be written as `{'@'}` / `{'|'}`.
  // Compiling every message here is the only place that check is cheap.
  it('every message compiles', async () => {
    for (const locale of ['ru', 'en']) {
      await setI18nLocale(locale)
      for (const key of leafKeys(locale === 'ru' ? ru : en)) {
        expect(
          () => i18n.global.t(key),
          `${locale}:${key} does not compile — escape @ as {'@'} and | as {'|'}`,
        ).not.toThrow()
      }
    }
    await setI18nLocale('ru')
  })
})

describe('i18n runtime', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('starts on ru with ru as the fallback', () => {
    expect(i18n.global.locale.value).toBe('ru')
    expect(i18n.global.fallbackLocale.value).toBe('ru')
    expect(SUPPORTED_LOCALES).toContain('en')
  })

  it('normalizes unknown locales to the fallback', () => {
    expect(normalizeLocale('de')).toBe('ru')
    expect(normalizeLocale(undefined)).toBe('ru')
    expect(normalizeLocale('en')).toBe('en')
  })

  it('loads the en bundle on demand and switches to it', async () => {
    const active = await setI18nLocale('en')
    expect(active).toBe('en')
    expect(i18n.global.locale.value).toBe('en')
    expect(i18n.global.availableLocales).toContain('en')
    expect(i18n.global.t('common.action.save')).toBe('Save')
    expect(document.documentElement.getAttribute('lang')).toBe('en')
  })

  it('falls back to ru for an unsupported locale', async () => {
    const active = await setI18nLocale('de')
    expect(active).toBe('ru')
    expect(i18n.global.t('common.action.save')).toBe('Сохранить')
    expect(document.documentElement.getAttribute('lang')).toBe('ru')
  })

  // Naive UI ships its own locale packs; App.vue feeds n-config-provider from
  // naivePack(), so the app language has to move the component texts too.
  it('maps the app language onto a Naive UI locale pack', () => {
    expect(naivePack('ru').locale.name).toBe('ru-RU')
    expect(naivePack('en').locale.name).toBe('en-US')
    expect(naivePack('de').locale.name).toBe('ru-RU')
  })

  it('renders Naive UI components in the mapped language', () => {
    const render = (language) =>
      mount(NConfigProvider, {
        props: { locale: naivePack(language).locale, dateLocale: naivePack(language).dateLocale },
        slots: { default: () => h(NEmpty) },
      }).text()

    expect(render('ru')).toContain('Нет данных')
    expect(render('en')).toContain('No Data')
  })

  // The global setup file (tests/setup.js) installs i18n into every mount, so
  // from wave 1 of #2799 on, a component may call $t without its spec knowing
  // anything about localisation. If that registration ever falls out of the
  // vitest config, this is the test that says so — instead of ~40 spec files
  // failing at once with "$t is not a function".
  it('is installed into every mount by the global test setup', () => {
    const w = mount({ template: `<i>{{ $t('common.action.save') }}</i>` })
    expect(w.text()).toBe('Сохранить')
  })

  it('loadLocaleMessages is idempotent', async () => {
    await loadLocaleMessages('en')
    const before = i18n.global.getLocaleMessage('en')
    await loadLocaleMessages('en')
    expect(i18n.global.getLocaleMessage('en')).toBe(before)
  })
})
