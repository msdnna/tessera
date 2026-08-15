import { marked } from 'marked'
import { markedHighlight } from 'marked-highlight'
import DOMPurify from 'dompurify'
import { isTauri, serverBase } from '@/utils/serverBase'
import hljs from 'highlight.js/lib/core'
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import json from 'highlight.js/lib/languages/json'
import yaml from 'highlight.js/lib/languages/yaml'
import python from 'highlight.js/lib/languages/python'
import bash from 'highlight.js/lib/languages/bash'
import go from 'highlight.js/lib/languages/go'
import sql from 'highlight.js/lib/languages/sql'
import xml from 'highlight.js/lib/languages/xml'
import css from 'highlight.js/lib/languages/css'
import markdown from 'highlight.js/lib/languages/markdown'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import ini from 'highlight.js/lib/languages/ini'

// Register a focused set of languages (keeps the bundle small vs the full set);
// each registration also wires its aliases (js, ts, py, sh, yml, html, …).
for (const [name, lang] of Object.entries({
  javascript,
  typescript,
  json,
  yaml,
  python,
  bash,
  go,
  sql,
  xml,
  css,
  markdown,
  dockerfile,
  ini,
})) {
  hljs.registerLanguage(name, lang)
}

function escapeHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

marked.setOptions({ breaks: true, gfm: true })
// Syntax-highlight fenced code blocks via highlight.js. `mermaid` is left as
// (escaped) plain text so RichContent can turn it into a diagram.
marked.use(
  markedHighlight({
    langPrefix: 'hljs language-',
    highlight(code, lang) {
      if (lang === 'mermaid') return escapeHtml(code)
      return hljs.getLanguage(lang)
        ? hljs.highlight(code, { language: lang }).value
        : escapeHtml(code)
    },
  }),
)

// Attributes the rich editor emits that DOMPurify must preserve: link targets
// and mention chips (`<span data-type="mention" data-id="…">@Name</span>`).
const SANITIZE_OPTS = {
  ADD_ATTR: ['target', 'rel', 'data-type', 'data-id', 'data-label', 'class'],
}

// On desktop (Tauri) the webview is served from a custom protocol, so the
// backend's root-relative '/api/…' media URLs (GitLab avatar/asset proxies,
// rewritten attachment links) would resolve against that protocol and 404.
// Rewrite src/href starting with '/api/' to the configured server origin. Guarded
// by isTauri() at registration time → web sanitisation is untouched.
if (isTauri()) {
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    for (const attr of ['src', 'href']) {
      const v = node.getAttribute && node.getAttribute(attr)
      if (v && v.startsWith('/api/')) node.setAttribute(attr, serverBase() + v)
    }
  })
}

// looksLikeHtml decides whether to pass content through as raw HTML (legacy
// TipTap-era output) vs render it as Markdown. It must be STRICT: Markdown may
// itself contain inline HTML (e.g. GitLab descriptions with <details>), so we
// only treat content as HTML when it *starts* with a block tag the rich editor
// emitted — otherwise a single inline tag anywhere would wrongly suppress
// Markdown parsing (code fences, bold, etc. rendered as plain text).
function looksLikeHtml(src) {
  return /^\s*<(?:p|div|h[1-6]|ul|ol|blockquote|pre|table|hr)\b/i.test(src)
}

// renderMarkdown turns user-entered markdown into sanitised HTML safe for
// v-html (strips scripts, event handlers, etc.). Links open in a new tab.
export function renderMarkdown(src) {
  if (!src) return ''
  const html = marked.parse(String(src))
  return DOMPurify.sanitize(html, SANITIZE_OPTS)
}

