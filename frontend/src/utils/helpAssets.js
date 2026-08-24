// Resolves the screenshots referenced by help articles (#2793).
//
// The articles are loaded as `?raw` strings, so Vite never sees their image
// links: `![Доска](../assets/board-light.png)` would reach the browser verbatim
// and resolve against /help/<slug>, i.e. 404 in dev and in the bundle alike. The
// asset files are imported here as URLs instead, and the renderer swaps each
// relative src for the built one — hashed filename, correct base path, and the
// images get emitted into dist/ because something in src/ references them.
//
// The glob is eager: it produces one URL string per screenshot, nothing is
// downloaded until an <img> is actually rendered.
const URLS = import.meta.glob('../../../docs/help/assets/*', {
  eager: true,
  query: '?url',
  import: 'default',
})

// Keyed by file name — articles live at different depths under docs/help, so
// their relative paths differ ("../assets/x.png" vs "../../assets/x.png") while
// the target is the same single assets directory.
const BY_NAME = new Map(
  Object.entries(URLS).map(([path, url]) => [path.slice(path.lastIndexOf('/') + 1), url]),
)

// A light shot is written as `<name>-light.png` and its dark twin as
// `<name>-dark.png` (see e2e/shots/help-shots.spec.js). Articles link the light
// one; a reader in the dark theme gets the dark twin automatically, so the
// manual never shows a blinding white board on a dark page. Articles with only
// one variant (a diagram, say) are left alone.
const LIGHT_RE = /-light(\.[a-z0-9]+)$/i

export function helpAssetUrl(src, dark = false) {
  const name = String(src).split(/[?#]/)[0].split('/').pop()
  if (dark) {
    const twin = name.replace(LIGHT_RE, '-dark$1')
    if (twin !== name && BY_NAME.has(twin)) return BY_NAME.get(twin)
  }
  return BY_NAME.get(name) || ''
}

// Rewrites the src of every relative <img> in rendered article HTML. Absolute
// and protocol-relative sources (an external image) are left untouched; an
// unknown local file keeps its original src rather than becoming a broken empty
// one — the missing-asset case is caught by tests/cx-help-index.spec.js, not by
// silently blanking the picture.
export function resolveHelpImages(html, dark = false) {
  return html.replace(/(<img\b[^>]*?\bsrc=")([^"]+)(")/g, (whole, head, src, tail) => {
    if (/^(https?:)?\/\//.test(src) || src.startsWith('data:')) return whole
    const url = helpAssetUrl(src, dark)
    return url ? head + url + tail : whole
  })
}
