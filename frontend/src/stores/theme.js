import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { darkTheme } from 'naive-ui'
import { DARK, LIGHT, themeVars } from '@/styles/tokens'
import { users, getAccessToken } from '@/api'
import { normalizeDatePreset, DEFAULT_PREFS } from '@/utils/format'
import { sGet, sSet } from '@/utils/serverStorage'
import { detectBrowserLocale } from '@/i18n'

// Multi-color accent schemes. Default = purple. The palette is a token; the
// human name of a scheme is not, so it lives in the catalogue under
// `settings.appearance.color.<key>` and is resolved where it is drawn (#2799) —
// the same split the priority labels went through in wave 6.
export const COLOR_THEMES = [
  {
    key: 'purple',
    primary: '#7c5cff',
    hover: '#9277ff',
    pressed: '#6344e0',
    suppl: '#9277ff',
  },
  {
    key: 'blue',
    primary: '#2f80ed',
    hover: '#4f97f5',
    pressed: '#1f64c7',
    suppl: '#4f97f5',
  },
  {
    key: 'teal',
    primary: '#0eb0a9',
    hover: '#2cc1ba',
    pressed: '#07877f',
    suppl: '#2cc1ba',
  },
  {
    key: 'green',
    primary: '#18a058',
    hover: '#36ad6a',
    pressed: '#0c7a43',
    suppl: '#36ad6a',
  },
  {
    key: 'orange',
    primary: '#f0a020',
    hover: '#fcb040',
    pressed: '#c97c10',
    suppl: '#fcb040',
  },
  {
    key: 'red',
    primary: '#e0533d',
    hover: '#ea6e5a',
    pressed: '#c23c28',
    suppl: '#ea6e5a',
  },
  {
    key: 'pink',
    primary: '#eb2f96',
    hover: '#f759ab',
    pressed: '#c41d7f',
    suppl: '#f759ab',
  },
]

