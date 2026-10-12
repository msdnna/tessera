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
  ADD_ATTR: ['target', 'rel', 'data-type', 'data-id', 'data-label', 'data-task-ref', 'class'],
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

// wordChar reports whether `c` continues a handle. Written by hand rather than as
// `\w`, which is ASCII-only in a JS regex while the names being matched are
// usually Cyrillic: the letter test is "changes under case folding", which holds
// for every alphabet.
function wordChar(c) {
  if (!c) return false
  if ((c >= '0' && c <= '9') || c === '_' || c === '-') return true
  return c.toLowerCase() !== c.toUpperCase()
}

// handleEndsAt reports whether the handle that stops at index `i` really ends
// there, or runs on into the word — the right-hand boundary the alternation below
// cannot express for non-ASCII names. Without it "@Ивановна" draws a chip reading
// "@Иван", stamped with that member's id: the reader hovers the wrong person and,
// because mentionIdsIn reads its ids straight off these chips, the wrong person is
// notified (#2932).
//
// A dot ends a handle only when nothing word-like follows, so "@ivan.petrov" is
// not a mention of "@ivan" while "@Иван." closing a sentence still draws its chip.
function handleEndsAt(s, i) {
  const c = s.charAt(i)
  if (wordChar(c)) return false
  return !(c === '.' && wordChar(s.charAt(i + 1)))
}

// highlightMentions wraps "@…" tokens in a styled span. Known members are matched
// on both `label` (what the composer inserts today — a GitLab login) and `display`
// (their name): comments written before mentions switched to logins still hold
// "@Евгений Полянский", and matching only the label would leave them highlighting
// just the first word via the generic token below. Any other "@handle" (a username
// like @v.sokolov not tied to a known member) is highlighted via a generic fallback
// token. Operates on rendered HTML, only at text boundaries, to avoid touching
// tags/attributes.
//
// The chip carries who it names — `data-type/data-id/data-label`, the same
// contract the legacy TipTap-era chips use (and already allowed by
// SANITIZE_OPTS), so hover cards resolve on old and new content alike. `data-id`
// is present only when the token matched a known member; a generic handle gets
// the label alone.
function highlightMentions(html, members) {
  const known = (members || []).filter((m) => m.label || m.display)
  // Map both handles a member can be written by to the member. A label mapping
  // wins over a display one so a login-shaped mention resolves to its own row.
  const byHandle = new Map()
  for (const m of known) {
    if (m.label && !byHandle.has(m.label)) byHandle.set(m.label, m)
    if (m.display && !byHandle.has(m.display)) byHandle.set(m.display, m)
  }
  const alts = [...new Set(known.flatMap((m) => [m.label, m.display]).filter(Boolean))]
    .sort((a, b) => b.length - a.length)
    .map(escapeRe)
  // Generic handle: a letter/digit then word chars, dots or hyphens.
  alts.push('[A-Za-z0-9][\\w.-]*')
  // The trailing lookaheads fence a member alternative off from an ASCII
  // continuation *inside the engine*, so "@ivanov" and "@ivan.petrov" backtrack
  // past the member "ivan" into the generic token and are drawn as the unresolved
  // handles they are. A bare dot is deliberately allowed through — requiring a
  // word after it is what keeps "@Иван." closing a sentence a chip, since no
  // generic alternative can match Cyrillic. The non-ASCII continuations `\w`
  // cannot see are left to handleEndsAt.
  const re = new RegExp(`(^|[\\s>(])@(${alts.join('|')})(?![\\w-])(?!\\.[\\w-])`, 'g')
  return html.replace(re, (whole, lead, handle, offset) => {
    if (!handleEndsAt(html, offset + whole.length)) return whole
    const m = byHandle.get(handle)
    const id = m && m.id ? ` data-id="${escapeAttrFromHtml(m.id)}"` : ''
    const label = ` data-label="${escapeAttrFromHtml(handle)}"`
    return `${lead}<span class="mention" data-type="mention"${id}${label}>@${handle}</span>`
  })
}

