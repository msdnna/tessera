import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect, openBoard } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// Second round of #2893: the three defects the first inventory could not see.
//
// Both failures here are invisible to the width gate in layout.spec.js, for
// opposite reasons:
//   - the subtask hover card DOES widen the page, but only after a tap, and no
//     spec in the tier tapped anything on a card;
//   - the two-pane panels do not widen anything at all — the right pane shrinks
//     to ~30px and wraps one letter per line, which costs the page zero pixels.
// So each gets its own assertion on the box that actually breaks.

const here = dirname(fileURLToPath(import.meta.url))
const SHOTS = resolve(here, '../.auth/mobile-shots')
mkdirSync(SHOTS, { recursive: true })

async function settle(page) {
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(400)
}

test('мобильная доска: тап по подзадаче не уводит модалку за экран', async ({
  page,
  seed,
  backend,
}) => {
  const stamp = Date.now().toString(36)
  const parent = await backend.createTask(seed.columns[0].id, `Родитель ${stamp}`)
  const child = await backend.createTask(seed.columns[0].id, `Подзадача ${stamp}`)
  await backend.patch(`/tasks/${child.id}/parent`, { parent_id: parent.id })

  await openBoard(page, seed.boardId)
  await settle(page)

  const row = page.locator('.subrow', { hasText: `Подзадача ${stamp}` }).first()
  await expect(row).toBeVisible()

  // The tap, not a hover: on touch, `mouseover` fires and `mouseleave` never
  // does, so the hover card opens on the first tap and stays. It is placed to
  // the right of a row that already starts deep into a 393px screen, so the page
  // grows by ~200px — and mobile Chrome then sizes the modal's fixed container
  // against the widened document, which is why the modal itself lands off-centre
  // (the report's first two screenshots).
  await row.tap()
  await page.waitForTimeout(600)

  await page.screenshot({ path: resolve(SHOTS, 'subtask-open.png'), fullPage: false })

  // Gate 1: the page must not have widened.
  await expectFitsWidth(page, 'доска после тапа по подзадаче')

  // Gate 2: the modal the tap opened is on screen. Checked separately because it
  // is the symptom the report actually shows; without it a future change that
  // keeps the page honest but still mispositions the modal would pass.
  const modal = page.locator('.n-modal .task-modal, .n-modal').first()
  await expect(modal).toBeVisible()
  const box = await modal.boundingBox()
  const vw = await page.evaluate(() => document.documentElement.clientWidth)
  expect(box.x, `модалка уехала вправо: x=${Math.round(box.x)}`).toBeGreaterThan(-2)
  expect(
    box.x + box.width,
    `модалка вылезает за правый край: right=${Math.round(box.x + box.width)} при vw=${vw}`,
  ).toBeLessThanOrEqual(vw + 2)

  // Gate 3: no hover card left hanging over the board after the tap.
  await expect(page.locator('.n-popover .mini-card')).toHaveCount(0)
})

// Фоновые задачи sit behind the instance-admin gate, and the tier's account is
// only an admin when it is the FIRST account in the database (the backend
// promotes that one automatically). So this spec asserts on a stand whose
// database was just created — `make e2e-backend-up E2E_DB_URL=…/<fresh db>` —
// and skips otherwise, because a 403'd run says nothing about the layout. It
// must never silently pass: a skip is visible in the run summary, a green
// assertion on an unopened modal would not be.
// Round 3 rewrote what this asserts. Round 2 stacked the two panes; the report
// came back with "секции стали совсем запутанными" — a capped list scrolling
// above an equally long detail, both inside the modal's own scroller, with a
// hairline to say where one ended. So the phone now shows one pane at a time.
// The old assertions (a wide detail pane AND a short list, on the same screen)
// are mutually exclusive under that layout and had to go; the width floor they
// were protecting survives here, checked on the detail screen where it applies.
test('фоновые задачи на телефоне: список и деталь — два экрана, а не две панели', async ({
  page,
  backend,
}) => {
  const me = await backend.get('/auth/me')
  test.skip(!me.user?.is_admin, 'нужен админ: экран фоновых задач за админ-гейтом')

  await page.goto('/')
  await settle(page)
  // On a phone the tools row lives in the sidebar drawer, not in the header
  // (Sidebar.vue renders WorkspaceTools when `mobile`), so the burger comes
  // first — otherwise the button simply is not in the DOM.
  await page.locator('.topbar .menu-btn').tap()
  await page.getByTestId('jobs-button').tap()

  const list = page.getByTestId('jobs-list')
  const detail = page.getByTestId('jobs-detail')
  await expect(list).toBeVisible()
  await page.waitForTimeout(400)
  await page.screenshot({ path: resolve(SHOTS, 'jobs-list.png'), fullPage: false })

  // Arrival is the list, whole. The detail pane must not be on screen at all:
  // side by side it would be showing «Выберите задание», which is a prompt for a
  // second pane — and the modal auto-picks the first job on the desktop, so
  // without the mobile guard the user would land inside a job they never tapped.
  await expect(detail).toBeHidden()
  const vh = await page.evaluate(() => window.innerHeight)
  const listH = await list.evaluate((el) => el.getBoundingClientRect().height)
  expect(
    listH,
    `список заданий обрезан по высоте: ${Math.round(listH)}px при экране ${vh}px`,
  ).toBeGreaterThan(vh * 0.3)

  const rows = page.locator('.bj-row')
  expect(await rows.count(), 'нет ни одного фонового задания — проверять нечего').toBeGreaterThan(0)
  await rows.first().tap()
  await page.waitForTimeout(300)
  await page.screenshot({ path: resolve(SHOTS, 'jobs-detail.png'), fullPage: false })

  await expect(detail).toBeVisible()
  await expect(list).toBeHidden()

  // The floor from round 2: side by side, `flex: 0 0 320px` on the list left the
  // detail ~40px on a 393px screen — it did not overflow, it wrapped one letter
  // per line. On its own screen it gets the whole modal body.
  const w = await detail.evaluate((el) => el.getBoundingClientRect().width)
  expect(
    w,
    `панель детали сжата до ${Math.round(w)}px — текст переносится по букве`,
  ).toBeGreaterThan(240)

  // A screen you cannot leave is worse than a cramped pane: the modal's close
  // button drops the whole thing, not the detail.
  const back = page.locator('.bj-back')
  await expect(back).toBeVisible()
  await back.tap()
  await page.waitForTimeout(300)
  await expect(list).toBeVisible()
  await expect(detail).toBeHidden()
})
