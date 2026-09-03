// Software noise suppression for the conference microphone (#2889).
//
// Why this exists at all, given the browser already has `noiseSuppression: true`
// (#2883): the browser filter is tuned to be safe and inaudible, and it leaves a
// fan, a keyboard and a room's hum comfortably audible on a standup call. The
// complaint that opened this task was exactly that.
//
// Why it is written here instead of installed:
//   - `@livekit/track-processors` is video only. It ships one dependency,
//     `@mediapipe/tasks-vision`, and contains no audio code whatsoever — the
//     package solves background blur, not noise.
//   - the noise filter LiveKit documents is `@livekit/krisp-noise-filter`, a
//     separate 12 MB package under livekit.io's terms of service, not an open
//     licence. It was ruled out for this task.
// What *is* free and open is the seam: `LocalAudioTrack.setProcessor` in
// `livekit-client` takes any object shaped like a TrackProcessor. So we keep the
// official pipeline and put our own filter in it.
//
// The filter is plain DSP — spectral gating with a per-bin adaptive noise floor.
// No neural network, no WASM blob, no new dependency, ~4 MFLOP/s of work. It is
// honestly good at *stationary* noise (fans, hum, hiss, air conditioning, street
// rumble) and only partly helps with impulsive noise (keyboard, a door). That is
// the well-known trade of this class of filter, and the reason the toggle exists
// rather than it being always on.

// FFT window and hop. The hop is one render quantum, so the worklet does exactly
// one frame per `process()` call and needs no ring buffer of its own. A 1024-point
// window at 48 kHz gives ~47 Hz bins — coarse enough to be cheap, fine enough to
// separate a voice harmonic from the noise floor between harmonics. Algorithmic
// latency is one window minus one hop: 896 samples, ~19 ms.
const FRAME = 1024
const HOP = 128

/** Name the worklet registers itself under, and the processor's `name`. */
export const DENOISE_NAME = 'tessera-denoise'

/**
 * Build the streaming denoiser.
 *
 * **This function must stay self-contained.** Its source text is shipped into the
 * AudioWorklet realm via `Function.prototype.toString()` (see
 * `denoiseSource`), where module scope does not exist — so it may not reference
 * anything outside its own body, imports included. `denoise.spec.js` guards that
 * by rebuilding it from its own source and comparing behaviour.
 *
 * @param {number} sampleRate stream rate in Hz, used for the high-pass cutoff
 * @param {object} [options] `{floor, over, window, powerSmooth, smooth, highPassHz}`
 * @returns {{frame:number, hop:number, process:Function}}
 */