// Relative luminance (WCAG) → choose readable text on a primary-tinted surface.
function relLuminance(hex) {
  const c = (i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * c(1) + 0.7152 * c(3) + 0.0722 * c(5)
}
function textOnPrimary(hex) {
  return relLuminance(hex) > 0.3 ? '#1f1f1f' : '#ffffff'
}

export const useThemeStore = defineStore('theme', () => {
  // Preferences live in the DB (per user) since web 0.56 / U1b; this store is
  // the single client-side source of truth and persist point. localStorage is a
  // first-paint cache so the theme doesn't flash before /auth/me hydrates it.
  // Legacy keys (tessera_color / tessera_dark) are read once for back-compat.
  const cached = JSON.parse(sGet('tessera_prefs') || 'null') || {}
  const legacyKey = sGet('tessera_color')
  const legacyDark = sGet('tessera_dark')

  const accentKey = cached.accent || legacyKey || 'purple'
  const activeTheme = ref(COLOR_THEMES.find((t) => t.key === accentKey) ?? COLOR_THEMES[0])

  // themeMode is the persisted preference (system | light | dark); isDark is the
  // effective boolean it resolves to (system follows the OS).
  const themeMode = ref(
    cached.theme || (legacyDark === '1' ? 'dark' : legacyDark === '0' ? 'light' : 'system'),
  )
  const systemDark = ref(window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false)
  window.matchMedia?.('(prefers-color-scheme: dark)').addEventListener?.('change', (e) => {
    systemDark.value = e.matches
  })
  const isDark = computed(
    () => themeMode.value === 'dark' || (themeMode.value === 'system' && systemDark.value),
  )

  // Localizing + board-background preferences (drive date pickers / formats /
  // — later — i18n, and the board canvas background).
  // No stored choice yet (first visit, or signed out after a reset) → follow the
  // browser's language list instead of assuming Russian (#2818). A signed-in
  // user's own preference still wins: hydrate() overwrites this from the server.
  const language = ref(cached.language || detectBrowserLocale())
  const timezone = ref(cached.timezone || '')
  const country = ref(cached.country || '')
  const timeFormat = ref(cached.time_format || '24h')
  // Named preset ('short' | 'medium' | 'long' | 'iso'), not a date-fns pattern:
  // a stored pattern hard-codes a field order that contradicts the chosen
  // language (#2798). Values written by older clients — and by Android, which
  // still offers the pattern list — are mapped onto the nearest preset on read.
  const dateFormat = ref(normalizeDatePreset(cached.date_format))
  const weekStart = ref(typeof cached.week_start === 'number' ? cached.week_start : 1)
  const boardBackground = ref(cached.board_background || '')

  // Tag-scope display: 'name' shows the friendly prefix label, 'raw' shows the
  // bare prefix (short chips). Device-level UX preference (long vs short tags),
  // so it lives in localStorage only — not synced to the DB prefs. See #2604.
  const tagPrefixMode = ref(localStorage.getItem('tessera_tag_prefix_mode') || 'name')
  function setTagPrefixMode(m) {
    tagPrefixMode.value = m === 'raw' ? 'raw' : 'name'
    localStorage.setItem('tessera_tag_prefix_mode', tagPrefixMode.value)
  }

  const palette = computed(() => (isDark.value ? DARK : LIGHT))
  const primaryColor = computed(() => activeTheme.value.primary)
  const onPrimaryColor = computed(() => textOnPrimary(activeTheme.value.primary))
  const naiveTheme = computed(() => (isDark.value ? darkTheme : null))

  // Single source of truth for Naive UI component overrides.
  const themeOverrides = computed(() => {
    const p = activeTheme.value
    const onPrimary = textOnPrimary(p.primary)
    const common = {
      primaryColor: p.primary,
      primaryColorHover: p.hover,
      primaryColorPressed: p.pressed,
      primaryColorSuppl: p.suppl,
      borderRadius: '10px',
    }
    const buttonPrimaryText = {
      textColorPrimary: onPrimary,
      textColorHoverPrimary: onPrimary,
      textColorPressedPrimary: onPrimary,
      textColorFocusPrimary: onPrimary,
    }
    const radioActive = {
      buttonColorActive: p.primary,
      buttonTextColorActive: onPrimary,
      buttonBorderColorActive: p.primary,
    }

    // v2 (#2951): both themes are built from the palette — the light theme used
    // to run on Naive's defaults, which drew its own greys and lines. Overlays
    // (popovers, menus, tooltips, modals) sit on the `pop` level with v2 shadows;
    // fields are filled wells with no resting border.
    const d = palette.value
    const dark = isDark.value
    const field = '1px solid transparent'
    const fieldFocus = `1px solid ${p.primary}`
    return {
      common: {
        ...common,
        textColor1: d.text1,
        textColor2: d.text2,
        textColor3: d.text3,
        placeholderColor: d.placeholder,
        borderColor: d.border,
        dividerColor: d.border,
        cardColor: d.card,
        modalColor: d.pop,
        popoverColor: d.pop,
        bodyColor: d.bg,
        inputColor: d.inputBg,
        hoverColor: d.hover,
        boxShadow1: d.shCard,
        boxShadow2: d.shPop,
        boxShadow3: d.shDrag,
      },
      Card: { borderColor: d.border, titleTextColor: d.text1, color: d.card },
      Button: dark ? { colorQuaternary: d.surfaceAlt, ...buttonPrimaryText } : buttonPrimaryText,
      Layout: {
        color: d.bg,
        headerColor: d.bg,
        siderColor: d.sidebar,
        headerBorderColor: 'transparent',
        siderBorderColor: 'transparent',
      },
      Input: {
        color: d.inputBg,
        colorFocus: d.inputBg,
        border: field,
        borderHover: field,
        borderFocus: fieldFocus,
        placeholderColor: d.placeholder,
        textColor: d.text2,
      },
      Select: {
        peers: {
          InternalSelection: {
            color: d.inputBg,
            colorActive: d.inputBg,
            border: field,
            borderHover: field,
            borderActive: fieldFocus,
            borderFocus: fieldFocus,
            textColor: d.text2,
            placeholderColor: d.placeholder,
            arrowColor: d.text3,
          },
        },
      },
      InternalSelectMenu: {
        color: d.pop,
        optionTextColor: d.text2,
        optionTextColorActive: d.text1,
        optionColorPending: d.hover,
        optionColorActivePending: d.hover,
      },
      Popover: { color: d.pop, boxShadow: d.shPop, textColor: d.text1 },
      Tooltip: { color: d.pop, boxShadow: d.shPop, textColor: d.text1 },
      DatePicker: {
        panelColor: d.pop,
        panelBoxShadow: d.shPop,
        calendarTitleTextColor: d.text1,
        calendarDaysTextColor: d.text3,
        itemTextColor: d.text2,
        itemTextColorActive: '#fff',
        itemColorHover: d.hover,
        panelHeaderDividerColor: 'transparent',
        panelActionDividerColor: 'transparent',
        arrowColor: d.text3,
      },
      Dropdown: { color: d.pop, optionColorHover: d.hover, boxShadow: d.shPop },
      Modal: { color: d.pop, boxShadow: d.shDrag },
      Dialog: { color: d.pop, textColor: d.text2, titleTextColor: d.text1 },
      ...(dark ? { Scrollbar: { color: d.pill, colorRail: d.border } } : {}),
      Tag: { colorBorder: d.border },
      Switch: { railColorActive: p.primary },
      Divider: { color: d.border },
      Radio: { colorChecked: p.primary, borderColorChecked: p.primary, ...radioActive },
      Empty: { textColor: d.text3, iconColor: d.text3 },
    }
  })

  // The full preferences object as the server expects it (PUT replaces all).
  function snapshot() {
    return {
      language: language.value,
      timezone: timezone.value,
      country: country.value,
      time_format: timeFormat.value,
      date_format: dateFormat.value,
      week_start: weekStart.value,
      theme: themeMode.value,
      accent: activeTheme.value.key,
      board_background: boardBackground.value,
    }
  }

  // Cache locally (first-paint) and, when signed in, persist to the DB. Fire-and-
  // forget: a failed write shouldn't block the UI from reflecting the choice.
  function persist() {
    const snap = snapshot()
    sSet('tessera_prefs', JSON.stringify(snap))
    // Keep legacy keys in sync so an older cached client still first-paints right.
    sSet('tessera_color', snap.accent)
    sSet('tessera_dark', isDark.value ? '1' : '0')
    if (getAccessToken()) {
      users.updatePreferences(snap).catch(() => {})
    }
  }

  // Apply server preferences (from /auth/me or an auth response) without
  // re-persisting them.
  function hydrate(prefs) {
    if (!prefs) return
    if (prefs.accent)
      activeTheme.value = COLOR_THEMES.find((t) => t.key === prefs.accent) ?? activeTheme.value
    if (prefs.theme) themeMode.value = prefs.theme
    if (prefs.language) language.value = prefs.language
    timezone.value = prefs.timezone ?? timezone.value
    country.value = prefs.country ?? country.value
    if (prefs.time_format) timeFormat.value = prefs.time_format
    if (prefs.date_format) dateFormat.value = normalizeDatePreset(prefs.date_format)
    if (typeof prefs.week_start === 'number') weekStart.value = prefs.week_start
    boardBackground.value = prefs.board_background ?? boardBackground.value
    sSet('tessera_prefs', JSON.stringify(snapshot()))
    sSet('tessera_color', activeTheme.value.key)
    sSet('tessera_dark', isDark.value ? '1' : '0')
  }

  function selectColor(t) {
    activeTheme.value = t
    persist()
  }
  function setThemeMode(mode) {
    themeMode.value = mode
    persist()
  }
  function toggle() {
    setThemeMode(isDark.value ? 'light' : 'dark')
  }
  function setBoardBackground(v) {
    boardBackground.value = v || ''
    persist()
  }
  // Merge localizing fields (language/timezone/country/formats/week_start).
  function setLocale(partial) {
    if (partial.language !== undefined) language.value = partial.language
    if (partial.timezone !== undefined) timezone.value = partial.timezone
    if (partial.country !== undefined) country.value = partial.country
    if (partial.time_format !== undefined) timeFormat.value = partial.time_format
    if (partial.date_format !== undefined)
      dateFormat.value = normalizeDatePreset(partial.date_format)
    if (partial.week_start !== undefined) weekStart.value = partial.week_start
    persist()
  }

  // Push palette + accent into CSS custom properties so plain (non-Naive)
  // components (sidebar, board columns) follow the theme too.
  function applyCssVars() {
    const root = document.documentElement
    const vars = themeVars(palette.value, activeTheme.value.primary, onPrimaryColor.value)
    for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v)
    root.setAttribute('data-theme', isDark.value ? 'dark' : 'light')
  }

  watch([isDark, activeTheme], applyCssVars, { immediate: true })

  // Logout cleanup. The accent drops back to the brand purple (#2817): the auth
  // screens are a pre-account zone drawn in the brand colour, so a leftover
  // account accent clashes with their hard-coded purple glow. Theme MODE
  // (light/dark) stays — it is a device-level comfort setting, and resetting it
  // would flash a logging-out dark-mode user into white. The rest of the
  // account-bound preferences (localizing + board background) drop to defaults.
  // The next login re-hydrates everything from that user's server prefs, so
  // their own accent comes straight back once signed in.
  function reset() {
    activeTheme.value = COLOR_THEMES[0]
    // Back to the system guess, not a hard 'ru': after logout the user is on the
    // auth screens again, which is exactly where the browser language rules.
    language.value = detectBrowserLocale()
    timezone.value = ''
    country.value = ''
    timeFormat.value = '24h'
    dateFormat.value = DEFAULT_PREFS.dateFormat
    weekStart.value = 1
    boardBackground.value = ''
    sSet('tessera_prefs', JSON.stringify(snapshot()))
    sSet('tessera_color', activeTheme.value.key)
    sSet('tessera_dark', isDark.value ? '1' : '0')
  }

  return {
    activeTheme,
    isDark,
    themeMode,
    palette,
    primaryColor,
    onPrimaryColor,
    naiveTheme,
    themeOverrides,
    // localizing + board background
    language,
    timezone,
    country,
    timeFormat,
    dateFormat,
    weekStart,
    boardBackground,
    tagPrefixMode,
    // actions
    selectColor,
    setThemeMode,
    toggle,
    setBoardBackground,
    setTagPrefixMode,
    setLocale,
    hydrate,
    reset,
  }
})
