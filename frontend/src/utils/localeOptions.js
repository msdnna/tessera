// Option lists for the settings localization selects, built from the native Intl
// data (no bundled dataset). Timezones come from Intl.supportedValuesOf; country
// names are localized via Intl.DisplayNames over the ISO 3166-1 alpha-2 space.
import { i18n } from '@/i18n'

export function timezoneOptions() {
  let zones
  try {
    zones = Intl.supportedValuesOf('timeZone')
  } catch {
    zones = ['UTC', 'Europe/Moscow', 'Europe/London', 'America/New_York', 'Asia/Tokyo']
  }
  return zones.map((z) => ({ label: z, value: z }))
}

export function countryOptions(locale = 'ru') {
  let dn
  try {
    dn = new Intl.DisplayNames([locale], { type: 'region' })
  } catch {
    dn = null
  }
  // No Intl.DisplayNames (very old engine): a single-entry list, named from the
  // catalogue rather than by a literal — the select is the only thing the user
  // sees here, and a bare "RU" would read as a bug.
  if (!dn) return [{ label: i18n.global.t('settings.localization.countryFallback'), value: 'RU' }]
  const A = 'A'.charCodeAt(0)
  const out = []
  for (let i = 0; i < 26; i++) {
    for (let j = 0; j < 26; j++) {
      const code = String.fromCharCode(A + i) + String.fromCharCode(A + j)
      let name
      try {
        name = dn.of(code)
      } catch {
        name = null
      }
      // Engines return the code unchanged for unassigned regions — drop those.
      if (name && name !== code) out.push({ label: name, value: code })
    }
  }
  out.sort((a, b) => a.label.localeCompare(b.label, locale))
  return out
}