export function createDenoiser(sampleRate, options) {
  const N = 1024
  const HOP_N = 128
  const BINS = N / 2 + 1
  const o = options || {}
  const cfg = {
    // Residual gain on a bin judged to be noise. Gating all the way to zero is
    // what makes cheap noise removal sound like a broken phone line: the room
    // tone vanishing between words reads as a dropout. Leaving ~ -18 dB of the
    // original keeps the call sounding like a room.
    floor: o.floor === undefined ? 0.12 : o.floor,
    // How far above the tracked minimum a bin must sit to count as signal. It
    // absorbs two things at once: the bias of a minimum (a minimum of a noisy
    // power estimate lands well below that noise's mean) and the margin proper.
    // Setting it to 0 disables the gate entirely, which is how the transparency
    // of the analysis/synthesis pair is tested.
    over: o.over === undefined ? 16 : o.over,
    // Frames the minimum is tracked over — about 1.4 s at 48 kHz, and the one
    // number the whole estimator rests on. It must comfortably exceed the
    // longest sound a person holds (a drawn-out vowel is well under a second),
    // or a sustained voice would sink into the floor estimate and be gated as
    // noise; and it must stay short enough to follow a fan being switched on.
    window: o.window === undefined ? 512 : o.window,
    // Light averaging of the per-bin power before any decision is taken. A raw
    // magnitude spectrum of noise scatters wildly bin to bin, and a gate reading
    // it directly chatters.
    powerSmooth: o.powerSmooth === undefined ? 0.15 : o.powerSmooth,
    // Temporal smoothing of the gain. Without it the gate chatters on the
    // consonants and the result sounds like it is being chopped.
    smooth: o.smooth === undefined ? 0.5 : o.smooth,
    // Rumble below a voice: desk thumps, HVAC, traffic. Removed outright — the
    // gate is slow to catch it because it is not steady.
    highPassHz: o.highPassHz === undefined ? 80 : o.highPassHz,
  }

  // sqrt-Hann for both analysis and synthesis: their product is a Hann window,
  // which sums to a constant N/HOP/2 across the overlap, so overlap-add
  // reconstructs the signal exactly when nothing is modified.
  const win = new Float32Array(N)
  for (let i = 0; i < N; i++) win[i] = Math.sqrt(0.5 * (1 - Math.cos((2 * Math.PI * i) / N)))
  const norm = N / HOP_N / 2

  // Radix-2 tables, built once.
  let levels = 0
  while (1 << levels < N) levels++
  const rev = new Uint16Array(N)
  for (let i = 0; i < N; i++) {
    let r = 0
    for (let b = 0; b < levels; b++) r |= ((i >> b) & 1) << (levels - 1 - b)
    rev[i] = r
  }
  const cosT = new Float32Array(N / 2)
  const sinT = new Float32Array(N / 2)
  for (let i = 0; i < N / 2; i++) {
    cosT[i] = Math.cos((2 * Math.PI * i) / N)
    sinT[i] = Math.sin((2 * Math.PI * i) / N)
  }

  function fft(re, im, inverse) {
    for (let i = 0; i < N; i++) {
      const j = rev[i]
      if (j > i) {
        let t = re[i]
        re[i] = re[j]
        re[j] = t
        t = im[i]
        im[i] = im[j]
        im[j] = t
      }
    }
    const sign = inverse ? 1 : -1
    for (let size = 2; size <= N; size <<= 1) {
      const half = size >> 1
      const step = N / size
      for (let i = 0; i < N; i += size) {
        for (let j = i, k = 0; j < i + half; j++, k += step) {
          const wr = cosT[k]
          const wi = sign * sinT[k]
          const ar = re[j + half]
          const ai = im[j + half]
          const xr = ar * wr - ai * wi
          const xi = ar * wi + ai * wr
          re[j + half] = re[j] - xr
          im[j + half] = im[j] - xi
          re[j] += xr
          im[j] += xi
        }
      }
    }
    if (inverse) {
      for (let i = 0; i < N; i++) {
        re[i] /= N
        im[i] /= N
      }
    }
  }

  const hist = new Float32Array(N)
  const ola = new Float32Array(N)
  const re = new Float32Array(N)
  const im = new Float32Array(N)
  const gain = new Float32Array(BINS).fill(1)
  const raw = new Float32Array(BINS)
  // Smoothed power, and the minimum-statistics pair. Two buffers rather than a
  // ring of `window` frames: `minCur` collects the block being filled, `minPrev`
  // holds the one before it, and their minimum is a sliding window of between
  // one and two blocks. That is the standard trick — it costs two arrays instead
  // of five hundred and the extra slack in the window length is harmless.
  const power = new Float32Array(BINS)
  const minCur = new Float32Array(BINS).fill(Infinity)
  const minPrev = new Float32Array(BINS).fill(Infinity)
  const noise = new Float32Array(BINS)
  let blockAge = 0
  let primed = 0
  // Bins whose centre sits below the cutoff are zeroed outright.
  const hpBin = Math.max(1, Math.round((cfg.highPassHz * N) / (sampleRate || 48000)))

  /**
   * Filter one render quantum. `input` and `output` are both HOP samples long and
   * may be the same array. Output lags input by one window minus one hop.
   */
  function process(input, output) {
    hist.copyWithin(0, HOP_N)
    hist.set(input, N - HOP_N)
    for (let i = 0; i < N; i++) {
      re[i] = hist[i] * win[i]
      im[i] = 0
    }
    fft(re, im, false)

    // Until the analysis window has been filled with real input it is still part
    // zero padding, so its power is far below the stream's. A minimum tracker
    // that saw those frames would latch onto near-silence and hold the gate open
    // for the rest of the block — the whole filter would do nothing, quietly.
    const priming = primed < N / HOP_N
    if (priming) primed++

    for (let k = 0; k < BINS; k++) {
      const p = re[k] * re[k] + im[k] * im[k]
      const ps = power[k] + (p - power[k]) * cfg.powerSmooth
      power[k] = ps
      if (priming || ps < minCur[k]) minCur[k] = ps
      const nf = minCur[k] < minPrev[k] ? minCur[k] : minPrev[k]
      noise[k] = nf
      // Wiener gain in the power domain: what fraction of this bin's energy is
      // not accounted for by the noise floor. Taking the square root converts it
      // back to an amplitude gain.
      const excess = ps - nf * cfg.over
      let g = ps > 1e-20 && excess > 0 ? Math.sqrt(excess / ps) : 0
      if (!(g > cfg.floor)) g = cfg.floor
      else if (g > 1) g = 1
      raw[k] = g
    }
    if (++blockAge >= cfg.window) {
      blockAge = 0
      minPrev.set(minCur)
      // The new block starts from the current frame, not from infinity, so the
      // estimate never spends a block being unusable.
      minCur.set(power)
    }
    for (let k = 0; k < BINS; k++) {
      // Blur across neighbouring bins before applying. A gate that keeps single
      // isolated bins produces "musical noise" — the warbling tones that make
      // naive spectral subtraction instantly recognisable.
      const a = raw[k > 0 ? k - 1 : 0]
      const c = raw[k < BINS - 1 ? k + 1 : BINS - 1]
      const blurred = (a + 2 * raw[k] + c) / 4
      const g = gain[k] * cfg.smooth + blurred * (1 - cfg.smooth)
      gain[k] = g
      const applied = k < hpBin ? 0 : g
      re[k] *= applied
      im[k] *= applied
      // The spectrum of a real signal is conjugate-symmetric, and the inverse
      // transform only stays real if the mirror bin gets the same gain.
      if (k > 0 && k < N / 2) {
        re[N - k] *= applied
        im[N - k] *= applied
      }
    }

    fft(re, im, true)
    ola.copyWithin(0, HOP_N)
    ola.fill(0, N - HOP_N)
    for (let i = 0; i < N; i++) ola[i] += re[i] * win[i]
    for (let i = 0; i < HOP_N; i++) output[i] = ola[i] / norm
  }

  return { frame: N, hop: HOP_N, process }
}

