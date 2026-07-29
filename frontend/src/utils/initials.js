// initials derives a 1–2 letter avatar label from a person's name/handle:
//   "Василий Соколов" → "ВС"  (two words → first letter of each)
//   "a.fokin"         → "AF"  (dot-separated handle → first letter of each part)
//   "msdnna"          → "MS"  (single token → first two letters)
export function initials(name) {
  const s = String(name || '').trim()
  if (!s) return '?'
  // dot-separated handle (a.fokin, v.sokolov)
  if (s.includes('.')) {
    const parts = s
      .split('.')
      .map((p) => p.trim())
      .filter(Boolean)
    if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  }
  const words = s.split(/\s+/).filter(Boolean)
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase()
  return s.slice(0, 2).toUpperCase()
}
