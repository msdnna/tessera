// The last request the server refused or never answered (#3117), for the error
// toast's «Подробнее»: what was asked, what came back, the request id to quote.
//
// Most callers report a failure as `message.error(e.message)` — a sentence, with
// everything a support conversation needs already thrown away. The axios
// interceptor notes the failure here as it rejects; an error toast raised a
// moment later picks it up. Matching by time, not by text: the caller often
// rephrases the error before showing it.

let last = null

export function noteFailure(f) {
  last = { ...f, at: Date.now() }
}

// The failure noted within `ms` before now, once — a second error toast must not
// claim the same request.
export function takeRecentFailure(ms = 2000, now = Date.now()) {
  if (!last || now - last.at > ms) return null
  const f = last
  last = null
  return f
}

export function clearFailure() {
  last = null
}
