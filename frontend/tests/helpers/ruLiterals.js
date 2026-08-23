// Scanner behind the "no new Russian literals" ratchet (#2799, wave 0).
//
// eslint's `vue/no-bare-strings-in-template` only sees templates, and roughly
// half of the UI text lives in `<script>` — option tables, toast bodies, slash
// commands. This walks the source tree instead, so both halves are covered by
// one gate.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

// vitest rewrites `import.meta.url` to a non-file scheme, so the module's own
// path is not available here — the working directory is. Both callers (vitest
// and the eslint config) run from frontend/.
export const FRONTEND_ROOT = process.cwd()
const SRC = join(FRONTEND_ROOT, 'src')

const SCANNED_EXT = ['.vue', '.js']
const CYRILLIC = /[Ѐ-ӿ]/

// Files that legitimately hold Cyrillic forever — they are data, not interface.
// Anything here is invisible to the ratchet in both directions.
export const ALLOWLIST = [
  // Message bundles: the Russian text is the point.
  'src/locales/',
  // Transliteration table mirroring the server's internal/slug (Go). Not UI —
  // it maps Cyrillic input to ASCII slugs and has to keep the alphabet inline.
  'src/utils/slug.js',
  // Slash-menu typing aliases ("/таблица", "/table"). The menu's own labels are
  // in the catalogue; these are search keys, and both alphabets have to answer
  // in either language — an English interface still has Russian-speaking hands
  // typing at it.
  'src/utils/docSlash.js',
  // Board-column status heuristic. Column names are user data seeded by the
  // server in Russian ("К работе", "На рассмотрении"), so this table matches
  // what the rows are actually called — translating it would stop it matching.
  'src/utils/columnStatus.js',
  // Estimate-input unit aliases ("3д 4ч" next to "3d 4h"). Every label this
  // module renders already comes from the catalogue; what is left is the single
  // letter a user may type, and both alphabets have to keep answering in either
  // language — the same rule as the slash-menu keys above.
  'src/utils/estimation.js',
]

// Strips comments before looking for Cyrillic, so a Russian word inside an
// English explanation ("with a «Назад»") is not mistaken for interface text.
// Deliberately conservative: only block comments and whole-line `//` / `*`
// lines are removed, never a trailing `//` — cutting those would also eat the
// tail of any string holding a URL.
export function stripComments(text) {
  return text
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\*)/.test(line))
    .join('\n')
}

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (SCANNED_EXT.some((e) => entry.endsWith(e))) out.push(full)
  }
  return out
}

export function isAllowlisted(relPath) {
  return ALLOWLIST.some((a) => (a.endsWith('/') ? relPath.startsWith(a) : relPath === a))
}

// Returns the sorted, POSIX-separated paths under src/ that still carry
// Russian text outside comments.
export function scanRussianFiles() {
  return walk(SRC)
    .map((f) => relative(FRONTEND_ROOT, f).split(sep).join('/'))
    .filter((rel) => !isAllowlisted(rel))
    .filter((rel) => CYRILLIC.test(stripComments(readFileSync(join(FRONTEND_ROOT, rel), 'utf8'))))
    .sort()
}
