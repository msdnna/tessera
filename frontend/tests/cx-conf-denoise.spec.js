import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// Advanced noise suppression for the conference microphone (#2864, subtask #2889).
//
// Three layers get their own tests, because they fail in unrelated ways:
//   - the DSP itself, exercised on synthetic signals (no WebAudio involved);
//   - the guard that keeps the DSP shippable into an AudioWorklet realm;
//   - the wiring: the LiveKit-shaped processor, the transport, the menu.

import {
  createDenoiser,
  createNoiseFilter,
  denoiseSource,
  denoiseSupported,
  denoiseWorkletSource,
  DENOISE_NAME,
} from '@/utils/denoise'

const SR = 48000
const HOP = 128

/** Deterministic white noise — a seeded LCG, so a failure is reproducible. */
function noiseGen(seed) {
  let s = seed >>> 0
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return (s / 4294967296) * 2 - 1
  }
}

/** Run a signal through a denoiser one render quantum at a time. */
function run(input, opts) {
  const d = createDenoiser(SR, opts)
  const out = new Float32Array(input.length)
  const block = new Float32Array(HOP)
  for (let i = 0; i + HOP <= input.length; i += HOP) {
    d.process(input.subarray(i, i + HOP), block)
    out.set(block, i)
  }
  return out
}

function rms(buf, from = 0, to = buf.length) {
  let sum = 0
  for (let i = from; i < to; i++) sum += buf[i] * buf[i]
  return Math.sqrt(sum / (to - from))
}

describe('denoise DSP', () => {
  it('reconstructs the signal when the gate is wide open', () => {
    // With no over-subtraction and a unity floor every bin passes at gain 1, so
    // the analysis/synthesis pair must be transparent. This is the property that
    // catches a wrong window, a wrong overlap normalisation or a broken IFFT —
    // all of which would otherwise just sound "a bit off" and pass every other
    // assertion here.
    const n = 8192
    const input = new Float32Array(n)
    for (let i = 0; i < n; i++) input[i] = 0.4 * Math.sin((2 * Math.PI * 1000 * i) / SR)
    const out = run(input, { over: 0, floor: 1, highPassHz: 0 })
    // One window minus one hop of algorithmic latency.
    const lag = 1024 - HOP
    let worst = 0
    for (let i = 3000; i < n - HOP; i++) worst = Math.max(worst, Math.abs(out[i] - input[i - lag]))
    expect(worst).toBeLessThan(0.01)
  })

  it('drops steady broadband noise close to the residual floor', () => {
    const n = SR // one second
    const rnd = noiseGen(7)
    const input = new Float32Array(n)
    for (let i = 0; i < n; i++) input[i] = 0.05 * rnd()
    const out = run(input)
    const before = rms(input, n / 2, n)
    const after = rms(out, n / 2, n)
    // Measured ~0.17 of the input, i.e. within a whisker of the 0.12 floor the
    // filter deliberately leaves behind. The bound is loose enough to survive
    // retuning and tight enough that a filter which stopped working fails it.
    expect(after).toBeLessThan(before * 0.3)
  })

  it('picks up a noise source that starts mid-call', () => {
    // A fan or an air conditioner switched on halfway through a standup. The
    // estimator is a sliding minimum, so it cannot react instantly — but it must
    // react. This is the one assertion covering the block rotation, without
    // which the filter would suppress only noise that was there at the start.
    const n = SR * 4
    const rnd = noiseGen(5)
    const input = new Float32Array(n)
    for (let i = 0; i < n; i++) {
      // Quiet room tone throughout, a loud fan from one second in.
      input[i] = 0.002 * rnd() + (i > SR ? 0.05 * rnd() : 0)
    }
    const out = run(input)
    const early = rms(out, SR + 2000, SR + 10000)
    const late = rms(out, n - 10000, n)
    expect(late).toBeLessThan(early * 0.5)
  })

  it('keeps speech-band bursts while gating the gaps between them', () => {
    // The shape that actually matters: a tone burst standing in constant hiss,
    // the way a sentence stands in room noise. The burst must survive and the
    // gap must go quiet — a filter that only did one of the two would pass the
    // test above and still be useless on a call.
    const n = SR
    const rnd = noiseGen(11)
    const input = new Float32Array(n)
    const on = (i) => Math.floor(i / (SR * 0.35)) % 2 === 0
    for (let i = 0; i < n; i++) {
      const tone = on(i) ? 0.3 * Math.sin((2 * Math.PI * 500 * i) / SR) : 0
      input[i] = tone + 0.03 * rnd()
    }
    const out = run(input)
    // Sampled inside a burst and inside a gap, both far enough in that the noise
    // estimate has settled and clear of the transition.
    const burst = rms(out, Math.round(SR * 0.75), Math.round(SR * 0.95))
    const gap = rms(out, Math.round(SR * 0.45), Math.round(SR * 0.65))
    expect(burst).toBeGreaterThan(0.12)
    expect(gap).toBeLessThan(burst * 0.25)
  })

  it('removes rumble below the voice band even with the gate wide open', () => {
    // Desk thumps and ventilation live under 80 Hz and are too erratic for the
    // gate to catch, so they are cut outright. Gate disabled here so the only
    // thing that can pass this is the high-pass itself.
    const n = SR / 2
    const input = new Float32Array(n)
    for (let i = 0; i < n; i++) input[i] = 0.3 * Math.sin((2 * Math.PI * 25 * i) / SR)
    const out = run(input, { over: 0, floor: 1 })
    expect(rms(out, n / 2, n)).toBeLessThan(rms(input, n / 2, n) * 0.3)
  })

  it('emits finite samples for a silent input', () => {
    // The gain is a ratio with the magnitude on the bottom; digital silence is
    // the input that turns a missing guard into NaN across the whole call.
    const out = run(new Float32Array(4096))
    expect(out.every((v) => Number.isFinite(v))).toBe(true)
    expect(rms(out)).toBe(0)
  })
})

