import { describe, it, expect, beforeEach } from 'vitest'
import { h } from 'vue'
import { mount } from '@vue/test-utils'
import { NConfigProvider, NEmpty } from 'naive-ui'
import ru from '@/locales/ru'
import en from '@/locales/en'
import { i18n, setI18nLocale, loadLocaleMessages, normalizeLocale, SUPPORTED_LOCALES } from '@/i18n'
import { naivePack } from '@/i18n/naive'
import { WHATS_NEW } from '@/data/whatsNew'

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

  // Key parity is blind to the values: `"save": "Сохранить"` sitting in the en
  // bundle passes every check above and then shows Russian to an English user.
  // The exceptions are text that is deliberately not translated, listed with a
  // reason — an endonym in the language picker, and two references to a board
  // column whose name is server-seeded Russian user data (pitfalls 3–4 of #2796).
  const CYRILLIC_IN_EN_ALLOWED = {
    'common.language.ru': 'endonym — the picker names each language in itself',
    'gitlab.modal.writeback.groupLabelPlaceholder':
      'sample of a real GitLab label, seeded in Russian',
    'tour.steps.task-create.body': 'quotes the «К работе» column by its actual name',
    'tour.steps.dnd-card.body': 'quotes the «К работе» column by its actual name',
  }

  it('has no Russian left in the en bundle', () => {
    const cyrillic = /[Ѐ-ӿ]/
    const leaked = leafKeys(en).filter((key) => {
      if (key in CYRILLIC_IN_EN_ALLOWED) return false
      return cyrillic.test(key.split('.').reduce((acc, part) => acc[part], en))
    })
    expect(
      leaked,
      'Untranslated keys in en/. Translate them, or — if the Russian is intentional ' +
        '(a name, user data) — add the key to CYRILLIC_IN_EN_ALLOWED with the reason.',
    ).toEqual([])
  })

  it('keeps the allowlist for untranslated en values honest', () => {
    // An allowlisted key that no longer holds Russian means the exception
    // outlived its reason and should go, before it starts covering a real leak.
    const cyrillic = /[Ѐ-ӿ]/
    const stale = Object.keys(CYRILLIC_IN_EN_ALLOWED).filter((key) => {
      const value = key.split('.').reduce((acc, part) => acc?.[part], en)
      return typeof value !== 'string' || !cyrillic.test(value)
    })
    expect(stale, 'Drop these from CYRILLIC_IN_EN_ALLOWED — they carry no Russian.').toEqual([])
  })

  // Key parity compares names, not the plural branches behind them: `ru` needs
  // three forms (1 / 2–4 / 5+, see russianPlural in src/i18n), `en` two. A key
  // pluralised in one locale and flat in the other renders the whole "a | b | c"
  // string verbatim, pipes and all, at whatever count the user happens to hit.
  it('pluralised messages carry the right number of forms in both locales', () => {
    // `{'|'}` is a literal pipe, not a branch — the markdown table in the spec
    // template and the GitLab regex placeholder are written that way (#2799).
    const forms = (s) => s.replaceAll("{'|'}", '').split('|').length
    const wrong = []
    for (const key of leafKeys(ru)) {
      const r = key.split('.').reduce((acc, part) => acc[part], ru)
      const e = key.split('.').reduce((acc, part) => acc[part], en)
      if (typeof r !== 'string' || typeof e !== 'string') continue
      if (forms(r) === 1 && forms(e) === 1) continue
      if (forms(r) < 3 || forms(e) < 2) wrong.push(`${key} (ru:${forms(r)} en:${forms(e)})`)
    }
    expect(wrong, 'Pluralised keys need 3 forms in ru and 2 in en.').toEqual([])
  })

  // A `{count}` dropped in translation is not an error anywhere — vue-i18n
  // renders the message without it, and the interface quietly loses the number.
  it('ru and en interpolate the same placeholders', () => {
    const named = /\{\s*(\w+)\s*\}/g
    // The set of names, not the list: a pluralised ru message repeats `{n}` in
    // each of its three branches while en has two, and that is not a mismatch.
    const set = (s) => [...new Set([...s.matchAll(named)].map((m) => m[1]))].sort()
    const mismatched = []
    for (const key of leafKeys(ru)) {
      const r = key.split('.').reduce((acc, part) => acc[part], ru)
      const e = key.split('.').reduce((acc, part) => acc[part], en)
      if (typeof r !== 'string' || typeof e !== 'string') continue
      const [a, b] = [set(r), set(e)]
      if (a.join() !== b.join()) mismatched.push(`${key} (ru:${a.join('|')} en:${b.join('|')})`)
    }
    expect(mismatched, 'Placeholders differ between locales.').toEqual([])
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

// The What's New entries hold catalogue keys instead of text (#2800), and the
// modal renders whatever key it is given — a typo would surface to the user as
// the raw key string. Nothing else checks these keys: they are built in a data
// file, not written in a template where eslint would see them.
describe('What’s New release notes', () => {
  it('every key a release entry points at exists in both locales', () => {
    const keys = WHATS_NEW.flatMap((rel) => [
      rel.titleKey,
      ...rel.itemKeys,
      ...(rel.spotlight ? [rel.spotlight.titleKey, rel.spotlight.bodyKey] : []),
    ])
    const resolve = (bundle, key) => key.split('.').reduce((acc, part) => acc?.[part], bundle)
    for (const [name, bundle] of [
      ['ru', ru],
      ['en', en],
    ]) {
      const missing = keys.filter((k) => typeof resolve(bundle, k) !== 'string')
      expect(missing, `${name} is missing release-note keys`).toEqual([])
    }
  })

  it('leaves no orphan release notes in the catalogue', () => {
    // A release dropped from the list but left in the catalogue is dead weight
    // that still has to be kept translated.
    const used = new Set(
      WHATS_NEW.flatMap((rel) => [
        rel.titleKey,
        ...rel.itemKeys,
        ...(rel.spotlight ? [rel.spotlight.titleKey, rel.spotlight.bodyKey] : []),
      ]),
    )
    const orphans = leafKeys(ru.whatsNew).filter((k) => !used.has(`whatsNew.${k}`))
    expect(orphans).toEqual([])
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
