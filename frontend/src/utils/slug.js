// Slug preview for the address fields — a port of the server's `internal/slug`
// (Go) so what the user sees while typing is exactly what gets stored. The
// transliteration table is duplicated on purpose: the alternative is a round
// trip per keystroke. Kept honest by the table in tests/ut-misc.spec.js, which
// mirrors the Go cases; if the Go table changes, change it here in the same edit.
const CYRILLIC = {
  а: 'a',
  б: 'b',
  в: 'v',
  г: 'g',
  д: 'd',
  е: 'e',
  ё: 'e',
  ж: 'zh',
  з: 'z',
  и: 'i',
  й: 'y',
  к: 'k',
  л: 'l',
  м: 'm',
  н: 'n',
  о: 'o',
  п: 'p',
  р: 'r',
  с: 's',
  т: 't',
  у: 'u',
  ф: 'f',
  х: 'h',
  ц: 'ts',
  ч: 'ch',
  ш: 'sh',
  щ: 'shch',
  ъ: '',
  ы: 'y',
  ь: '',
  э: 'e',
  ю: 'yu',
  я: 'ya',
}

// makeSlug turns a display name into a slug: transliterated, lowercased, with
// runs of non-alphanumerics collapsed to single hyphens. Returns '' when
// nothing usable remains — callers treat that as "no address yet".
export function makeSlug(name) {
  const raw = name == null ? '' : String(name)
  const text = raw.trim().toLowerCase()
  let out = ''
  let prevHyphen = false
  for (const ch of text) {
    if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
      out += ch
      prevHyphen = false
    } else if (ch in CYRILLIC) {
      // Hard/soft signs map to nothing, and must not count as emitted text —
      // otherwise 'ъ!' would keep a leading hyphen the server wouldn't produce.
      if (CYRILLIC[ch]) {
        out += CYRILLIC[ch]
        prevHyphen = false
      }
    } else if (!prevHyphen && out.length > 0) {
      out += '-'
      prevHyphen = true
    }
  }
  return out.replace(/^-+|-+$/g, '')
}
