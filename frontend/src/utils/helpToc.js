// Which table-of-contents entry is "current" while reading a help article (#2811).
//
// Kept as a pure function of geometry — the caller measures the scrolling column
// and passes numbers in. jsdom has no layout, so anything that read offsets off
// real elements here would be untestable; and the rule below is the part worth
// testing, not the DOM plumbing around it.

// The band at the top of the reading column that counts as "current": a heading
// is the one being read once it has scrolled into the upper third.
export const TOC_BAND = 0.3

/**
 * The heading the reader is currently on.
 *
 * Normally that is the last heading above the top band. At the very bottom of
 * the article that rule breaks down: the remaining headings can no longer reach
 * the band — there is nothing left to scroll — so the highlight sticks on a
 * section the reader has already passed, which is the bug on the screenshots.
 * When the column is scrolled to its end we therefore take the last heading that
 * is visible at all: everything below the band is on screen and finished.
 *
 * @param {Array<{id: string, top: number}>} headings in reading order, `top` being
 *   the heading's offset inside the scrolling column
 * @param {object} geom scroll geometry of that column
 * @param {number} geom.scrollTop current scroll offset
 * @param {number} geom.clientHeight visible height
 * @param {number} geom.scrollHeight total scrollable height
 * @param {number} [geom.band] fraction of the height that counts as the top band
 * @returns {string} heading id, or '' above the first heading / with no headings
 */
export function pickActive(headings, { scrollTop, clientHeight, scrollHeight, band = TOC_BAND }) {
  const list = Array.isArray(headings) ? headings : []
  if (!list.length) return ''

  // Sub-pixel scroll offsets mean the end of the column is never hit exactly.
  const atEnd = scrollTop + clientHeight >= scrollHeight - 1
  const line = atEnd ? scrollTop + clientHeight : scrollTop + clientHeight * band

  let active = ''
  for (const h of list) {
    if (h.top > line) break
    active = h.id
  }
  return active
}

/**
 * Measures the headings of an article inside its scrolling column.
 *
 * Offsets are taken relative to the column's own scroll origin rather than the
 * viewport, so the numbers stay valid while the reader scrolls and only need
 * re-taking when the content itself moves (a new article, images loading in).
 *
 * @param {Element|null} root the scrolling column
 * @param {string} selector CSS selector for the headings inside it
 * @returns {Array<{id: string, top: number}>} headings in document order
 */
export function measureHeadings(root, selector) {
  if (!root) return []
  const base = root.getBoundingClientRect().top - root.scrollTop
  return [...root.querySelectorAll(selector)]
    .filter((el) => el.id)
    .map((el) => ({ id: el.id, top: el.getBoundingClientRect().top - base }))
}