describe('denoise worklet source', () => {
  it('carries a denoiser that is self-contained outside module scope', () => {
    // The worklet realm has no imports and no module scope: the DSP gets there
    // as text. If `createDenoiser` ever closes over something at module level it
    // will keep working here and break only in a real browser, in the worklet,
    // where nothing is watching. So rebuild it from its own source and demand
    // identical output.
    const rebuilt = new Function(`${denoiseSource()} return createDenoiser`)()
    expect(typeof rebuilt).toBe('function')
    const rnd = noiseGen(3)
    const input = new Float32Array(4096)
    for (let i = 0; i < input.length; i++) {
      input[i] = 0.2 * Math.sin((2 * Math.PI * 700 * i) / SR) + 0.04 * rnd()
    }
    const a = run(input)
    const original = createDenoiser
    const b = (() => {
      const d = rebuilt(SR, undefined)
      const out = new Float32Array(input.length)
      const block = new Float32Array(HOP)
      for (let i = 0; i + HOP <= input.length; i += HOP) {
        d.process(input.subarray(i, i + HOP), block)
        out.set(block, i)
      }
      return out
    })()
    expect(original).toBeTypeOf('function')
    expect(Array.from(b)).toEqual(Array.from(a))
  })

  it('registers the processor under the name the filter asks for', () => {
    const src = denoiseWorkletSource()
    expect(src).toContain(`registerProcessor(${JSON.stringify(DENOISE_NAME)}`)
    expect(src).toContain('extends AudioWorkletProcessor')
  })
})

describe('createNoiseFilter', () => {
  let ctx
  let processed

  beforeEach(() => {
    processed = { kind: 'audio', id: 'processed' }
    globalThis.MediaStream = class {
      constructor(tracks) {
        this.tracks = tracks
      }
      getAudioTracks() {
        return this.tracks
      }
    }
    globalThis.AudioWorkletNode = class {
      constructor(context, name, opts) {
        this.name = name
        this.opts = opts
        this.port = { postMessage: vi.fn() }
      }
      connect() {}
      disconnect() {}
    }
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:denoise')
    ctx = {
      audioWorklet: { addModule: vi.fn(async () => {}) },
      createMediaStreamSource: vi.fn(() => ({ connect: vi.fn(), disconnect: vi.fn() })),
      createMediaStreamDestination: vi.fn(() => ({
        stream: { getAudioTracks: () => [processed] },
      })),
    }
  })

  it('exposes the processed track after init and drops it on destroy', async () => {
    const f = createNoiseFilter()
    expect(f.name).toBe(DENOISE_NAME)
    await f.init({ audioContext: ctx, track: { id: 'raw' } })
    expect(ctx.audioWorklet.addModule).toHaveBeenCalled()
    // The track the SFU will receive is the destination's, not the microphone's.
    expect(f.processedTrack).toBe(processed)
    await f.destroy()
    expect(f.processedTrack).toBeUndefined()
  })

  it('rebuilds the graph on restart rather than reusing a dead source', async () => {
    // A MediaStreamSource is bound to one track for its life, so a device switch
    // has to build a new one — reusing it would leave the call publishing the
    // previous microphone.
    const f = createNoiseFilter()
    await f.init({ audioContext: ctx, track: { id: 'raw' } })
    await f.restart({ audioContext: ctx, track: { id: 'other' } })
    expect(ctx.createMediaStreamSource).toHaveBeenCalledTimes(2)
    expect(ctx.createMediaStreamSource.mock.calls[1][0].getAudioTracks()[0]).toEqual({
      id: 'other',
    })
  })

  it('refuses to start without an audio context instead of failing silently', async () => {
    const f = createNoiseFilter()
    await expect(f.init({ track: { id: 'raw' } })).rejects.toThrow(/audio context/i)
  })

  it('reports availability from AudioWorklet support', () => {
    expect(denoiseSupported()).toBe(true)
    const saved = globalThis.AudioWorkletNode
    delete globalThis.AudioWorkletNode
    expect(denoiseSupported()).toBe(false)
    globalThis.AudioWorkletNode = saved
  })
})

