import { i18n } from '@/i18n'

// The password rule the server enforces, in one place (#2932).
//
// It lived inline in three views as three different expressions — `pw.length >= 8`
// in the settings form, `pw.value.length >= 8` on the reset page, `password.value
// .length < 8` on register — and all three knew only half of it. The upper bound
// is bcrypt's and nobody had ever written it down: a password over 72 *bytes*
// cannot be hashed at all, so a 37-letter Russian passphrase came back as
// "internal error" from a server that had done nothing wrong.
//
// Mirrored here rather than merely handled on failure for the reason the avatar
// and attachment pickers already are: the user should learn the passphrase is
// too long while typing it, not after submitting a form that then signs them out
// of nothing and tells them the server broke.

// Runes, matching `binding:"min=8"` — gin's validator counts characters, not
// UTF-16 units, so `"".length` would disagree on anything outside the BMP.
export const MIN_PASSWORD_CHARS = 8

// Bytes, matching bcrypt's own ceiling (auth.MaxPasswordBytes). Deliberately a
// different unit from the minimum, because the two rules genuinely are.
export const MAX_PASSWORD_BYTES = 72

const encoder = new TextEncoder()

/**
 * Length of a password in the unit bcrypt measures.
 * @param {string} pw
 * @returns {number} UTF-8 byte length
 */
export function passwordBytes(pw) {
  return encoder.encode(String(pw ?? '')).length
}

/**
 * Length of a password in the unit the server's minimum measures.
 * @param {string} pw
 * @returns {number} count of code points
 */
export function passwordChars(pw) {
  return [...String(pw ?? '')].length
}

/**
 * What is wrong with a password, if anything.
 *
 * Returns a reason rather than a boolean so the caller can say which end it
 * failed — "too short" and "too long" are different instructions to the user,
 * and a single `valid` flag was what made the long case unreportable.
 *
 * @param {string} pw
 * @returns {'short'|'long'|null} null when it can be stored
 */
export function passwordProblem(pw) {
  const s = String(pw ?? '')
  if (passwordChars(s) < MIN_PASSWORD_CHARS) return 'short'
  if (passwordBytes(s) > MAX_PASSWORD_BYTES) return 'long'
  return null
}

/**
 * True when a password can be stored. Sugar for the common call site.
 * @param {string} pw
 */
export function isPasswordValid(pw) {
  return passwordProblem(pw) === null
}

/**
 * The message for a failing password, phrased in the reader's language and in
 * the unit that end of the rule uses — characters for the minimum, bytes for the
 * ceiling, because saying "72 characters" would be a lie for any alphabet whose
 * letters are two bytes wide.
 *
 * @param {string} pw
 * @returns {string} empty when the password is fine
 */
export function passwordError(pw) {
  const problem = passwordProblem(pw)
  if (!problem) return ''
  const t = i18n.global.t
  return problem === 'short'
    ? t('common.auth.validation.passwordTooShort')
    : t('common.auth.validation.passwordTooLong', { bytes: MAX_PASSWORD_BYTES })
}

/**
 * A rough strength of a password for the meter under the field (v2, #3107):
 * 0 — nothing typed or it cannot be stored, 1 weak … 4 strong. Length does most
 * of the work (it is what makes a password hard to guess), a mix of letter case,
 * digits and symbols adds a step. Advice only — the server's rule is
 * `passwordProblem`, and a "weak" password that passes it is still accepted.
 *
 * @param {string} pw
 * @returns {0|1|2|3|4}
 */
export function passwordStrength(pw) {
  const s = String(pw ?? '')
  if (!s || passwordProblem(s)) return s ? 1 : 0
  const n = passwordChars(s)
  const kinds = [/\p{Ll}/u, /\p{Lu}/u, /\d/, /[^\p{L}\d]/u].filter((re) => re.test(s)).length
  let score = n >= 16 ? 3 : n >= 12 ? 2 : 1
  if (kinds >= 3) score += 1
  return Math.min(4, score)
}
