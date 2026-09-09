import { expect } from '@playwright/test'

// Layout probes for the mobile tier (#2893).
//
// The one hard gate is the *page*, not the element: `documentElement.scrollWidth`
// must not exceed its client width. Everything else this module collects is
// advisory and travels in the failure message / the report attachment.
//
// That split is deliberate. The obvious detector — "no element's bounding box may
// stick out past the viewport" — is wrong here, and measurably so: on every
// public screen `.auth-glow` (the decorative blur behind the auth card) sits at
// left 24 with right 510 in a 390px viewport, 120px past the edge, while the page
// itself does not scroll a pixel because an ancestor clips it. A suite that
// failed on rectangles would have been red on its first green build, and a suite
// that is red by default gets switched off. So rectangles only ever explain a
// failure the page-level gate already found.

// SCAN is stringified into the page: Playwright's evaluate() ships the function
// source, so it cannot close over anything from this module.
const SCAN = () => {
  const de = document.documentElement
  const vw = de.clientWidth

  // clipped(el) — true when some ancestor cannot let `el` widen the page.
  // `overflow-x` other than `visible` (hidden/clip/auto/scroll) all clip or
  // scroll internally; a fixed/sticky ancestor is out of the document flow.
  const clipped = (el) => {
    for (let p = el.parentElement; p && p !== de; p = p.parentElement) {
      const s = getComputedStyle(p)
      if (s.overflowX !== 'visible') return true
      if (s.position === 'fixed') return true
    }
    return false
  }

  const describe = (el) => {
    const raw = el.className
    const cls = (raw && raw.baseVal !== undefined ? raw.baseVal : raw || '').toString()
    const id = el.getAttribute('data-testid')
    return (
      el.tagName.toLowerCase() +
      (id ? `[${id}]` : '') +
      (cls ? '.' + cls.trim().split(/\s+/).join('.') : '')
    )
  }

  const wide = []
  const smallTaps = []
  const truncated = []
  const TAPPABLE =
    'button, a[href], input, select, textarea, [role="button"], [role="tab"], [role="switch"]'

  for (const el of document.querySelectorAll('body *')) {
    const b = el.getBoundingClientRect()
    if (b.width === 0 && b.height === 0) continue

    if ((b.right > vw + 1 || b.left < -1) && !clipped(el)) {
      wide.push({
        el: describe(el).slice(0, 90),
        left: Math.round(b.left),
        right: Math.round(b.right),
      })
    }

    if (el.matches(TAPPABLE) && b.width > 0 && b.height > 0 && b.height < 32) {
      smallTaps.push({
        el: describe(el).slice(0, 90),
        w: Math.round(b.width),
        h: Math.round(b.height),
      })
    }

    // The breakage the page-level gate structurally cannot see: content cut off
    // inside a box that clips WITHOUT offering a way to scroll to the rest
    // (`overflow-x: hidden|clip`). `auto`/`scroll` are exempt — the board's
    // column strip is meant to scroll sideways, and flagging it would bury the
    // real hits. Everything an ellipsis truncates on purpose is `<= 8px` over,
    // which is why the threshold is not 1px.
    const s = getComputedStyle(el)
    if (
      (s.overflowX === 'hidden' || s.overflowX === 'clip') &&
      el.scrollWidth > el.clientWidth + 8 &&
      el.clientWidth > 0
    ) {
      truncated.push({
        el: describe(el).slice(0, 90),
        scrollW: el.scrollWidth,
        clientW: el.clientWidth,
        over: el.scrollWidth - el.clientWidth,
      })
    }
  }

  // The pane, not the document, is the honest gate once you are signed in.
  // AppLayout pins the shell to `height: 100vh` and puts the page inside Naive's
  // `.n-layout-scroll-container`, which scrolls on its own — so a child 600px
  // wide inside a 393px viewport leaves `documentElement.scrollWidth` at exactly
  // 393 and the document gate never fires. That is not a hypothesis: planting
  // such a box is what the negative-control spec does, and the document gate
  // stayed green on it. The document check is still worth keeping for the public
  // screens (/login, /register, …), which have no shell and do scroll the page.
  const paneEl = document.querySelector('.page-slot--mobile .n-layout-scroll-container')
  let pane = null
  if (paneEl) {
    // Naming the culprit is the whole value of this gate: `clipped()` rejects
    // everything inside the pane (the pane scrolls, so it clips), which means the
    // `wide` list above is empty by construction on every signed-in screen. So
    // widen the search here against the PANE's right edge instead of the
    // viewport's, and keep only the outermost offenders — a 500px table drags
    // its every ancestor into the list otherwise.
    const edge = paneEl.getBoundingClientRect().left + paneEl.clientWidth
    const hits = []
    for (const el of paneEl.querySelectorAll('*')) {
      const b = el.getBoundingClientRect()
      if (b.width === 0 && b.height === 0) continue
      if (b.right <= edge + 1) continue
      if (hits.some((h) => h.node.contains(el))) continue // an ancestor already reported
      hits.push({
        node: el,
        el: describe(el).slice(0, 90),
        right: Math.round(b.right),
        w: Math.round(b.width),
      })
    }
    pane = {
      scrollW: paneEl.scrollWidth,
      clientW: paneEl.clientWidth,
      over: hits.slice(0, 10).map(({ el, right, w }) => ({ el, right, w })),
    }
  }

  return {
    vw,
    docScrollW: de.scrollWidth,
    bodyScrollW: document.body.scrollWidth,
    pane,
    wide: wide.slice(0, 15),
    wideCount: wide.length,
    smallTaps: smallTaps.slice(0, 15),
    smallTapCount: smallTaps.length,
    truncated: truncated.slice(0, 15),
    truncatedCount: truncated.length,
  }
}