// ── transport wiring ──────────────────────────────────────────────────────

const lk = vi.hoisted(() => {
  const RoomEvent = { Disconnected: 'disconnected', MediaDevicesChanged: 'mediaDevicesChanged' }
  const state = { rooms: [] }

  function micTrack() {
    return {
      mediaStreamTrack: { id: 'mic' },
      setProcessor: vi.fn(async () => {}),
      stopProcessor: vi.fn(async () => {}),
      setAudioContext: vi.fn(),
    }
  }

  class Room {
    constructor(options) {
      this.options = options
      this.handlers = {}
      this.remoteParticipants = new Map()
      this.canPlaybackAudio = true
      this.mic = null
      const self = {
        identity: 'me',
        sid: 'sid-me',
        name: 'Я',
        isMicrophoneEnabled: false,
        isCameraEnabled: false,
        getTrackPublication: (source) =>
          source === 'microphone' && this.mic ? { track: this.mic } : undefined,
        setMicrophoneEnabled: vi.fn(async (on) => {
          self.isMicrophoneEnabled = on
          this.mic = on ? this.mic || micTrack() : this.mic
        }),
        setCameraEnabled: vi.fn(async () => {}),
      }
      this.localParticipant = self
      state.rooms.push(this)
    }
    on(event, fn) {
      ;(this.handlers[event] ||= []).push(fn)
      return this
    }
    async connect() {}
    async disconnect() {}
    async switchActiveDevice() {}
    static async getLocalDevices() {
      return []
    }
  }
  return { RoomEvent, Room, state, micTrack }
})

vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))

// Hoisted with the mock factory: a plain const would still be in its temporal
// dead zone when the hoisted `vi.mock` runs.
const api = vi.hoisted(() => ({ token: vi.fn() }))
vi.mock('@/api', () => ({ conferences: api, getAccessToken: () => null, apiBaseURL: () => '/api' }))

import { useConfTransport, DENOISE_KEY } from '@/composables/useConfTransport'

