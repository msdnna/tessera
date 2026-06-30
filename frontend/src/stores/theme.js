import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { darkTheme } from 'naive-ui'
import { DARK, LIGHT } from '@/styles/tokens'
import { users } from '@/api'

// Multi-color accent schemes (a reference tracker-style). Default = purple.
export const COLOR_THEMES = [
  {
    name: 'Фиолетовый',
    key: 'purple',
    primary: '#7c5cff',
    hover: '#9277ff',
    pressed: '#6344e0',
    suppl: '#9277ff',
  },
  {
    name: 'Синий',
    key: 'blue',
    primary: '#2f80ed',
    hover: '#4f97f5',
    pressed: '#1f64c7',
    suppl: '#4f97f5',
  },
  {
    name: 'Бирюзовый',
    key: 'teal',
    primary: '#0eb0a9',
    hover: '#2cc1ba',
    pressed: '#07877f',
    suppl: '#2cc1ba',
  },
  {
    name: 'Зелёный',
    key: 'green',
    primary: '#18a058',
    hover: '#36ad6a',
    pressed: '#0c7a43',
    suppl: '#36ad6a',
  },
  {
    name: 'Оранжевый',
    key: 'orange',
    primary: '#f0a020',
    hover: '#fcb040',
    pressed: '#c97c10',
    suppl: '#fcb040',
  },
  {
    name: 'Красный',
    key: 'red',
    primary: '#e0533d',
    hover: '#ea6e5a',
    pressed: '#c23c28',
    suppl: '#ea6e5a',
  },
  {
    name: 'Розовый',
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
  const cached = JSON.parse(localStorage.getItem('tessera_prefs') || 'null') || {}
  const legacyKey = localStorage.getItem('tessera_color')
  const legacyDark = localStorage.getItem('tessera_dark')

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
  const language = ref(cached.language || 'ru')
  const timezone = ref(cached.timezone || '')
  const country = ref(cached.country || '')
  const timeFormat = ref(cached.time_format || '24h')
  const dateFormat = ref(cached.date_format || 'dd.MM.yyyy')
  const weekStart = ref(typeof cached.week_start === 'number' ? cached.week_start : 1)
  const boardBackground = ref(cached.board_background || '')

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
      borderRadius: '8px',
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

    if (!isDark.value) {
      return { common, Button: buttonPrimaryText, Radio: radioActive }
    }

    const d = DARK
    return {
      common: {
        ...common,
        textColor1: d.text1,
        textColor2: d.text2,
        textColor3: d.text3,
        textColorPlaceholder: d.placeholder,
        borderColor: d.border,
        cardBorderColor: d.border,
      },
      Card: { borderColor: d.border, titleTextColor: d.text1, color: d.surface },
      Button: { colorQuaternary: d.surfaceAlt, ...buttonPrimaryText },
      Layout: { color: d.bg, colorHeader: d.surface, borderColor: d.border },
      Input: {
        color: d.surfaceAlt,
        colorFocus: d.surfaceAlt,
        borderColor: d.border,
        borderColorFocus: p.primary,
        placeholderColor: d.placeholder,
        textColor: d.text2,
      },
      Select: {
        peers: {
          InternalSelection: {
            color: d.surfaceAlt,
            colorActive: d.surfaceAlt,
            border: `1px solid ${d.border}`,
            borderHover: `1px solid ${p.primary}`,
            borderActive: `1px solid ${p.primary}`,
            borderFocus: `1px solid ${p.primary}`,
            textColor: d.text2,
            placeholderColor: d.placeholder,
            arrowColor: d.text3,
          },
        },
      },
      InternalSelectMenu: {
        color: d.surfaceAlt,
        optionTextColor: d.text2,
        optionTextColorActive: d.text1,
        optionColorPending: d.hover,
        optionColorActivePending: d.hover,
      },
      Popover: { color: d.surfaceAlt },
      Tooltip: { color: d.surfaceAlt },
      DatePicker: {
        panelColor: d.surfaceAlt,
        panelBoxShadow: '0 6px 24px rgba(0,0,0,0.5)',
        calendarTitleTextColor: d.text1,
        calendarDaysTextColor: d.text3,
        itemTextColor: d.text2,
        itemTextColorActive: '#fff',
        itemColorHover: d.hover,
        panelHeaderDividerColor: d.border,
        panelActionDividerColor: d.border,
        arrowColor: d.text3,
      },
      Dropdown: { color: d.surfaceAlt, optionColorHover: d.hover },
      Modal: { color: d.surface, boxShadow: '0 8px 32px rgba(0,0,0,0.6)' },
      Dialog: { color: d.surface, textColor: d.text2, titleTextColor: d.text1 },
      Scrollbar: { color: d.surfaceAlt, colorRail: d.border },
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
    localStorage.setItem('tessera_prefs', JSON.stringify(snap))
    // Keep legacy keys in sync so an older cached client still first-paints right.
    localStorage.setItem('tessera_color', snap.accent)
    localStorage.setItem('tessera_dark', isDark.value ? '1' : '0')
    if (localStorage.getItem('tessera_token')) {
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
    if (prefs.date_format) dateFormat.value = prefs.date_format
    if (typeof prefs.week_start === 'number') weekStart.value = prefs.week_start
    boardBackground.value = prefs.board_background ?? boardBackground.value
    localStorage.setItem('tessera_prefs', JSON.stringify(snapshot()))
    localStorage.setItem('tessera_color', activeTheme.value.key)
    localStorage.setItem('tessera_dark', isDark.value ? '1' : '0')
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
    if (partial.date_format !== undefined) dateFormat.value = partial.date_format
    if (partial.week_start !== undefined) weekStart.value = partial.week_start
    persist()
  }

  // Push palette + accent into CSS custom properties so plain (non-Naive)
  // components (sidebar, board columns) follow the theme too.
  function applyCssVars() {
    const root = document.documentElement
    const p = palette.value
    root.style.setProperty('--t-bg', p.bg)
    root.style.setProperty('--t-surface', p.surface)
    root.style.setProperty('--t-surface-alt', p.surfaceAlt)
    root.style.setProperty('--t-hover', p.hover)
    root.style.setProperty('--t-border', p.border)
    root.style.setProperty('--t-text1', p.text1)
    root.style.setProperty('--t-text2', p.text2)
    root.style.setProperty('--t-text3', p.text3)
    root.style.setProperty('--t-primary', activeTheme.value.primary)
    root.style.setProperty('--t-on-primary', onPrimaryColor.value)
    root.setAttribute('data-theme', isDark.value ? 'dark' : 'light')
  }

  watch([isDark, activeTheme], applyCssVars, { immediate: true })

  // Logout cleanup. Appearance (theme mode + accent) is a device-level preference:
  // we KEEP it so the auth screens and any reload hold the look the user last chose
  // — and re-persist it to localStorage. Only the account-bound preferences
  // (localizing + board background) drop back to defaults. The next login
  // re-hydrates everything from that user's server prefs (so a different account's
  // saved appearance still wins once signed in).
  function reset() {
    language.value = 'ru'
    timezone.value = ''
    country.value = ''
    timeFormat.value = '24h'
    dateFormat.value = 'dd.MM.yyyy'
    weekStart.value = 1
    boardBackground.value = ''
    localStorage.setItem('tessera_prefs', JSON.stringify(snapshot()))
    localStorage.setItem('tessera_color', activeTheme.value.key)
    localStorage.setItem('tessera_dark', isDark.value ? '1' : '0')
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
    // actions
    selectColor,
    setThemeMode,
    toggle,
    setBoardBackground,
    setLocale,
    hydrate,
    reset,
  }
})
