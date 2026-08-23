// vue-i18n scaffolding (#2797, stage 1 of #2796).
//
// Keys are semantic (`task.due.overdue`), never the Russian text itself, so a
// wording change never renames a key. Messages live in src/locales/<locale>/
// split by feature namespace.
//
// `ru` is bundled eagerly (default + fallback, needed for the first paint);
// every other locale is fetched by dynamic import() on demand.
import { createI18n } from 'vue-i18n'
import ru from '@/locales/ru'

export const FALLBACK_LOCALE = 'ru'
export const SUPPORTED_LOCALES = ['ru', 'en']

// Lazy bundles. One entry per non-default locale; the import() literal has to
// stay inline for Vite to see it and emit a chunk.
const loaders = {
  en: () => import('@/locales/en'),
}

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: FALLBACK_LOCALE,
  fallbackLocale: FALLBACK_LOCALE,
  messages: { [FALLBACK_LOCALE]: ru },
})

export function normalizeLocale(locale) {
  return SUPPORTED_LOCALES.includes(locale) ? locale : FALLBACK_LOCALE
}

// Fetches a locale bundle unless it is already registered. Returns the locale
// actually available afterwards — a failed chunk fetch (offline, stale deploy)
// degrades to the fallback instead of leaving the UI without messages.
export async function loadLocaleMessages(locale) {
  const target = normalizeLocale(locale)
  if (i18n.global.availableLocales.includes(target)) return target
  const loader = loaders[target]
  if (!loader) return FALLBACK_LOCALE
  try {
    const mod = await loader()
    i18n.global.setLocaleMessage(target, mod.default ?? mod)
    return target
  } catch (e) {
    console.warn('[i18n] failed to load locale', target, e)
    return FALLBACK_LOCALE
  }
}

// Single entry point for switching the UI language: loads the bundle, flips
// vue-i18n and keeps <html lang> honest (screen readers, hyphenation, :lang()).
export async function setI18nLocale(locale) {
  const active = await loadLocaleMessages(locale)
  i18n.global.locale.value = active
  document.documentElement.setAttribute('lang', active)
  return active
}

export default i18n