describe('noise suppression in the transport', () => {
  beforeEach(() => {
    lk.state.rooms.length = 0
    api.token.mockResolvedValue({ data: { url: 'wss://x/livekit', token: 't' } })
    localStorage.clear()
    globalThis.AudioWorkletNode = class {}
    globalThis.MediaStream = class {}
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:denoise')
    // jsdom has no navigator.mediaDevices, which the transport reads as an
    // insecure context and refuses to join at all.
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn() },
      configurable: true,
      writable: true,
    })
  })
  afterEach(() => {
    delete globalThis.AudioWorkletNode
    Object.defineProperty(navigator, 'mediaDevices', {
      value: undefined,
      configurable: true,
      writable: true,
    })
  })

  async function joined() {
    const t = useConfTransport()
    await t.join('conf-1')
    await flushPromises()
    return t
  }

  it('leaves the microphone untouched while the toggle is off', async () => {
    const t = await joined()
    expect(t.denoise.value).toBe(false)
    expect(lk.state.rooms[0].mic.setProcessor).not.toHaveBeenCalled()
  })

  it('attaches and detaches the filter, remembering the choice', async () => {
    const t = await joined()
    await t.setDenoise(true)
    const mic = lk.state.rooms[0].mic
    expect(mic.setProcessor).toHaveBeenCalledTimes(1)
    expect(mic.setProcessor.mock.calls[0][0].name).toBe(DENOISE_NAME)
    expect(localStorage.getItem(DENOISE_KEY)).toBe('1')

    await t.setDenoise(false)
    expect(mic.stopProcessor).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem(DENOISE_KEY)).toBe('0')
  })

  it('turns itself on at join when it was on last time', async () => {
    localStorage.setItem(DENOISE_KEY, '1')
    await joined()
    expect(lk.state.rooms[0].mic.setProcessor).toHaveBeenCalledTimes(1)
  })

  it('supplies an audio context when the SDK has not set one', async () => {
    // livekit-client throws rather than attaching a processor to a track with no
    // AudioContext, and whether it has set one depends on the autoplay policy.
    // Losing noise suppression to that would be an invisible, intermittent bug.
    globalThis.AudioContext = class {
      async close() {}
    }
    const t = await joined()
    const mic = lk.state.rooms[0].mic
    let first = true
    mic.setProcessor.mockImplementation(async () => {
      if (first) {
        first = false
        throw new Error(
          'Audio context needs to be set on LocalAudioTrack in order to enable processors',
        )
      }
    })
    await t.setDenoise(true)
    expect(mic.setAudioContext).toHaveBeenCalled()
    expect(mic.setProcessor).toHaveBeenCalledTimes(2)
    expect(t.denoise.value).toBe(true)
    delete globalThis.AudioContext
  })

  it('falls back to off — and says so — when the filter will not start', async () => {
    const t = await joined()
    const mic = lk.state.rooms[0].mic
    mic.setProcessor.mockRejectedValue(new Error('worklet blew up'))
    await t.setDenoise(true)
    expect(t.denoise.value).toBe(false)
    expect(localStorage.getItem(DENOISE_KEY)).toBe('0')
  })

  it('applies the filter to a microphone published after joining muted', async () => {
    localStorage.setItem(DENOISE_KEY, '1')
    const t = useConfTransport()
    await t.join('conf-1', { mic: false })
    await flushPromises()
    expect(lk.state.rooms[0].mic).toBe(null)
    await t.setMic(true)
    await flushPromises()
    expect(lk.state.rooms[0].mic.setProcessor).toHaveBeenCalledTimes(1)
  })
})

// ── the control ───────────────────────────────────────────────────────────

import DeviceMenu from '@/components/conference/DeviceMenu.vue'

function menu(props) {
  return mount(DeviceMenu, {
    props: {
      devices: { audioinput: [{ id: 'm1', label: 'Гарнитура' }], videoinput: [], audiooutput: [] },
      selected: { audioinput: 'm1', videoinput: '', audiooutput: '' },
      ...props,
    },
  })
}
// Naive registers its components under the bare name; NDropdown is the export
// alias, not the component's own `name`.
const dropdown = (w) => w.findComponent({ name: 'Dropdown' })
const denoiseEntry = (w) =>
  dropdown(w)
    .props('options')
    .flatMap((o) => o.children || [])
    .find((c) => c.key === 'denoise|toggle')

describe('DeviceMenu — noise suppression entry', () => {
  it('offers the toggle only where the browser can run it', () => {
    // An AudioWorklet-less browser gets no control at all: a switch that flips
    // and changes nothing is worse than an absent one.
    const off = menu({ denoiseAvailable: false })
    expect(denoiseEntry(off)).toBeUndefined()
    off.unmount()
    const on = menu({ denoiseAvailable: true })
    expect(denoiseEntry(on)).toBeDefined()
    on.unmount()
  })

  it('marks the entry the same way a chosen device is marked', () => {
    // Accent checkmark via the NIcon `color` PROP (var(--t-primary)), like the
    // board dropdowns — not a '●' in the text nor a colourless icon (#2891).
    const w = menu({ denoiseAvailable: true, denoise: true })
    expect(denoiseEntry(w).icon().props.color).toBe('var(--t-primary)')
    w.unmount()
    const w2 = menu({ denoiseAvailable: true, denoise: false })
    expect(denoiseEntry(w2).icon).toBeUndefined()
    w2.unmount()
  })

  it('emits a toggle instead of trying to switch to a device called "toggle"', async () => {
    const w = menu({ denoiseAvailable: true })
    dropdown(w).vm.$emit('select', 'denoise|toggle')
    await nextTick()
    expect(w.emitted('toggle-denoise')).toHaveLength(1)
    expect(w.emitted('select')).toBeUndefined()
    w.unmount()
  })

  it('still switches devices', async () => {
    const w = menu({ denoiseAvailable: true })
    dropdown(w).vm.$emit('select', 'audioinput|m1')
    await nextTick()
    expect(w.emitted('select')[0]).toEqual(['audioinput', 'm1'])
    expect(w.emitted('toggle-denoise')).toBeUndefined()
    w.unmount()
  })
})