/**
 * The denoiser's own source text, as an assignment the worklet realm can eval.
 *
 * Stringifying the live function rather than keeping a second copy of the DSP in
 * a `public/` file is what keeps one source of truth: the code the unit tests
 * exercise is literally the code that runs in the worklet. Minification renames
 * things, but it renames them inside this same text, so it stays consistent.
 */
export function denoiseSource() {
  return `const createDenoiser = ${createDenoiser.toString()};`
}

/** Full worklet module: the DSP plus the AudioWorkletProcessor that drives it. */
export function denoiseWorkletSource() {
  return `${denoiseSource()}
class TesseraDenoiseProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    this.denoiser = createDenoiser(sampleRate, (options && options.processorOptions) || {});
    this.enabled = true;
    this.port.onmessage = (e) => {
      if (e.data && typeof e.data.enabled === 'boolean') this.enabled = e.data.enabled;
    };
  }
  process(inputs, outputs) {
    const input = inputs[0];
    const output = outputs[0];
    if (!output || !output.length) return true;
    // No input yet (the source has not started) — emit silence rather than
    // returning false, which would retire the node for the rest of the call.
    if (!input || !input.length || !input[0] || !input[0].length) {
      for (let c = 0; c < output.length; c++) output[c].fill(0);
      return true;
    }
    // Mono: a conference microphone is one voice, and the SFU publishes mono
    // anyway. Extra input channels are averaged in rather than dropped.
    const src = input[0];
    const first = output[0];
    if (this.enabled) this.denoiser.process(src, first);
    else first.set(src);
    for (let c = 1; c < output.length; c++) output[c].set(first);
    return true;
  }
}
registerProcessor(${JSON.stringify(DENOISE_NAME)}, TesseraDenoiseProcessor);
`
}

// The blob URL is built once per page: `addModule` is idempotent per context but
// not free, and a fresh URL per join would leak one object URL per call.
let workletUrl = ''
function moduleUrl() {
  if (!workletUrl) {
    workletUrl = URL.createObjectURL(
      new Blob([denoiseWorkletSource()], { type: 'application/javascript' }),
    )
  }
  return workletUrl
}

/** True when this browser can run the filter at all. */
export function denoiseSupported() {
  return (
    typeof AudioWorkletNode !== 'undefined' &&
    typeof URL !== 'undefined' &&
    typeof URL.createObjectURL === 'function'
  )
}

/**
 * A `livekit-client` TrackProcessor that runs {@link createDenoiser} over the
 * microphone before it reaches the SFU.
 *
 * Duck-typed on purpose: the shape is `{name, init, restart, destroy,
 * processedTrack}` and nothing here imports the SDK, so this file stays testable
 * without it and `useConfTransport` remains the only place that knows LiveKit.
 *
 * @param {object} [options] tuning passed through to the denoiser
 */
export function createNoiseFilter(options) {
  let source = null
  let node = null
  let dest = null

  async function teardown() {
    try {
      source?.disconnect()
      node?.disconnect()
    } catch {
      // Nodes belong to a context that is already gone; nothing to release.
    }
    source = null
    node = null
    dest = null
    filter.processedTrack = undefined
  }

  const filter = {
    name: DENOISE_NAME,
    processedTrack: undefined,
    async init(opts) {
      await teardown()
      const ctx = opts.audioContext
      if (!ctx) throw new Error('denoise: no audio context')
      await ctx.audioWorklet.addModule(moduleUrl())
      source = ctx.createMediaStreamSource(new MediaStream([opts.track]))
      node = new AudioWorkletNode(ctx, DENOISE_NAME, {
        numberOfInputs: 1,
        numberOfOutputs: 1,
        outputChannelCount: [1],
        processorOptions: options || {},
      })
      dest = ctx.createMediaStreamDestination()
      source.connect(node)
      node.connect(dest)
      filter.processedTrack = dest.stream.getAudioTracks()[0]
    },
    // Called when the underlying capture changes (device switch, unmute). The
    // graph is bound to one MediaStreamTrack for its life, so it is rebuilt.
    async restart(opts) {
      await filter.init(opts)
    },
    async destroy() {
      await teardown()
    },
  }
  return filter
}

export const DENOISE_FRAME = FRAME
export const DENOISE_HOP = HOP
