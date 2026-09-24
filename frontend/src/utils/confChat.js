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
 *
 * `mergeChatTail` at the bottom is the log's rather than the composer's, and
 * lives here for the same reason: a rule that only ever runs inside a component
 * is a rule nobody can pin with a test.
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

/**
 * Whether message `a` comes before message `b` in the log.
 *
 * The server's cursor is (created_at, id) — see `ListConferenceMessages` — and
 * this is that order, restated for the client. `created_at` is compared as an
 * instant rather than as text: Go drops trailing zeros from the fractional
 * second, so "10:00:00.1Z" and "10:00:00.05Z" sort the wrong way round as
 * strings. The id is the tie-break, exactly as in SQL.
 */
function chatBefore(a, b) {
  const ta = Date.parse(a?.created_at) || 0
  const tb = Date.parse(b?.created_at) || 0
  if (ta !== tb) return ta < tb
  return String(a?.id || '') < String(b?.id || '')
}

/**
 * Folds a freshly fetched tail page into the log already on screen.
 *
 * The room socket's nudge carries no payload, so the rail's only answer is to
 * refetch the newest page — but *replacing* the log with that page throws away
 * every older page the reader paged back through. In a lively call that happens
 * on every line anybody types: the message being re-read vanishes mid-sentence
 * and the view lands somewhere arbitrary, which is the very thing the "don't
 * scroll unless the reader is at the bottom" rule exists to prevent (#2932).
 *
 * So: keep what is strictly older than the page's first message, and let the
 * page be the truth from there on — a line deleted inside that window really
 * goes away, which a blind append would keep on screen forever. An empty page
 * means an empty chat (the tail is the newest PAGE messages; nothing can hide
 * behind it), so nothing is kept.
 *
 * Stitching requires the page to actually touch what is on screen, and that is
 * what the id overlap below decides. Come back from a tunnel to more than PAGE
 * new lines and the page no longer reaches the log we hold: keeping both halves
 * would paint the conversation with a silent hole in the middle, and the missing
 * lines are the ones nobody can ask for — "показать более ранние" pages back
 * from the *oldest* message on screen, so it walks away from the gap rather than
 * into it. The page alone is then the truth; the history it displaces is one
 * "load older" away, while the hole was permanent (#2932). Android's
 * `confChatMergeTail` has decided it this way since the rail was written.
 *
 * The id filter is the belt to the cursor's braces: two messages a microsecond
 * apart land in the same millisecond once `Date.parse` is done with them, and a
 * message that straddles the boundary that way must not be drawn twice — Vue
 * would key both rows the same.
 *
 * @param {Array<object>} current what the rail shows now, oldest first
 * @param {Array<object>} fresh the newly fetched tail page, oldest first
 * @returns {Array<object>} the merged log, oldest first
 */
export function mergeChatTail(current, fresh) {
  const page = fresh || []
  if (!page.length) return []
  const ids = new Set(page.map((m) => m?.id))
  const shown = current || []
  if (!shown.some((m) => ids.has(m?.id))) return [...page]
  const older = shown.filter((m) => !ids.has(m?.id) && chatBefore(m, page[0]))
  return [...older, ...page]
}