// replaceOutsideMarkup applies `fn` to the DISPLAYED TEXT of already-rendered
// HTML — everything that is neither a code sample nor markup.
//
// Two things are stepped over, for two different reasons:
//
//   • <code>/<pre> blocks, because their contents are a sample and not prose:
//     "#2550" in a diff is not a task link, and "@root" in a shell snippet is
//     not a mention.
//   • tags themselves, because an attribute is not text. Both decorations below
//     splice an element in place of what they matched, and an attribute value
//     carries user prose too: marked writes an image's alt text and a link's
//     title into one. «![скрин #123](/a.png)» therefore had an <a …> spliced
//     inside its own alt="…", which ends the attribute at the chip's first
//     quote — the image lost its alt, the element grew two junk attributes, and
//     the tail of the chip («#123"&gt;») was drawn as visible text next to it.
//     A link title took the anchor's own text with it (#2932).
//
// Stepping over tags also means a decoration's left-hand boundary at the start
// of a text run is the run's own start, which both patterns already accept as
// `^`; the `>` in their lead classes stays for the legacy raw-HTML path, where
// an unescaped `>` inside an attribute can still leave one in a text run.
const SKIP_RE = /<(code|pre)\b[^>]*>[\s\S]*?<\/\1>|<[^>]*>/gi

// SKIP_DECORATED_RE is SKIP_RE plus a finished mention chip, taken whole. Used for
// the pass that runs AFTER highlightMentions (see renderRich): a chip's text names a
// person, not prose, so "#5" inside «@Отдел #5» is no more a task link than "#2550"
// in a code sample — and an <a> spliced into a chip would sit inside the element the
// hover card resolves, swallowing the hover on the half it covers. Both attribute
// orders are matched: today's chips are written by highlightMentions, the legacy
// TipTap-era ones lead with data-type (#2932).
const SKIP_DECORATED_RE =
  /<(code|pre)\b[^>]*>[\s\S]*?<\/\1>|<span[^>]*\b(?:class="mention"|data-type="mention")[^>]*>[\s\S]*?<\/span>|<[^>]*>/gi
export function replaceOutsideMarkup(html, fn, skip = SKIP_RE) {
  const s = String(html)
  let out = ''
  let last = 0
  for (const m of s.matchAll(skip)) {
    out += fn(s.slice(last, m.index)) + m[0]
    last = m.index + m[0].length
  }
  return out + fn(s.slice(last))
}

