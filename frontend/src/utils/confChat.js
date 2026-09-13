/**
 * The rules the in-call chat's composer runs on (#2864, subtask #2873).
 *
 * They live here rather than inside `ConferenceChat.vue` because all three are
 * the server's, restated: `handlers/conference_chat.go` refuses an over-long
 * body, an over-sized file and too many files, and the composer's job is to say
 * so before the press instead of after it. A rule that is only ever true inside
 * a component is a rule nobody can pin with a test — and the one that went
 * missing here (the body length) is exactly the one that had no test.
 *
 * The Android client keeps the same three in `util/ConferenceChat.kt`; when one
 * of them moves on the server, all three copies move together.
 */

/** The server's `maxChatBody`, in *code points* — see {@link confChatBodyLength}. */
export const CONF_CHAT_MAX_BODY = 4000

/** The server's `maxChatFiles`. */
export const CONF_CHAT_MAX_FILES = 5

/** The server's per-file cap (25 MiB), checked here so a 30 MB video fails instantly. */
export const CONF_CHAT_MAX_BYTES = 25 * 1024 * 1024

/**
 * The message body's length as the server counts it.
 *
 * Code points, not `String.length`. JS measures UTF-16 units, Go measures runes,
 * and every emoji is two of the former and one of the latter — counting the JS
 * way makes the composer stricter than the server and refuses to send a message
 * the server would have taken. Trimmed because the server trims before it counts.
 */
export function confChatBodyLength(body) {
  return [...String(body || '').trim()].length
}

/** Whether the body is longer than the server will accept. */
export function confChatBodyTooLong(body) {
  return confChatBodyLength(body) > CONF_CHAT_MAX_BODY
}

/** Whether the composer's send button does anything. */
export function confChatCanSend(draft, pendingCount, sending) {
  if (sending) return false
  if (confChatBodyTooLong(draft)) return false
  return confChatBodyLength(draft) > 0 || pendingCount > 0
}
