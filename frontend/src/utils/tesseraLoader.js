// tessera-loader — фирменный анимированный лоадер (портирован из
// design/tessera-brand-v2/loader/tessera-loader.js в ESM-модуль).
// mountTesseraLoader(el, {size, loop}) → {destroy()}
//
// Геометрия кадров и тайминги — из loader-states.json (встроены ниже) и являются
// ОБЩИМ ИСТОЧНИКОМ ИСТИНЫ с Android-портом (ui/components/TesseraLoader.kt): любые
// правки чисел здесь должны зеркалиться там, иначе поведение платформ разъедется.
//
// Заливка задаётся параметром `paint` (любой SVG paint): по умолчанию
// `currentColor` (цвет берётся из CSS `color` контейнера), либо ссылка на SVG-
// градиент, напр. `url(#t-accent-grad-svg)` — тогда каждая плитка/глиф несёт
// фирменный диагональный акцент-градиент (паритет с Android accentGradient).
// Уважает prefers-reduced-motion (показывает статичный кадр `kanban`).

const S = {
  corner: {
    t: 1,
    p: [
      { x: 304.6, y: 127, w: 54.6, h: 54.6, rx: 16.7, op: 1 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
    ],
  },
  cover: {
    t: 0,
    p: [
      { x: 132, y: 132, w: 248, h: 248, rx: 58, op: 1 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
    ],
  },
  kanban: {
    t: 0,
    p: [
      { x: 96, y: 104, w: 140, h: 304, rx: 24, op: 1 },
      { x: 276, y: 104, w: 140, h: 200, rx: 24, op: 1 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
    ],
  },
  list: {
    t: 0,
    p: [
      { x: 156, y: 132, w: 276, h: 32, rx: 16, op: 1 },
      { x: 156, y: 240, w: 276, h: 32, rx: 16, op: 1 },
      { x: 156, y: 348, w: 276, h: 32, rx: 16, op: 1 },
      { x: 80, y: 130, w: 36, h: 36, rx: 10, op: 1 },
      { x: 80, y: 238, w: 36, h: 36, rx: 10, op: 1 },
      { x: 80, y: 346, w: 36, h: 36, rx: 10, op: 1 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
    ],
  },
  timeline: {
    t: 0,
    p: [
      { x: 200, y: 124, w: 224, h: 32, rx: 16, op: 1 },
      { x: 200, y: 240, w: 172, h: 32, rx: 16, op: 1 },
      { x: 200, y: 356, w: 200, h: 32, rx: 16, op: 1 },
      { x: 112, y: 112, w: 56, h: 56, rx: 28, op: 1 },
      { x: 112, y: 228, w: 56, h: 56, rx: 28, op: 1 },
      { x: 112, y: 344, w: 56, h: 56, rx: 28, op: 1 },
      { x: 124, y: 120, w: 32, h: 272, rx: 16, op: 1 },
    ],
  },
  gantt: {
    t: 0,
    p: [
      { x: 80, y: 112, w: 200, h: 48, rx: 26, op: 1 },
      { x: 168, y: 196, w: 210, h: 48, rx: 26, op: 1 },
      { x: 120, y: 280, w: 170, h: 48, rx: 26, op: 1 },
      { x: 236, y: 364, w: 184, h: 48, rx: 26, op: 1 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
    ],
  },
  matrix: {
    t: 0,
    p: [
      { x: 96, y: 96, w: 140, h: 140, rx: 20, op: 1 },
      { x: 276, y: 96, w: 140, h: 140, rx: 20, op: 1 },
      { x: 96, y: 276, w: 140, h: 140, rx: 20, op: 1 },
      { x: 276, y: 276, w: 140, h: 140, rx: 20, op: 1 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
      { x: 246, y: 246, w: 20, h: 20, rx: 10, op: 0 },
    ],
  },
}
const GLYPH = {
  d: 'M1101 9L1101 9Q1050 9 1013.500-2Q977-13 953-36Q929-59 917.500-94.500Q906-130 906-179L906-179L906-339L871-339Q842-340 830.500-356.500Q819-373 819-412L819-412Q819-448 833-465.500Q847-483 875-483L875-483L906-483L906-585Q906-605 909.500-621Q913-637 929-647.500Q945-658 981-658Q1017-658 1033-647Q1049-636 1052.500-619Q1056-602 1056-582L1056-582L1056-485L1123-487Q1142-487 1159-483.500Q1176-480 1187-464Q1198-448 1198-411L1198-411Q1198-377 1187.500-361Q1177-345 1160-340.500Q1143-336 1123-336L1123-336L1056-338L1056-185Q1056-169 1058.500-158.500Q1061-148 1066-143Q1071-138 1080-136Q1089-134 1102-134L1102-134Q1124-134 1141-130Q1158-126 1167.500-112Q1177-98 1177-65L1177-65Q1177-29 1166-13Q1155 3 1137.500 6Q1120 9 1101 9Z',
  s: 0.34,
  tx: -86.89,
  ty: 382,
}
const T = { unfold: 540, hold: 620, collapse: 480, turn: 780 }
// Views the tessera unfolds into. There is no intro; `corner` (the "t" logo with
// the tile) comes LAST — once per cycle, after matrix — so the brand mark flashes
// at the end of the loop instead of "playing" as a preamble before it.
const VIEWS = ['kanban', 'list', 'timeline', 'gantt', 'matrix', 'corner']

const seg = (a, b, ra, rb, dur) => ({ a, b, ra, rb, dur })
const LOOP = []
VIEWS.forEach((v) => {
  LOOP.push(seg(S.cover, S[v], 0, 0, T.unfold))
  LOOP.push(seg(S[v], S[v], 0, 0, T.hold))
  LOOP.push(seg(S[v], S.cover, 0, 0, T.collapse))
  LOOP.push(seg(S.cover, S.cover, 0, 90, T.turn))
})

const sum = (a) => a.reduce((s, x) => s + x.dur, 0)
const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2)
const lerp = (a, b, k) => a + (b - a) * k

function at(segs, t) {
  let acc = 0
  let i = 0
  while (i < segs.length - 1 && t > acc + segs[i].dur) {
    acc += segs[i].dur
    i++
  }
  const s = segs[i]
  const k = ease(Math.min(1, (t - acc) / s.dur))
  return {
    t: lerp(s.a.t, s.b.t, k),
    rot: lerp(s.ra, s.rb, k),
    p: s.a.p.map((pa, j) => {
      const pb = s.b.p[j]
      return {
        x: lerp(pa.x, pb.x, k),
        y: lerp(pa.y, pb.y, k),
        w: lerp(pa.w, pb.w, k),
        h: lerp(pa.h, pb.h, k),
        rx: lerp(pa.rx, pb.rx, k),
        op: lerp(pa.op, pb.op, k),
      }
    }),
  }
}

const NS = 'http://www.w3.org/2000/svg'

export function mountTesseraLoader(el, o = {}) {
  const size = o.size || 140
  const doLoop = o.loop !== false
  const paint = o.paint || 'currentColor'
  const svg = document.createElementNS(NS, 'svg')
  svg.setAttribute('viewBox', '0 0 512 512')
  svg.setAttribute('width', size)
  svg.setAttribute('height', size)
  svg.setAttribute('fill', paint)

  const gl = document.createElementNS(NS, 'g')
  gl.setAttribute('transform', `translate(${GLYPH.tx} ${GLYPH.ty}) scale(${GLYPH.s})`)
  const gp = document.createElementNS(NS, 'path')
  gp.setAttribute('d', GLYPH.d)
  gp.setAttribute('fill', paint)
  gl.appendChild(gp)
  svg.appendChild(gl)

  const spin = document.createElementNS(NS, 'g')
  svg.appendChild(spin)
  const rects = []
  for (let i = 0; i < 7; i++) {
    const q = document.createElementNS(NS, 'rect')
    q.setAttribute('fill', paint)
    spin.appendChild(q)
    rects.push(q)
  }
  el.appendChild(svg)

  const reduce =
    typeof window !== 'undefined' &&
    window.matchMedia &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches

  function draw(st) {
    gl.setAttribute('opacity', st.t)
    spin.setAttribute('transform', `rotate(${st.rot} 256 256)`)
    st.p.forEach((p, j) => {
      const q = rects[j]
      q.setAttribute('x', p.x)
      q.setAttribute('y', p.y)
      q.setAttribute('width', Math.max(0, p.w))
      q.setAttribute('height', Math.max(0, p.h))
      q.setAttribute('rx', p.rx)
      q.setAttribute('opacity', p.op)
    })
  }

  if (reduce) {
    draw({ t: 0, rot: 0, p: S.kanban.p })
    return {
      destroy() {
        if (svg.parentNode === el) el.removeChild(svg)
      },
    }
  }

  draw(at(LOOP, 0)) // первый кадр сразу, без ожидания rAF (стартуем с плитки-cover)
  const lTot = sum(LOOP)
  let raf
  let start = null
  function frame(ts) {
    if (start === null) start = ts
    const e = ts - start
    draw(doLoop ? at(LOOP, e % lTot) : at(LOOP, Math.min(e, lTot)))
    raf = requestAnimationFrame(frame)
  }
  raf = requestAnimationFrame(frame)
  return {
    destroy() {
      cancelAnimationFrame(raf)
      if (svg.parentNode === el) el.removeChild(svg)
    },
  }
}
