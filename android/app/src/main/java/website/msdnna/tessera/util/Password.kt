package website.msdnna.tessera.util

import website.msdnna.tessera.R
import website.msdnna.tessera.ui.UiText

// The password rule the server enforces, in one place (#2932) — mirror of
// `frontend/src/utils/password.js` and `backend/internal/auth.CheckPasswordPolicy`.
//
// Android knew even less about it than the web did: the registration form had no
// rule at all, and the settings form had `next.length >= 8`. Neither end of the
// real rule is measured in `String.length`, so both were wrong — quietly, and in
// opposite directions.
//
// Mirrored rather than merely handled on failure for the same reason the avatar
// and attachment pickers are: the user should learn the passphrase cannot be
// stored while typing it, not after a submit that comes back in English.

/** Runes, matching the server's `binding:"min=8"` — gin's validator counts
 *  characters, not UTF-16 units. */
const val MIN_PASSWORD_CHARS = 8

/** Bytes, matching bcrypt's own ceiling (`auth.MaxPasswordBytes`). Deliberately a
 *  different unit from the minimum, because the two rules genuinely are: a
 *  37-letter Russian passphrase is 74 bytes and cannot be hashed at all. */
const val MAX_PASSWORD_BYTES = 72

/** Which end of the rule a password fails — "too short" and "too long" are
 *  different instructions to the user, and a single `valid` flag was what made
 *  the long case unreportable. */
enum class PasswordProblem { SHORT, LONG }

/** Length in the unit the server's minimum measures: code points, not UTF-16
 *  units (eight emoji are eight characters to the server and sixteen to
 *  `String.length`). */
fun passwordChars(pw: String?): Int {
    val s = pw ?: ""
    return if (s.isEmpty()) 0 else s.codePointCount(0, s.length)
}

/** Length in the unit bcrypt measures: UTF-8 bytes. */
fun passwordBytes(pw: String?): Int = (pw ?: "").toByteArray(Charsets.UTF_8).size

/** What is wrong with a password, or null when it can be stored. */
fun passwordProblem(pw: String?): PasswordProblem? {
    val s = pw ?: ""
    if (passwordChars(s) < MIN_PASSWORD_CHARS) return PasswordProblem.SHORT
    if (passwordBytes(s) > MAX_PASSWORD_BYTES) return PasswordProblem.LONG
    return null
}

/** True when a password can be stored. Sugar for the common call site. */
fun isPasswordValid(pw: String?): Boolean = passwordProblem(pw) == null

/**
 * The reason a password is refused, phrased in the unit that end of the rule
 * uses — characters for the minimum, bytes for the ceiling, because saying
 * "72 characters" would be a lie for any alphabet whose letters are two bytes
 * wide. Null when the password is fine.
 *
 * [UiText], not a ready string: the message outlives the keystroke that produced
 * it and must survive a locale change like every other error on these screens.
 */
fun passwordError(pw: String?): UiText? = when (passwordProblem(pw)) {
    PasswordProblem.SHORT -> UiText.Res(R.string.auth_password_too_short, listOf(MIN_PASSWORD_CHARS))
    PasswordProblem.LONG -> UiText.Res(R.string.auth_password_too_long, listOf(MAX_PASSWORD_BYTES))
    null -> null
}
