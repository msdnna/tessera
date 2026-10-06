package website.msdnna.tessera.util

/**
 * Avatar initials, in one place — the Kotlin mirror of web `utils/initials.js`.
 *
 * The rule has three branches and they are not interchangeable:
 *   "Василий Соколов" → "ВС"  (two words → first letter of each)
 *   "a.fokin"         → "AF"  (dot-separated handle → first letter of each part)
 *   "msdnna"          → "MS"  (single token → first two letters)
 *
 * It used to live as five private copies — the card, the modal, the member
 * avatar, the profile screen and the admin list — and two of them knew only the
 * first and last branch. For a dot handle those two produced `take(2)` of the raw
 * string: "A.", a letter and a punctuation mark, on the very screens where a
 * person looks at their *own* avatar. The dot case is not exotic here: a GitLab
 * roster hands us logins in exactly that shape, and both of those screens fall
 * back to the e-mail when the name is empty ("ivan.petrov@…" → "IP", not "IV").
 */
fun avatarInitials(name: String?): String {
    val s = (name ?: "").trim()
    if (s.isEmpty()) return "?"
    // A dot-separated handle (a.fokin, v.sokolov) before whitespace: the parts of
    // a handle are the words here.
    if (s.contains('.')) {
        val parts = s.split('.').map { it.trim() }.filter { it.isNotEmpty() }
        if (parts.size >= 2) return "${parts[0].first()}${parts[1].first()}".uppercase()
    }
    val words = s.split(Regex("\\s+")).filter { it.isNotEmpty() }
    if (words.size >= 2) return "${words[0].first()}${words[1].first()}".uppercase()
    return s.take(2).uppercase()
}
