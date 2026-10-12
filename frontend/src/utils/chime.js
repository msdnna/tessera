// The toast chime (#3121): two short soft notes made by WebAudio, so there is no
// sound file to ship or cache — the same in the browser and the desktop webview.
// Quiet on purpose (a notice, not an alarm) and rate-limited: a burst of toasts
// rings once. Browsers keep audio locked until the page has been interacted
// with; before that the call is a silent no-op.

const GAP_MS = 2000
let ctx = null
let last = 0

export function playChime({ now = Date.now() } = {}) {
  if (now - last < GAP_MS) return false
  const AC = globalThis.AudioContext || globalThis.webkitAudioContext
  if (!AC) return false
  try {
    ctx ||= new AC()
    if (ctx.state === 'suspended') ctx.resume?.()
    const t0 = ctx.currentTime
    const notes = [
      [880, 0],
      [1318.5, 0.09],
    ]
    for (const [freq, at] of notes) {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, t0 + at)
      gain.gain.exponentialRampToValueAtTime(0.07, t0 + at + 0.015)
      gain.gain.exponentialRampToValueAtTime(0.0001, t0 + at + 0.22)
      osc.connect(gain).connect(ctx.destination)
      osc.start(t0 + at)
      osc.stop(t0 + at + 0.24)
    }
    last = now
    return true
  } catch {
    return false
  }
}

// For tests: forget the rate limit.
export function resetChime() {
  last = 0
}
