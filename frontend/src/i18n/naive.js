// Naive UI's own locale packs (date pickers, pagination, empty states, …).
// They are separate from our message bundles, so switching the app language
// has to move both — this maps one to the other in a single place, keeping
// App.vue's n-config-provider honest and unit-testable (#2797).
import { ruRU, dateRuRU, enUS, dateEnUS } from 'naive-ui'
import { normalizeLocale } from '@/i18n'

const PACKS = {
  ru: { locale: ruRU, dateLocale: dateRuRU },
  en: { locale: enUS, dateLocale: dateEnUS },
}

export function naivePack(language) {
  return PACKS[normalizeLocale(language)]
}