function escapeRe(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// The chip text is lifted out of already-rendered HTML, so `<`/`>`/`&` are
// escaped there already — re-escaping `&` would double it ("&amp;amp;"). Only a
// bare quote can still break out of the attribute, so that is all we touch.
function escapeAttrFromHtml(s) {
  return String(s).replace(/"/g, '&quot;')
}

// highlightMentions wraps "@…" tokens in a styled span. Known member display
// names (which may contain spaces, e.g. "@Ann Lee") are matched first; any other
// "@handle" (a username like @v.sokolov from GitLab, not a Tessera user) is also
// highlighted via a generic fallback token. Operates on rendered HTML and only
// at text boundaries to avoid touching tags/attributes.
//
// The chip carries who it names — `data-type/data-id/data-label`, the same
// contract the legacy TipTap-era chips use (and already allowed by
// SANITIZE_OPTS), so hover cards resolve on old and new content alike. `data-id`
// is present only when the token matched a known member; a generic handle gets
// the label alone.
function highlightMentions(html, members) {
  const known = (members || []).filter((m) => m.label)
  const byLabel = new Map(known.map((m) => [m.label, m]))
  const alts = known
    .map((m) => m.label)
    .sort((a, b) => b.length - a.length)
    .map(escapeRe)
  // Generic handle: a letter/digit then word chars, dots or hyphens.
  alts.push('[A-Za-z0-9][\\w.-]*')
  const re = new RegExp(`(^|[\\s>(])@(${alts.join('|')})`, 'g')
  return html.replace(re, (_, lead, handle) => {
    const m = byLabel.get(handle)
    const id = m && m.id ? ` data-id="${escapeAttrFromHtml(m.id)}"` : ''
    const label = ` data-label="${escapeAttrFromHtml(handle)}"`
    return `${lead}<span class="mention" data-type="mention"${id}${label}>@${handle}</span>`
  })
}

// renderRich renders stored task content for display. Content is Markdown
// (HTML from the brief TipTap era is passed through too). `members` enables
// @-mention highlighting in the output.
export function renderRich(src, members) {
  if (!src) return ''
  const s = String(src)
  const html = looksLikeHtml(s) ? s : marked.parse(s)
  return DOMPurify.sanitize(highlightMentions(html, members), SANITIZE_OPTS)
}

// sanitizeSvgFragment cleans a rendered SVG before it reaches the DOM. Mermaid
// hands us its diagram as markup, which is the one place displayed content
// bypasses renderRich() — mermaid runs with securityLevel:'strict', so this is
// defence in depth rather than an open hole.
//
// Three deviations from DOMPurify's defaults, each load-bearing:
//   • `html: true` + `foreignObject` in ADD_TAGS — mermaid draws node labels as
//     ordinary HTML inside <foreignObject> (DOMPurify lists it as svgDisallowed),
//     so without both the labels vanish and diagrams render with no text at all.
//   • `HTML_INTEGRATION_POINTS` — allowing the <foreignObject> element is not
//     enough; DOMPurify strips HTML children of an SVG parent unless the parent
//     is a registered integration point. The option replaces the map wholesale,
//     hence 'annotation-xml' is repeated to keep the built-in default.
//   • `style` in ADD_TAGS — mermaid ships the diagram's classes in an inline
//     <style> block; dropping it leaves an unstyled skeleton.
//
// We return a DocumentFragment rather than a string on purpose. DOMPurify keeps
// foreignObject off by default because SVG/HTML namespace confusion is an mXSS
// vector, and that class of bypass needs a serialise → re-parse round trip to
// land. Handing back the already-parsed nodes removes the round trip: the caller
// appends what DOMPurify inspected, not a re-parse of it. Everything inside the
// <foreignObject> is still sanitised as HTML — scripts, event handlers and
// foreign schemes are removed.
export function sanitizeSvgFragment(svg) {
  if (!svg) return null
  return DOMPurify.sanitize(String(svg), {
    USE_PROFILES: { svg: true, svgFilters: true, html: true },
    ADD_TAGS: ['style', 'foreignObject'],
    ADD_ATTR: ['class', 'style'],
    HTML_INTEGRATION_POINTS: { foreignobject: true, 'annotation-xml': true },
    RETURN_DOM_FRAGMENT: true,
  })
}

// toggleTaskMarker flips the [ ]↔[x] of the index-th GFM task-list item in the
// source markdown (index = render order, top to bottom). Used by interactive
// preview checkboxes so ticking one rewrites the stored markdown. Returns the
// source unchanged if the index isn't found.
const TASK_MARKER_RE = /^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\])/
export function toggleTaskMarker(src, index) {
  if (!src) return src
  const lines = String(src).split('\n')
  let n = -1
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(TASK_MARKER_RE)
    if (!m) continue
    n++
    if (n === index) {
      const checked = m[2].toLowerCase() === 'x'
      lines[i] = lines[i].replace(TASK_MARKER_RE, `$1${checked ? ' ' : 'x'}$3`)
      break
    }
  }
  return lines.join('\n')
}

// toEditorHtml normalises stored content into HTML for loading into the editor.
// Legacy Markdown is converted; existing HTML is passed through untouched.
export function toEditorHtml(src) {
  if (!src) return ''
  const s = String(src)
  return looksLikeHtml(s) ? s : marked.parse(s)
}
