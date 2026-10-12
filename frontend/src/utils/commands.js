// Quick-action helpers for the markdown editor's `/`-autocomplete.
//
// The parser itself lives on the backend (internal/quickact) — these are only
// the bits the editor needs locally: where a `/`-query starts, which registry
// entries match it, and what text to insert. Keep canonCommandKey in sync with
// commandKeyRe in backend/handlers/workspace_commands.go.
import { i18n } from '@/i18n'

const KEY_RE = /^[a-z0-9][a-z0-9_-]{0,31}$/

// builtinLabel translates a built-in command's display text (#2932).
//
// The registry is the backend's, and its Description/Example are hard-coded
// Russian — the API hands them over verbatim, so an English UI used to draw a
// fully Russian `/`-popup. The *parser* was bilingual all along (args.go maps
// none/low/high/urgent as well as нет/низкий/высокий/срочный, ParseDate takes
// today/tomorrow), so only the shop window had to catch up; the English examples
// below are arguments the backend really executes.
//
// The server string stays the fallback: a command added to the registry before
// the bundles know about it still shows something, and a workspace-defined
// command is never touched — its wording belongs to whoever wrote it.
//
// Reading `locale.value` explicitly makes a caller's computed() re-run on a
// language switch; `t()` alone already does, but the dependency is worth being
// obvious about here, since this is called from a Pinia getter.
function builtinLabel(key, field, fallback) {
  const g = i18n.global
  void g.locale.value
  const path = `commands.builtin.${key}.${field}`
  return g.te(path) ? g.t(path) : fallback
}

// canonCommandKey normalises a user-typed key to its storage form: no leading
// slash, lowercase, trimmed. Returns '' when nothing usable is left.
export function canonCommandKey(raw) {
  return String(raw || '')
    .trim()
    .replace(/^\/+/, '')
    .trim()
    .toLowerCase()
}

export function isValidCommandKey(key) {
  return KEY_RE.test(canonCommandKey(key))
}

// builtinCommandKeys collects every key the backend treats as built-in: the
// registry keys AND their aliases, because quickact.byKey maps both and
// IsBuiltin looks the dictionary editor's key up in that same map. Checking
// only `.key` lets a custom "/done" through the editor and into a 400 that
// rejects the whole PUT — and the toast can't say which row caused it.
export function builtinCommandKeys(items = []) {
  const out = new Set()
  for (const c of items || []) {
    for (const raw of [c?.key, ...(c?.aliases || [])]) {
      const key = canonCommandKey(raw)
      if (key) out.add(key)
    }
  }
  return out
}

// detectSlashQuery finds an open `/`-query in the text up to the caret and
// returns { start, query } (start = index of the slash), or null.
//
// Unlike @-mentions, the trigger is a slash at the START OF A LINE only. Firing
// after any whitespace would pop the menu on `cd /home`, `src/utils` and `24/7`
// — all common in task text. This mirrors the backend, which only treats a line
// whose first non-space character is `/` as a command.
export function detectSlashQuery(upto) {
  const m = String(upto || '').match(/(?:^|\n)\/([a-z0-9_-]*)$/)
  if (!m) return null
  return { start: upto.length - m[1].length - 1, query: m[1] }
}

// commandItems flattens the API's registry response into popup rows. Built-in
// commands come first (registry order = popup order), custom ones after.
export function commandItems(builtin = [], custom = []) {
  return [
    ...(builtin || []).map((c) => ({
      key: c.key,
      description: builtinLabel(c.key, 'description', c.description || ''),
      aliases: c.aliases || [],
      arg: c.arg || 'none',
      argOptional: !!c.arg_optional,
      example: builtinLabel(c.key, 'example', c.example || ''),
      builtin: true,
    })),
    ...(custom || []).map((c) => ({
      key: c.key,
      description: c.description || '',
      aliases: [],
      arg: 'none',
      argOptional: false,
      example: `/${c.key}`,
      builtin: false,
    })),
  ]
}

// matchCommands filters rows for a query, matching key, aliases and description
// (so «срок» finds /due). An empty query lists everything, capped at `limit`.
export function matchCommands(items, query, limit = 8) {
  const q = String(query || '').toLowerCase()
  const rows = (items || []).filter((c) => {
    if (!q) return true
    if (c.key.toLowerCase().includes(q)) return true
    if ((c.aliases || []).some((a) => a.toLowerCase().includes(q))) return true
    return (c.description || '').toLowerCase().includes(q)
  })
  return rows.slice(0, limit)
}

// commandInsertText is what picking a row types into the editor: commands that
// take an argument leave the caret after a space, argument-less ones end the
// line so the next command can follow.
export function commandInsertText(item) {
  if (!item) return ''
  const takesArg = item.builtin && item.arg && item.arg !== 'none'
  return takesArg ? `/${item.key} ` : `/${item.key}\n`
}

// hasCommandLine reports whether a body has any line that starts with a slash —
// the cheap gate before asking the backend for a command preview.
//
// A digit counts as the start of a key, the same as a letter: the dictionary
// editor stores keys matching KEY_RE above, which opens with `[a-z0-9]`, so a
// letters-only gate skipped the preview request for "/1on1" entirely — the
// editor showed nothing at all for a command the workspace had defined.
export function hasCommandLine(body) {
  return /(?:^|\n)\s*\/[a-z0-9]/i.test(String(body || ''))
}
