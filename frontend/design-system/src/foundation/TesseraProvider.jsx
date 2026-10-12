/**
 * Root wrapper for every Tessera design: picks light/dark mode, one of the seven
 * accent schemes and the UI language, publishes the matching `--t-*` CSS custom
 * properties on `:root`, and mounts the shared SVG gradient definitions that
 * accent icons and priority flags paint with. Render it once, around the whole
 * screen — components outside it fall back to light/purple/ru and gradient icons
 * lose their fill.
 * @category Foundation
 */
import { createElement, useContext, useLayoutEffect, useMemo } from 'react'
import { DARK, LIGHT, PRIORITY_COLORS, themeVars } from '@/styles/tokens'
import { COLOR_THEMES } from '@/stores/theme'
import { DEFAULT_THEME, TesseraContext, setRootState } from '../runtime/context'

function textOnPrimary(hex) {
  const c = (i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
  }
  const lum = 0.2126 * c(1) + 0.7152 * c(3) + 0.0722 * c(5)
  return lum > 0.3 ? '#1f1f1f' : '#ffffff'
}

const stop = (offset, color) =>
  createElement('stop', { key: offset, offset, style: { stopColor: color } })
const gradStops = (c) => [
  stop('0', `color-mix(in srgb, ${c} 86%, #000)`),
  stop('0.5', c),
  stop('1', `color-mix(in srgb, ${c} 86%, #fff)`),
]
const diag = { x1: '0', y1: '1', x2: '1', y2: '0' }

function GradientDefs() {
  return createElement(
    'svg',
    { width: 0, height: 0, 'aria-hidden': true, style: { position: 'absolute' } },
    createElement(
      'defs',
      null,
      createElement(
        'linearGradient',
        { id: 't-accent-grad-svg', ...diag },
        gradStops('var(--t-primary)'),
      ),
      createElement('linearGradient', { id: 't-error-grad-svg', ...diag }, [
        stop('0', '#b33030'),
        stop('0.5', '#e5484d'),
        stop('1', '#f58181'),
      ]),
      ...PRIORITY_COLORS.slice(1).map((c, i) =>
        createElement(
          'linearGradient',
          { key: i, id: `t-prio-grad-${i + 1}`, ...diag },
          gradStops(c),
        ),
      ),
    ),
  )
}

function providerVars(mode, accent) {
  const t = COLOR_THEMES.find((x) => x.key === accent) ?? COLOR_THEMES[0]
  return themeVars(mode === 'dark' ? DARK : LIGHT, t.primary, textOnPrimary(t.primary))
}

export function TesseraProvider({
  mode = DEFAULT_THEME.mode,
  accent = DEFAULT_THEME.accent,
  language = DEFAULT_THEME.language,
  children,
}) {
  // Only the outermost provider owns :root (body background, teleported
  // popovers/dropdowns). A nested one scopes its colours to its own subtree via
  // the wrapper's inline custom properties — e.g. a dark panel inside a light page.
  const outermost = !useContext(TesseraContext).provided
  const value = useMemo(
    () => ({ mode, accent, language, provided: true }),
    [mode, accent, language],
  )
  const vars = useMemo(() => providerVars(mode, accent), [mode, accent])

  useLayoutEffect(() => {
    if (!outermost) return
    setRootState({ vars, mode: mode === 'dark' ? 'dark' : 'light', language })
  }, [outermost, vars, mode, language])

  return createElement(
    TesseraContext.Provider,
    { value },
    outermost ? createElement(GradientDefs) : null,
    createElement(
      'div',
      { className: 'tds-root', 'data-theme': mode === 'dark' ? 'dark' : 'light', style: vars },
      children,
    ),
  )
}
