// Task titles are single-line everywhere they're read back: the card renders
// them as HTML (a newline collapses into a space) while the modal keeps them in
// an <input>, which strips newlines out of value per spec. A title that carries
// a real "\n" therefore looks different in the two places — see #2813.
//
// The server normalizes too (backend/handlers/tasks.go); this copy is what keeps
// the input honest before the request goes out, and covers pasted multi-line text.

// normalizeTitle collapses every whitespace run — newlines, tabs, doubled
// spaces — into one space and trims the ends. Returns '' when nothing usable
// remains, which callers already treat as "don't submit".
export function normalizeTitle(s) {
  return s == null ? '' : String(s).replace(/\s+/g, ' ').trim()
}
