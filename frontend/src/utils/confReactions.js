// Reactions in a call (#3054, CallScreen «HappyOutline»).
//
// A reaction is a moment, not a record: it goes over the SFU room's data channel
// (LiveKit `publishData`, topic `reaction`) to whoever is in the call right now
// and is never stored — someone who joins a minute later has nothing to catch up
// on. That is also why it does not ride our own room socket: that socket evicts a
// participant whose buffer overflows (see internal/confroom), and a burst of
// applause must not disconnect the people clapping.
//
// Pure — the transport does the sending and the listening; this file says what
// goes on the wire, what is accepted off it, and how long a bubble stays up.

// The palette, in the order of the picker. A closed set on purpose: whatever
// arrives off the wire is checked against it, so a peer cannot put arbitrary
// text over everyone's stage.
export const REACTIONS = ['👍', '👏', '😂', '🎉', '❤️', '🤔']

// The data-channel topic, shared with Android.
export const REACTION_TOPIC = 'reaction'

// How long a bubble floats over the stage.
export const REACTION_TTL_MS = 4000

// At most this many bubbles on screen: a room of twenty all clapping at once
// should read as applause, not cover the speaker.
export const REACTION_MAX = 12

// Our own sending is throttled — a held key or a frantic click is one gesture,
// and the channel is everyone's.
export const REACTION_BURST = 4
export const REACTION_WINDOW_MS = 2000

const enc = typeof TextEncoder !== 'undefined' ? new TextEncoder() : null
const dec = typeof TextDecoder !== 'undefined' ? new TextDecoder() : null

// The wire form: `{"t":"reaction","e":"👍"}` — the same JSON Android sends.
export function encodeReaction(emoji) {
  if (!REACTIONS.includes(emoji) || !enc) return null
  return enc.encode(JSON.stringify({ t: 'reaction', e: emoji }))
}

// The emoji a payload carries, or null for anything that is not one of ours.
export function decodeReaction(payload) {
  if (!payload || !dec) return null
  try {
    const msg = JSON.parse(dec.decode(payload))
    if (msg?.t !== 'reaction' || !REACTIONS.includes(msg.e)) return null
    return msg.e
  } catch {
    return null
  }
}

// Whether we may send another one now, given the times of our recent ones.
export function canReact(sentAt, now = Date.now()) {
  return sentAt.filter((at) => now - at < REACTION_WINDOW_MS).length < REACTION_BURST
}

let seq = 0

// addReaction returns the feed with one more bubble, newest last, capped.
// `lane` spreads bubbles across the stage's bottom edge so a burst does not
// stack into one column; it is derived from the sequence, so it is stable for a
// bubble's whole life.
export function addReaction(feed, { identity, name, emoji, at = Date.now() }) {
  seq += 1
  const item = { id: `r${seq}`, identity, name: name || '', emoji, at, lane: seq % 5 }
  const next = [...feed, item]
  return next.length > REACTION_MAX ? next.slice(next.length - REACTION_MAX) : next
}

// The feed without the bubbles that have had their time.
export function pruneReactions(feed, now = Date.now()) {
  return feed.filter((r) => now - r.at < REACTION_TTL_MS)
}
