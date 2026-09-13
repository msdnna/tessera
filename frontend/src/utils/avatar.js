/**
 * The rules the avatar picker runs on (#2932).
 *
 * They are the server's, restated: `handlers/users.go` (UploadMyAvatar) refuses
 * anything over 2 MiB and anything that does not sniff as PNG/JPEG/GIF/WebP.
 * The picker had no rules of its own at all, so both refusals arrived as the
 * server's bare English sentence — "avatar exceeds 2 MiB" — rendered verbatim in
 * a Russian UI (`utils/errors.js` knows no sentinel for either), in other units
 * than the hint sitting one line above it. Saying it here says it before the
 * upload instead of after it, in the reader's own language and units.
 *
 * Kept out of `SettingsView.vue` because a rule that is only ever true inside a
 * component is a rule nobody can pin with a test.
 */

/** The server's `maxAvatarBytes`. */
export const AVATAR_MAX_BYTES = 2 * 1024 * 1024

/** The types the server's sniff allowlist accepts, in its order. */
export const AVATAR_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']

/**
 * Why the server would refuse this file, decided before it is sent.
 *
 * The size check is exact — the server compares the same number of bytes. The
 * type check is deliberately one-sided: it only fires on a type the browser
 * actually reported and the server certainly rejects (an SVG picked through the
 * file dialog's "All files"). A file the browser types as '' is handed over
 * untouched, because the server sniffs the *bytes* — refusing it here would make
 * the picker stricter than the server and bounce an image it would have taken.
 *
 * @param {File|null|undefined} file
 * @returns {''|'tooLarge'|'badType'} a reason code, or '' if nothing is wrong
 */
export function avatarPickError(file) {
  if (!file) return ''
  if (file.size > AVATAR_MAX_BYTES) return 'tooLarge'
  const type = String(file.type || '').toLowerCase()
  if (type && !AVATAR_TYPES.includes(type)) return 'badType'
  return ''
}