// linkTaskRefs turns "#123" into a chip that RichContent resolves and navigates
// on click. Capped at 7 digits so long numeric strings don't become links; an
// unknown number simply fails to resolve and tells the user so.
const TASK_REF_RE = /(^|[\s>([])#(\d{1,7})\b(?!\.\d)/g
function linkTaskRefs(html) {
  return html.replace(
    TASK_REF_RE,
    (_, lead, n) => `${lead}<a class="task-ref" data-task-ref="${n}" href="#">#${n}</a>`,
  )
}

// renderRich renders stored task content for display. Content is Markdown
// (HTML from the brief TipTap era is passed through too). `members` enables
// @-mention highlighting in the output; `opts.taskRefs` turns "#123" into task
// links (opt-in — on a board card they would swallow clicks meant for the card).
export function renderRich(src, members, opts = {}) {
  if (!src) return ''
  const s = String(src)
  const html = looksLikeHtml(s) ? s : marked.parse(s)
  // One walk per decoration, not one walk running both: what the first one splices in
  // is markup, and the second must read it as markup too. Chained inside a single
  // callback, linkTaskRefs saw the <span> highlightMentions had just written as plain
  // text — so a member named «Отдел #5» got an <a> spliced into its own data-label,
  // which ends the attribute at the chip's first quote. The chip then lost its label,
  // wore `data-task-ref`/`href` as attributes of its own, and the tail («#5">») was
  // drawn as visible text beside it. The same attribute breakout as the alt/title one
  // replaceOutsideMarkup was written for, one layer in: the walk protected the markup
  // that arrived, not the markup the decorations produce (#2932).
  const withMentions = replaceOutsideMarkup(html, (part) => highlightMentions(part, members))
  const decorated = opts.taskRefs
    ? replaceOutsideMarkup(withMentions, linkTaskRefs, SKIP_DECORATED_RE)
    : withMentions
  return DOMPurify.sanitize(decorated, SANITIZE_OPTS)
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
// source markdown. Used by interactive preview checkboxes so ticking one rewrites
// the stored markdown. Returns the source unchanged if the index isn't found.
//
// `index` is the position of the clicked <input> among the RENDERED checkboxes
// (RichContent counts them off the DOM), which is not the same thing as the
// index-th line that looks like a marker — and the gap is not academic:
//
//   • "- [ ] …" inside a ``` fence or an indented code block is a code SAMPLE,
//     drawn as text with no box (a task description that documents a checklist
//     is exactly where such a sample lives);
//   • "> - [ ] …" inside a blockquote IS drawn as a box, but does not start its
//     line with a bullet;
//   • "- [ ]x", with no space after the bracket, is literal text to GFM.
//
// Counting lines therefore drifted from the rendering in both directions: the
// first real box in a description that quoted a checklist rewrote the line inside
// the code fence, leaving the clicked box to snap back on the next render.
//
// Rather than re-deriving CommonMark's block rules here (indentation inside a
// list is a nested item, not code — the rules do not survive a line scan), each
// candidate line is tried against the renderer: flip it, re-lex, and keep the one
// edit that moved checkbox `index` and left the others alone. The oracle is the
// parser that draws the boxes, so the two cannot drift again. Cost is one lex per
// candidate line, on a click, over content a person wrote by hand.
const TASK_MARKER_RE = /^([ \t>]*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\])/
export function toggleTaskMarker(src, index) {
  if (!src) return src
  const before = taskStates(String(src))
  if (!(index >= 0) || index >= before.length) return src

  const lines = String(src).split('\n')
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(TASK_MARKER_RE)
    if (!m) continue
    const trial = lines.slice()
    trial[i] = lines[i].replace(TASK_MARKER_RE, `$1${m[2].toLowerCase() === 'x' ? ' ' : 'x'}$3`)
    const candidate = trial.join('\n')
    if (flipsOnly(before, taskStates(candidate), index)) return candidate
  }
  return src
}

// taskStates lists the checked/unchecked state of every task item marked's own
// lexer finds, in document order — the order the boxes end up in the DOM. An
// item is emitted before its own sub-tokens so a nested list follows its parent,
// matching how the <input> elements nest.
function taskStates(src) {
  const out = []
  const walk = (tokens) => {
    for (const tok of tokens || []) {
      if (tok.items) {
        for (const item of tok.items) {
          if (item.task) out.push(!!item.checked)
          walk(item.tokens)
        }
        continue
      }
      if (tok.tokens) walk(tok.tokens)
    }
  }
  walk(marked.lexer(String(src)))
  return out
}

// flipsOnly reports whether `after` is `before` with the index-th box toggled.
// A candidate line inside a code block passes no test here: flipping it changes
// the rendered text but not a single box, so the scan moves on to the next one.
function flipsOnly(before, after, index) {
  if (after.length !== before.length) return false
  return before.every((was, i) => (i === index ? after[i] !== was : after[i] === was))
}

// toEditorHtml normalises stored content into HTML for loading into the editor.
// Legacy Markdown is converted; existing HTML is passed through untouched.
export function toEditorHtml(src) {
  if (!src) return ''
  const s = String(src)
  return looksLikeHtml(s) ? s : marked.parse(s)
}
