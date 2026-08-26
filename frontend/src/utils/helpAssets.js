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

// The English twin of a shot is `<name>-<light|dark>.en.png` next to the Russian
// original (#2816), so a language without its own shots simply has no such file
// and keeps the Russian one. Only the extension moves — the articles keep
// linking `board-light.png` and never mention a language.
const EXT_RE = /(\.[a-z0-9]+)$/i
const DEFAULT_LANG = 'ru'

function withLang(name, lang) {
  if (!lang || lang === DEFAULT_LANG) return name
  return EXT_RE.test(name) ? name.replace(EXT_RE, `.${lang}$1`) : `${name}.${lang}`
}

// Candidates in preference order. Theme beats language deliberately: a white
// screenshot on a dark page hurts to look at, a Russian screenshot in an English
// article merely reads as untranslated.
// Exported for the tests: the English shots are added in waves, so the naming
// and the preference order have to be checkable without depending on which
// `.en.png` files happen to be in the repo yet.
export function helpAssetCandidates(src, dark = false, lang = DEFAULT_LANG) {
  return candidates(String(src).split(/[?#]/)[0].split('/').pop(), dark, lang)
}

// The Set dedupes: on Russian `withLang` is the identity, so every candidate
// would otherwise be listed twice.
function candidates(name, dark, lang) {
  const out = []
  if (dark) {
    const twin = name.replace(LIGHT_RE, '-dark$1')
    if (twin !== name) out.push(withLang(twin, lang), twin)
  }
  out.push(withLang(name, lang), name)
  return [...new Set(out)]
}

export function helpAssetUrl(src, dark = false, lang = DEFAULT_LANG) {
  const name = String(src).split(/[?#]/)[0].split('/').pop()
  for (const c of candidates(name, dark, lang)) {
    if (BY_NAME.has(c)) return BY_NAME.get(c)
  }
  return ''
}

// Rewrites the src of every relative <img> in rendered article HTML. Absolute
// and protocol-relative sources (an external image) are left untouched; an
// unknown local file keeps its original src rather than becoming a broken empty
// one — the missing-asset case is caught by tests/cx-help-index.spec.js, not by
// silently blanking the picture.
export function resolveHelpImages(html, dark = false, lang = DEFAULT_LANG) {
  return html.replace(/(<img\b[^>]*?\bsrc=")([^"]+)(")/g, (whole, head, src, tail) => {
    if (/^(https?:)?\/\//.test(src) || src.startsWith('data:')) return whole
    const url = helpAssetUrl(src, dark, lang)
    return url ? head + url + tail : whole
  })
}
