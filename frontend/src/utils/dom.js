// scrollParent — the nearest ancestor of `el` that actually scrolls (overflow-y
// auto/scroll and taller content than box). Returns null if none — the caller
// then has nothing to scroll. Used to scroll the comments pane to the latest
// message and to keep the modal from jumping during textarea auto-grow.
export function scrollParent(el) {
  let p = el?.parentElement
  while (p) {
    const oy = getComputedStyle(p).overflowY
    if ((oy === 'auto' || oy === 'scroll') && p.scrollHeight > p.clientHeight) return p
    p = p.parentElement
  }
  return null
}
