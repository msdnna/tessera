import { DARK } from '@/styles/tokens'

// Making a colour that came from Word readable on the dark sheet (#2755).
//
// An imported document keeps the colours the author chose — that is what makes
// it look like the original, and stripping them would lose information the
// document actually carries. But Word documents are written against white
// paper, so a heading in #1f4e79 lands on the dark sheet at a contrast ratio of
// about 1.9:1 and is effectively unreadable.
//
// The stored value is therefore never touched: the colour in the document JSON
// stays exactly as the file had it, and only the *rendered* colour is lifted,
// per theme, by the mark's renderHTML (see DocColor in docSchema.js). Baking
// the adjustment into the document instead would make a light-theme user
// inherit the dark-theme correction, and it would survive an export back to
// .docx as a colour the author never picked.
//
// The lift keeps hue and saturation and moves lightness only, so "dark blue
// heading" stays a blue heading rather than turning into the theme's ink.

// WCAG AA for body text. Text imported from a document is read, not glanced at,
// so the lower 3:1 "large text" bar is not the right target here.
const MIN_RATIO = 4.5

// Ceiling on the lift. A colour pushed all the way to L=1 is white, which is
// exactly the "everything became theme-coloured" outcome this is meant to
// avoid; stopping short keeps a trace of the original hue even when the source
// colour is nearly black.
const MAX_LIGHTNESS = 0.94

const NAMED = { black: [0, 0, 0], white: [255, 255, 255], red: [255, 0, 0] }

/**
 * Parses a CSS colour into RGB channels.
 *
 * Covers the notations LibreOffice and the editor's own picker produce (#rgb,
 * #rrggbb, rgb()/rgba(), and the handful of keywords that turn up in exported
 * HTML). Anything else returns null and is then left alone rather than guessed
 * at — a colour we cannot read is not a colour we should rewrite.
 *
 * @param {string} value CSS colour
 * @returns {number[]|null} [r, g, b] in 0..255
 */
export function parseCssColor(value) {
  const css = String(value || '')
    .trim()
    .toLowerCase()
  if (!css) return null
  if (NAMED[css]) return [...NAMED[css]]
  const hex = css.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/)
  if (hex) {
    const h = hex[1]
    const wide =
      h.length === 3 ? [...h].map((c) => c + c) : [h.slice(0, 2), h.slice(2, 4), h.slice(4, 6)]
    return wide.map((p) => parseInt(p, 16))
  }
  const fn = css.match(/^rgba?\(([^)]+)\)$/)
  if (fn) {
    const parts = fn[1]
      .split(/[\s,/]+/)
      .filter(Boolean)
      .slice(0, 3)
    if (parts.length < 3) return null
    const rgb = parts.map((p) =>
      p.endsWith('%') ? Math.round((parseFloat(p) * 255) / 100) : Math.round(parseFloat(p)),
    )
    return rgb.some((n) => Number.isNaN(n)) ? null : rgb.map((n) => Math.min(255, Math.max(0, n)))
  }
  return null
}

/**
 * WCAG relative luminance of an sRGB colour.
 * @param {number[]} rgb [r, g, b] in 0..255
 * @returns {number} 0..1
 */
export function relativeLuminance([r, g, b]) {
  const lin = (c) => {
    const s = c / 255
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

/**
 * WCAG contrast ratio between two colours.
 * @param {number[]} a [r, g, b]
 * @param {number[]} b [r, g, b]
 * @returns {number} 1..21
 */
export function contrastRatio(a, b) {
  const la = relativeLuminance(a)
  const lb = relativeLuminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

function rgbToHsl([r, g, b]) {
  const rr = r / 255
  const gg = g / 255
  const bb = b / 255
  const max = Math.max(rr, gg, bb)
  const min = Math.min(rr, gg, bb)
  const l = (max + min) / 2
  const d = max - min
  if (!d) return [0, 0, l]
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  let h
  if (max === rr) h = ((gg - bb) / d + (gg < bb ? 6 : 0)) / 6
  else if (max === gg) h = ((bb - rr) / d + 2) / 6
  else h = ((rr - gg) / d + 4) / 6
  return [h, s, l]
}

function hslToRgb([h, s, l]) {
  if (!s) {
    const v = Math.round(l * 255)
    return [v, v, v]
  }
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s
  const p = 2 * l - q
  const channel = (t) => {
    let tt = t
    if (tt < 0) tt += 1
    if (tt > 1) tt -= 1
    if (tt < 1 / 6) return p + (q - p) * 6 * tt
    if (tt < 1 / 2) return q
    if (tt < 2 / 3) return p + (q - p) * (2 / 3 - tt) * 6
    return p
  }
  return [channel(h + 1 / 3), channel(h), channel(h - 1 / 3)].map((c) => Math.round(c * 255))
}

function toHex(rgb) {
  return '#' + rgb.map((c) => c.toString(16).padStart(2, '0')).join('')
}

/**
 * Moves a colour's lightness until it reads against the given background.
 *
 * The input is returned untouched when it already passes or cannot be parsed,
 * so this is safe to call on every colour in a document: the common case (a
 * palette that was already chosen for this theme) costs one parse and changes
 * nothing.
 *
 * @param {string} color CSS colour from the document
 * @param {string} background CSS colour of the surface it is drawn on
 * @param {number} [minRatio] contrast to reach
 * @returns {string} a CSS colour — the original, or a lightness-shifted hex
 */
export function readableInk(color, background, minRatio = MIN_RATIO) {
  const rgb = parseCssColor(color)
  const bg = parseCssColor(background)
  if (!rgb || !bg) return color
  if (contrastRatio(rgb, bg) >= minRatio) return color

  // Which way to move depends on the surface, not on the colour: on the dark
  // sheet every failing colour has to get lighter, on paper darker. Stepping
  // by 1% keeps the result close to the original — a binary search would land
  // on a colour further from the author's than it needs to be.
  const [h, s, l0] = rgbToHsl(rgb)
  const up = relativeLuminance(bg) < 0.5
  const limit = up ? MAX_LIGHTNESS : 1 - MAX_LIGHTNESS
  let best = rgb
  for (let step = 1; step <= 100; step += 1) {
    const l = up ? Math.min(limit, l0 + step / 100) : Math.max(limit, l0 - step / 100)
    best = hslToRgb([h, s, l])
    if (contrastRatio(best, bg) >= minRatio) break
    if (l === limit) break
  }
  return toHex(best)
}

/**
 * The colour to paint on the dark document sheet.
 * @param {string} color CSS colour stored in the document
 * @returns {string} CSS colour
 */
export function darkSheetInk(color) {
  return readableInk(color, DARK.surface)
}
