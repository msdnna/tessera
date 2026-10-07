import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect, openBoard } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// Third round of #2893, point 3: "длинные формулировки в Ганте, и переносы в
// плашках «просрочено» и «без дат» из-за них".
//
// v2 (#2959): the chart's own toolbar row is gone — its controls live in the
// board's sub-toolbar (ChartControls), the counters and the link hint in the
// board's «Вид доски» panel, section «Шкала» (#2986; a bottom sheet on a phone).
// The breakage the report showed can still happen there,
// and the width gates still cannot see it: squeezing costs the page no width. So
// this asserts on the boxes that would break — the controls' height and each
// chip's line count — and on the hint staying off a touch screen.

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

  const controls = page.getByTestId('chart-controls')
  await expect(controls).toBeVisible({ timeout: 10000 })
  await page.screenshot({ path: resolve(SHOTS, 'gantt-toolbar.png'), fullPage: false })

  // Gate 1: the controls stay one row of 34px controls, not a wrapped column.
  const h = await controls.evaluate((el) => el.getBoundingClientRect().height)
  expect(h, `контролы Ганта заняли ${Math.round(h)}px — ряд переносится`).toBeLessThan(50)

  // Gate 2: each counter chip in the view panel is one line. The panel opens from
  // the same header menu; its «Шкала» section is a tap away.
  await page
    .locator('.tb-right .n-button')
    .filter({ has: page.locator('.n-icon') })
    .first()
    .tap()
  await page.locator('.n-dropdown-option', { hasText: 'Вид доски' }).first().click()
  await expect(page.getByTestId('board-customize')).toBeVisible({ timeout: 10000 })
  await page.getByTestId('customize-sec-scale').tap()
  const chips = page.locator('.cp-counter')
  await expect(chips.first()).toBeVisible()
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

  // The link hint is dropped rather than shown: it describes dragging a knob
  // that `:hover` reveals at 14px, an affordance a touch screen does not have.
  await expect(page.locator('.cp-scale .cp-note', { hasText: /Тяните|Drag/ })).toHaveCount(0)
  await page.keyboard.press('Escape')

  await expectFitsWidth(page, 'Гант на телефоне')
})
