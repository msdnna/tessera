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

// Colour swatch fill — gradient for a real colour, a flat neutral for "default".
// Always an *image* (a flat colour is expressed as a 2-stop gradient) so callers
// can assign it to `background-image` (not the `background` shorthand, which
// would reset background-origin to padding-box and make the gradient repeat in a
// transparent border ring — a visible seam inside a round swatch).
export function swatchBg(c) {
  return c ? hueGrad(c) : 'linear-gradient(var(--t-border), var(--t-border))'
}
