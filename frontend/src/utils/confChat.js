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
 * The server's `MaxChatTotalBytes` — the cap on one message's files *together*.
 *
 * The fourth rule, and the one nobody had: `maxChatFiles` bounds how many files
 * travel, never how much. Five 20 MB videos passed every check on every client,
 * and the request was then refused server-side — before the route had a body
 * budget of its own, by the transport, with "request body too large", which
 * names nothing the user picked (#2932).
 */
export const CONF_CHAT_MAX_TOTAL_BYTES = 25 * 1024 * 1024

/**
 * Why the server would refuse this file on top of the ones already queued,
 * decided before anything is uploaded.
 *
 * Order mirrors the server's: the count first (`maxChatFiles`), then the file
 * (`maxAttachmentBytes`), then the batch (`MaxChatTotalBytes`) — so a single
 * 30 MB video reads as one over-sized file, which is what the composer can name,
 * rather than as a batch the user has not built yet.
 *
 * @param {Array<{size?: number}>} pending files already queued for this message
 * @param {File|null|undefined} file the newly picked one
 * @returns {''|'tooMany'|'tooLarge'|'tooLargeTotal'} a reason code, or ''
 */
export function confChatPickError(pending, file) {
  if (!file) return ''
  const queued = pending || []
  if (queued.length >= CONF_CHAT_MAX_FILES) return 'tooMany'
  const size = Number(file.size) || 0
  if (size > CONF_CHAT_MAX_BYTES) return 'tooLarge'
  const total = queued.reduce((sum, f) => sum + (Number(f?.size) || 0), 0)
  if (total + size > CONF_CHAT_MAX_TOTAL_BYTES) return 'tooLargeTotal'
  return ''
}

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
