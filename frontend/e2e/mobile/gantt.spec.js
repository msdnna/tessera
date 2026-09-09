import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect, openBoard } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// Third round of #2893, point 3: "длинные формулировки в Ганте, и переносы в
// плашках «просрочено» и «без дат» из-за них".
//
// Both halves are one flex row. `.tl-toolbar` is a nowrap row of four rigid
// items; on 393px the Gantt's hint ("Тяните от правого края задачи к другой,
// чтобы создать зависимость") has nowhere to go, so it wraps into a five-line
// column, and the squeeze pushes into the counters — a chip with a 20px radius
// and two lines of text renders as a circle, which is the "29 без дат" blob in
// the report.
//
// Neither is visible to the width gates: squeezing costs the page no width. So
// this asserts on the two boxes that actually break — the toolbar's height and
// each chip's line count.

const here = dirname(fileURLToPath(import.meta.url))
const SHOTS = resolve(here, '../.auth/mobile-shots')
mkdirSync(SHOTS, { recursive: true })

async function settle(page) {
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(400)
}

test('Гант на телефоне: подсказка не растёт в колонку, плашки в одну строку', async ({
  page,
  seed,
  backend,
}) => {
  // Overdue + undated tasks so BOTH chips exist: the counters are conditional,
  // and an empty `.tl-counters` would make the assertions below vacuous.
  const stamp = Date.now().toString(36)
  await backend.createTask(seed.columns[0].id, `Просрочка ${stamp}`, {
    due_date: '2020-01-01T00:00:00Z',
  })
  await backend.createTask(seed.columns[0].id, `Без дат ${stamp}`)

  await openBoard(page, seed.boardId)
  await settle(page)

  // The layout switcher on a phone lives in the header's "⋮" menu (Topbar renders
  // BoardMobileMenu only in its mobile branch). Naive's dropdown will not take a
  // data-testid — the option is reached by its label.
  await page
    .locator('.tb-right .n-button')
    .filter({ has: page.locator('.n-icon') })
    .first()
    .tap()
  await page.locator('.n-dropdown-option', { hasText: 'Гант' }).first().click()
  await page.waitForTimeout(700)

  const toolbar = page.locator('.tl-toolbar')
  await expect(toolbar).toBeVisible({ timeout: 10000 })
  await page.screenshot({ path: resolve(SHOTS, 'gantt-toolbar.png'), fullPage: false })

  // Gate 1: the toolbar stays a toolbar. The "Сегодня" button sets the row height
  // at ~30px, so two rows fit under 90px while the reported five-line column
  // measured well past 120. The chart itself starts right below it — every pixel
  // the toolbar takes is taken from the bars.
  const h = await toolbar.evaluate((el) => el.getBoundingClientRect().height)
  expect(h, `тулбар Ганта занял ${Math.round(h)}px — подсказка выросла в колонку`).toBeLessThan(90)

  // Gate 2: each chip is one line. Measured as height, not as text: a chip that
  // wraps is what turns into the circle in the screenshot, and its own box is the
  // only place that shows.
  const chips = page.locator('.tl-counter')
  const n = await chips.count()
  expect(n, 'плашек-счётчиков нет — проверять нечего').toBeGreaterThan(0)
  for (let i = 0; i < n; i++) {
    const chip = chips.nth(i)
    const box = await chip.evaluate((el) => ({
      h: el.getBoundingClientRect().height,
      text: el.textContent.trim(),
    }))
    expect(box.h, `плашка «${box.text}» переносится: ${Math.round(box.h)}px высотой`).toBeLessThan(
      30,
    )
  }

  // The hint is dropped rather than wrapped, and that is a decision worth
  // pinning: it describes dragging a link knob that `:hover` reveals at 14px, so
  // on a touch screen it points at an affordance that does not exist there.
  await expect(page.locator('.tl-hint')).toBeHidden()

  await expectFitsWidth(page, 'Гант на телефоне')
})
