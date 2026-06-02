import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { darkTheme } from 'naive-ui'
import { DARK, LIGHT } from '@/styles/tokens'

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
  const savedKey = localStorage.getItem('tessera_color') || 'purple'
  const activeTheme = ref(COLOR_THEMES.find((t) => t.key === savedKey) ?? COLOR_THEMES[0])
  const isDark = ref(localStorage.getItem('tessera_dark') === '1')

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

  function selectColor(t) {
    activeTheme.value = t
    localStorage.setItem('tessera_color', t.key)
  }
  function toggle() {
    isDark.value = !isDark.value
    localStorage.setItem('tessera_dark', isDark.value ? '1' : '0')
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

  return {
    activeTheme,
    isDark,
    palette,
    primaryColor,
    onPrimaryColor,
    naiveTheme,
    themeOverrides,
    selectColor,
    toggle,
  }
})
