import { test as base, expect } from '@playwright/test'
import { readFileSync } from 'fs'
import { SEED_FILE } from './global-setup.js'
import { api, createTask, seedBoard } from './api.js'

// The seed written by global-setup: run id, credentials, access token and the
// workspace/project/board/columns chain every board spec starts from.
const seed = JSON.parse(readFileSync(SEED_FILE, 'utf-8'))

export const test = base.extend({
  // Sign the browser in before the test body runs. Specs that exercise the login
  // flow itself opt out with `test.use({ signedIn: false })`.
  signedIn: [true, { option: true }],

  seed: [seed, { option: true }],

  // Each test signs in for itself rather than replaying a `storageState` file.
  //
  // This is not a stylistic choice: `/api/auth/refresh` ROTATES the refresh
  // token, so a captured session cookie works exactly once. Sharing one snapshot
  // across the suite meant the first test spent the cookie and every later test
  // booted with a revoked one, got a 401 out of `bootstrap()` and silently landed
  // on /login — where the failure surfaced as "card not found", nowhere near the
  // real cause.
  //
  // The login goes through `fetch`, not the form, for speed; the form itself is
  // covered by auth.spec.js. Two things must end up in place, and the app needs
  // both (stores/auth.js):
  //   - the httpOnly refresh cookie, which only `X-Auth-Mode: cookie` sets;
  //   - `localStorage.tessera_user`, without which `bootstrap()` returns early
  //     and never even tries the cookie.
  page: async ({ page, signedIn }, use) => {
    if (signedIn) await signIn(page)
    await use(page)
  },

  // `backend` is the same API client global-setup used, pre-bound to the run's
  // token — for seeding preconditions and for poking the server "from the side"
  // in the realtime spec.
  // The empty pattern is mandatory, not sloppy: Playwright reads a fixture's
  // dependencies off the destructuring pattern and rejects a plain parameter
  // ("First argument must use the object destructuring pattern"). This fixture
  // depends on nothing, hence `{}` (see the eslint override for e2e/**).
  backend: async ({}, use) => {
    const t = seed.token
    await use({
      raw: api,
      get: (p) => api.get(p, t),
      post: (p, b) => api.post(p, b, t),
      patch: (p, b) => api.patch(p, b, t),
      del: (p) => api.del(p, t),
      createTask: (columnId, title, extra) => createTask(t, seed.boardId, columnId, title, extra),
      // A private board for specs that mutate structure (rename/delete columns),
      // so they can't strip the shared board out from under the others.
      freshBoard: (label) => seedBoard(t, `${seed.runId}-${label}`),
    })
  },

  // Opens the shared seeded board, ready to interact with.
  board: async ({ page }, use) => {
    await openBoard(page, seed.boardId)
    await use(page)
  },
})

export { expect }

// signIn puts a page's context into a signed-in state. Exported because the
// realtime spec opens a second context and has to sign that one in too.
export async function signIn(page, creds = seed.creds) {
  await page.goto('/login')
  const ok = await page.evaluate(async (c) => {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Auth-Mode': 'cookie' },
      body: JSON.stringify({ email: c.email, password: c.password }),
    })
    if (!res.ok) return `login ${res.status}: ${(await res.text()).slice(0, 200)}`
    const data = await res.json()
    localStorage.setItem('tessera_user', JSON.stringify(data.user))
    return true
  }, creds)
  if (ok !== true) throw new Error(`e2e sign-in failed: ${ok}`)
}

// openBoard navigates to a board and waits for it to settle.
//
// No workspace seeding here: BoardView switches the app to the board's own
// workspace while resolving it (#2721), so a deep link is self-sufficient. The
// harness used to seed `localStorage.tessera_ws` itself, which masked exactly
// that bug — deep-link.spec.js now covers the case instead.
//
// Both waits are load-bearing. `/board/<uuid>` is the legacy entry point:
// BoardView resolves it and then `router.replace`s to the canonical
// `/project/<slug>/board/<slug>`, which re-keys KanbanBoard and remounts it.
// Waiting for the canonical URL first also means a dropped session fails here,
// with an obvious "still on /login" URL, instead of much later as a missing card.
export async function openBoard(page, boardId) {
  await page.goto(`/board/${boardId}`)
  await page.waitForURL(/\/project\/[^/]+\/board\//, { timeout: 15000 })
  await expect(page.getByTestId('column').first()).toBeVisible()
}

// column() locates a board column by its visible name. Columns carry the name in
// a data attribute so the locator survives the header markup being restyled.
export function column(page, name) {
  return page.locator(`[data-testid="column"][data-column-name="${name}"]`)
}

export function cardsIn(page, columnName) {
  return column(page, columnName).getByTestId('task-card')
}

// dragCard moves a card onto another column with raw mouse events.
//
// vuedraggable/SortableJS listens to pointer events, not HTML5 drag-and-drop, so
// `locator.dragTo()` is unreliable here. It also arms on a 160ms delay
// (`:delay="160"` in KanbanBoard.vue), which is why the press is held before the
// first move, and it needs several intermediate moves — a single jump can land
// between the frames Sortable samples and drop nothing.
// Aim at the column's card list (`.drop`), NOT at the column box. An empty
// column collapses to little more than its header, so a fixed offset from the
// column's top lands *below* the column entirely and Sortable sees no target —
// the card then snaps back and the spec fails with a very unhelpful "card not in
// the target column".
export async function dragCard(page, card, targetColumn) {
  const from = await card.boundingBox()
  const dropZone = targetColumn.locator('.drop')
  await expect(dropZone).toBeVisible()
  const to = await dropZone.boundingBox()
  if (!from || !to) throw new Error('drag source or target is not visible')

  const sx = from.x + from.width / 2
  const sy = from.y + from.height / 2
  const tx = to.x + to.width / 2
  const ty = to.y + to.height / 2

  await page.mouse.move(sx, sy)
  await page.mouse.down()
  await page.waitForTimeout(250) // clear the :delay="160" drag threshold

  const steps = 12
  for (let i = 1; i <= steps; i++) {
    await page.mouse.move(sx + ((tx - sx) * i) / steps, sy + ((ty - sy) * i) / steps)
    await page.waitForTimeout(20)
  }
  // Settle on the target before releasing: Sortable places the ghost on the last
  // observed move, and dropping in the same tick as the final move can land the
  // card back where it started.
  await page.waitForTimeout(150)
  await page.mouse.up()
}
