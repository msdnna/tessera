import { marked } from 'marked'
import { markedHighlight } from 'marked-highlight'
import DOMPurify from 'dompurify'
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
      return hljs.getLanguage(lang) ? hljs.highlight(code, { language: lang }).value : escapeHtml(code)
    },
  }),
)

// Attributes the rich editor emits that DOMPurify must preserve: link targets
// and mention chips (`<span data-type="mention" data-id="…">@Name</span>`).
const SANITIZE_OPTS = {
  ADD_ATTR: ['target', 'rel', 'data-type', 'data-id', 'data-label', 'class'],
}

// looksLikeHtml is a cheap heuristic: rich-editor output contains HTML tags,
// legacy descriptions/comments are plain Markdown.
function looksLikeHtml(src) {
  return /<\/?[a-z][\s\S]*>/i.test(src)
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

// highlightMentions wraps "@Name" tokens for known members in a styled span.
// Operates on rendered HTML; matches only at text boundaries to avoid touching
// tags/attributes. Longer names first so "@Ann Lee" wins over "@Ann".
function highlightMentions(html, members) {
  if (!members || !members.length) return html
  const labels = members
    .map((m) => m.label)
    .filter(Boolean)
    .sort((a, b) => b.length - a.length)
    .map(escapeRe)
  if (!labels.length) return html
  const re = new RegExp(`(^|[\\s>(])@(${labels.join('|')})`, 'g')
  return html.replace(re, '$1<span class="mention">@$2</span>')
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

// toEditorHtml normalises stored content into HTML for loading into the editor.
// Legacy Markdown is converted; existing HTML is passed through untouched.
export function toEditorHtml(src) {
  if (!src) return ''
  const s = String(src)
  return looksLikeHtml(s) ? s : marked.parse(s)
}
