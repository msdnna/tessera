// Quick-action helpers for the markdown editor's `/`-autocomplete.
//
// The parser itself lives on the backend (internal/quickact) — these are only
// the bits the editor needs locally: where a `/`-query starts, which registry
// entries match it, and what text to insert. Keep canonCommandKey in sync with
// commandKeyRe in backend/handlers/workspace_commands.go.

const KEY_RE = /^[a-z0-9][a-z0-9_-]{0,31}$/

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
      description: c.description || '',
      aliases: c.aliases || [],
      arg: c.arg || 'none',
      argOptional: !!c.arg_optional,
      example: c.example || '',
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
export function hasCommandLine(body) {
  return /(?:^|\n)\s*\/[a-z]/i.test(String(body || ''))
}
