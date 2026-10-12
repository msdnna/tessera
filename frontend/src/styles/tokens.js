// Theme palettes — v2 (#2951, design/tessera-v2/handoff/tokens-v2.css).
//
// v2 separates blocks by surface level, spacing and shadow instead of 1px lines,
// so the ladder below has deliberately visible steps (rail < sidebar < canvas <
// column < card < popover in the dark theme). The classic keys (bg, surface,
// surfaceAlt, inputBg, hover, border, text*) keep their meaning for the screens
// not redrawn yet; the v2 levels are new keys. Every key is published as a
// `--t-*` custom property by themeVars() below.
export const DARK = {
  bg: '#18181d', // canvas: board, topbar, page body
  surface: '#27272e', // = card
  surfaceAlt: '#1d1d23', // = column base, secondary panels
  // Background of text inputs — matches Naive's Input.color so hand-rolled
  // fields (the boxed comment composer) sit on the same surface as the rest.
  inputBg: '#212127',
  hover: '#222228',
  // v2 draws no separator lines; the hairline stays for screens not redrawn
  // yet and for the rare place a line is the only honest separator.
  border: '#2f2f37',
  text1: '#ececf1',
  text2: '#b5b5c0',
  text3: '#8b8b97',
  placeholder: '#63636e',
  // v2 surface levels
  rail: '#0b0b0e',
  sidebar: '#101014',
  col: '#1d1d23',
  card: '#27272e',
  cardHover: '#2c2c34',
  pop: '#2e2e36',
  hoverSide: '#1a1a1f',
  ctl: '#212127',
  pill: '#33333b',
  sub: '#202026',
  foot: '#17171c',
  kbd: '#3a3a43',
  scrim: 'rgba(6, 6, 9, 0.62)',
  // Column tint: share of the column colour mixed over `col`.
  colTone: '15%',
  // Active item (sidebar, segment): accent mixed over the sidebar level.
  activeFrom: '16%',
  activeTo: '26%',
  dropTone: '16%',
  shCard: '0 1px 2px rgba(0,0,0,.32), 0 2px 8px rgba(0,0,0,.16)',
  shHover: '0 2px 4px rgba(0,0,0,.36), 0 8px 20px rgba(0,0,0,.24)',
  shPop: '0 12px 36px rgba(0,0,0,.5), 0 2px 8px rgba(0,0,0,.3)',
  shDrag: '0 18px 44px rgba(0,0,0,.55), 0 4px 12px rgba(0,0,0,.35)',
  shDrop: 'inset 0 1px 3px rgba(0,0,0,.35)',
}

export const LIGHT = {
  bg: '#f8f8fa',
  surface: '#ffffff',
  surfaceAlt: '#f1f1f4',
  inputBg: '#ededf1',
  hover: '#ebebef',
  border: '#e4e4ea',
  text1: '#1c1c22',
  text2: '#4a4a55',
  text3: '#767682',
  placeholder: '#a9a9b3',
  rail: '#e2e2e8',
  sidebar: '#ededf1',
  col: '#f1f1f4',
  card: '#ffffff',
  cardHover: '#ffffff',
  pop: '#ffffff',
  hoverSide: '#e3e3e8',
  ctl: '#ededf1',
  pill: '#f1f1f5',
  sub: '#f5f5f8',
  foot: '#e5e5ea',
  kbd: '#e4e4ea',
  scrim: 'rgba(24, 24, 40, 0.26)',
  colTone: '11%',
  activeFrom: '11%',
  activeTo: '18%',
  dropTone: '12%',
  shCard: '0 1px 2px rgba(20,20,40,.06), 0 1px 4px rgba(20,20,40,.04)',
  shHover: '0 2px 6px rgba(20,20,40,.08), 0 8px 20px rgba(20,20,40,.07)',
  shPop: '0 12px 32px rgba(20,20,40,.14), 0 2px 6px rgba(20,20,40,.06)',
  shDrag: '0 18px 40px rgba(20,20,40,.18), 0 4px 12px rgba(20,20,40,.08)',
  shDrop: 'inset 0 1px 3px rgba(20,20,40,.08)',
}

// The `--t-*` custom properties for one palette + accent. Shared by the app's
// theme store and the design system's TesseraProvider, so both publish exactly
// the same set. Values that mix in the accent reference var(--t-primary): they
// resolve on the element that declares them, next to --t-primary itself.
export function themeVars(p, primary, onPrimary) {
  return {
    '--t-bg': p.bg,
    '--t-surface': p.surface,
    '--t-surface-alt': p.surfaceAlt,
    '--t-input-bg': p.inputBg,
    '--t-hover': p.hover,
    '--t-border': p.border,
    '--t-text1': p.text1,
    '--t-text2': p.text2,
    '--t-text3': p.text3,
    '--t-placeholder': p.placeholder,
    '--t-primary': primary,
    '--t-on-primary': onPrimary,
    '--t-rail': p.rail,
    '--t-sidebar': p.sidebar,
    '--t-col': p.col,
    '--t-col-tone': p.colTone,
    '--t-card': p.card,
    '--t-card-hover': p.cardHover,
    '--t-pop': p.pop,
    '--t-hover-side': p.hoverSide,
    '--t-ctl': p.ctl,
    '--t-pill': p.pill,
    '--t-sub': p.sub,
    '--t-foot': p.foot,
    '--t-kbd': p.kbd,
    '--t-scrim': p.scrim,
    '--t-active': `linear-gradient(to top right, color-mix(in srgb, var(--t-primary) ${p.activeFrom}, ${p.sidebar}), color-mix(in srgb, var(--t-primary) ${p.activeTo}, ${p.sidebar}))`,
    '--t-drop': `color-mix(in srgb, var(--t-primary) ${p.dropTone}, ${p.col})`,
    '--t-sh-card': p.shCard,
    '--t-sh-hover': p.shHover,
    '--t-sh-pop': p.shPop,
    '--t-sh-drag': p.shDrag,
    '--t-sh-drop': p.shDrop,
  }
}

// Task priority palette (index = priority 0..4). Used by board cards / pickers.
export const PRIORITY_COLORS = [
  '#9aa0aa', // 0 none
  '#5b9bd5', // 1 low
  '#3aa675', // 2 normal
  '#e0a418', // 3 high
  '#e0533d', // 4 urgent
]

// The matching labels are NOT here: a colour is a token, a string is not. They
// live in utils/priority.js, produced from the catalog per call (#2799).