export async function scanLayout(page) {
  return page.evaluate(SCAN)
}

// expectFitsWidth asserts the page does not scroll sideways, and — when it does —
// spends the failure message on the unclipped elements that plausibly caused it.
export async function expectFitsWidth(page, label) {
  const r = await scanLayout(page)
  const detail = r.wide.length
    ? '\nПодозреваемые (не клипаются предком):\n' +
      r.wide.map((w) => `  ${w.el}  left=${w.left} right=${w.right}`).join('\n')
    : '\nНи один элемент не выходит за вьюпорт — ширину тянет что-то из внутренних отступов/грида.'
  expect(
    r.docScrollW,
    `«${label}»: страница скроллится вбок — ${r.docScrollW}px при вьюпорте ${r.vw}px.${detail}`,
  ).toBeLessThanOrEqual(r.vw + 1)
  if (r.pane) {
    const who = r.pane.over.length
      ? '\nВылезает за правый край рабочей области:\n' +
        r.pane.over.map((o) => `  ${o.el}  right=${o.right} w=${o.w}`).join('\n')
      : ''
    expect(
      r.pane.scrollW,
      `«${label}»: рабочая область шире экрана — ${r.pane.scrollW}px при ${r.pane.clientW}px.${who}`,
    ).toBeLessThanOrEqual(r.pane.clientW + 1)
  }
  return r
}

// expectPaneFitsWidth is the same gate one level down: an overlay (modal, drawer)
// can sit inside a clipping ancestor, so the page stays honest while its own body
// is cut off. Checked against the pane's own scrollWidth vs clientWidth.
export async function expectPaneFitsWidth(locator, label) {
  const r = await locator.evaluate((el) => ({
    scrollW: el.scrollWidth,
    clientW: el.clientWidth,
  }))
  expect(
    r.scrollW,
    `«${label}»: содержимое панели шире неё самой — ${r.scrollW}px при ${r.clientW}px.`,
  ).toBeLessThanOrEqual(r.clientW + 1)
  return r
}
