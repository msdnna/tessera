// Same-hue accent gradients — the web port of the Android `accentGradient`:
// a soft diagonal of one hue running darker bottom-left → base colour at the
// centre → lighter top-right (so the element's middle keeps the exact base
// colour). Strength = 14% toward black/white at the corners. Used for any
// accent/priority/tag-coloured surface; pass the element's base colour.
const dark = (c) => `color-mix(in srgb, ${c || '#888'} 86%, #000)`
const light = (c) => `color-mix(in srgb, ${c || '#888'} 86%, #fff)`

// Diagonal fill/foreground gradient of a hue.
export function hueGrad(c) {
  return `linear-gradient(to top right, ${dark(c)}, ${c || '#888'} 50%, ${light(c)})`
}

// Vertical variant (bottom darker → top lighter) for tall, narrow accents
// (a card's left priority bar).
export function hueGradVert(c) {
  return `linear-gradient(to top, ${dark(c)}, ${c || '#888'} 50%, ${light(c)})`
}

// A soft opaque tint of a hue — a pill/chip interior that a gradient border can
// sit on (replaces the old translucent overlay).
export function softFill(c) {
  return `color-mix(in srgb, ${c || '#888'} 13%, var(--t-surface))`
}

// Pill/chip background with a same-hue gradient *border* that keeps the corner
// radius: a flat interior on the padding-box, the gradient on the border-box.
// Pair with `border: 1px solid transparent`. `filled` adds the soft interior
// tint (else transparent). Per Android: tags carry the gradient on border+text,
// the fill itself is left subtle/untouched.
export function tagPillBg(c, filled = true) {
  const fill = filled ? softFill(c) : 'transparent'
  return `linear-gradient(${fill}, ${fill}) padding-box, ${hueGrad(c)} border-box`
}

// ── readable text colour ───────────────────────────────────────────────
// A tag/label colour pulled from GitLab can be too dark for a dark theme or too
// light for a light theme, killing contrast when used as *text*. readableHue
// clamps the colour's lightness into a legible band for the active theme,
// keeping its hue/saturation, so `hueGrad(readableHue(c, dark))` stays on-brand
// but readable on either background.
function hexToRgb(hex) {
  let h = String(hex || '')
    .replace('#', '')
    .trim()
  if (h.length === 3)
    h = h
      .split('')
      .map((c) => c + c)
      .join('')
  if (h.length !== 6 || /[^0-9a-fA-F]/.test(h)) return null
  const n = parseInt(h, 16)
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 }
}
function rgbToHsl({ r, g, b }) {
  r /= 255
  g /= 255
  b /= 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const d = max - min
  const l = (max + min) / 2
  let h = 0
  let s = 0
  if (d) {
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    if (max === r) h = (g - b) / d + (g < b ? 6 : 0)
    else if (max === g) h = (b - r) / d + 2
    else h = (r - g) / d + 4
    h /= 6
  }
  return { h: h * 360, s, l }
}
function hslToHex({ h, s, l }) {
  h /= 360
  const hue = (p, q, t) => {
    if (t < 0) t += 1
    if (t > 1) t -= 1
    if (t < 1 / 6) return p + (q - p) * 6 * t
    if (t < 1 / 2) return q
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6
    return p
  }
  let r
  let g
  let b
  if (!s) {
    r = g = b = l
  } else {
    const q = l < 0.5 ? l * (1 + s) : l + s - l * s
    const p = 2 * l - q
    r = hue(p, q, h + 1 / 3)
    g = hue(p, q, h)
    b = hue(p, q, h - 1 / 3)
  }
  const to = (x) =>
    Math.round(x * 255)
      .toString(16)
      .padStart(2, '0')
  return `#${to(r)}${to(g)}${to(b)}`
}
export function readableHue(hex, isDark) {
  const rgb = hexToRgb(hex)
  if (!rgb) return hex || '#888'
  const hsl = rgbToHsl(rgb)
  hsl.l = isDark ? Math.max(hsl.l, 0.62) : Math.min(hsl.l, 0.42)
  return hslToHex(hsl)
}

// Readable text/icon colour to lay *on top of* a solid fill of `hex` (a selected
// tag chip, a swatch). Picks near-black or white by the fill's relative luminance,
// so a label on a solid bright tag (e.g. yellow) stays legible. Mirrors Android's
// `onPrimary`.
export function onColor(hex) {
  const rgb = hexToRgb(hex)
  if (!rgb) return '#fff'
  const chan = (v) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  const lum = 0.2126 * chan(rgb.r) + 0.7152 * chan(rgb.g) + 0.0722 * chan(rgb.b)
  return lum > 0.45 ? '#1f1f1f' : '#ffffff'
}

// Colour swatch fill — gradient for a real colour, a flat neutral for "default".
// Always an *image* (a flat colour is expressed as a 2-stop gradient) so callers
// can assign it to `background-image` (not the `background` shorthand, which
// would reset background-origin to padding-box and make the gradient repeat in a
// transparent border ring — a visible seam inside a round swatch).
export function swatchBg(c) {
  return c ? hueGrad(c) : 'linear-gradient(var(--t-border), var(--t-border))'
}
