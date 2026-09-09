import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect, openBoard } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// Third round of #2893, point 1: "сжатие колонок не работает — колонки остаются
// полноразмерными, но без контента".
//
// The report is precise and the cause is in this branch's own first round. The
// mobile override there said: on a phone a collapsed column keeps its full width
// and the 44px strip is hidden, because "columns are one-per-screen anyway". But
// a collapsed column renders every card as a bare `.card-ph` spacer (KanbanBoard's
// item template checks `colCollapsedNow` first), so revealing `.drop` at full
// width produced exactly what the screenshot shows: a full-size column with
// nothing in it. Nothing about that override was checked by a test — the page
// does not get wider, so no width gate could see it.
//
// Hence two assertions, and both are needed: the column has to become a strip,
// AND the strip has to carry the column's identity. A "fix" that only narrowed
// the box would leave a blank 44px sliver, which is the same defect at a
// different size.

const here = dirname(fileURLToPath(import.meta.url))
const SHOTS = resolve(here, '../.auth/mobile-shots')
mkdirSync(SHOTS, { recursive: true })

async function settle(page) {
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(400)
}

test('мобильная доска: свёрнутая колонка становится полосой, а не пустой колонкой', async ({
  page,
  seed,
  backend,
}) => {
  // A card in the column being collapsed: an empty column collapses by itself
  // (the auto-empty rule), and the defect is precisely about what happens to the
  // cards — a column with none of them could hide it.
  const stamp = Date.now().toString(36)
  await backend.createTask(seed.columns[0].id, `Карточка ${stamp}`)

  await openBoard(page, seed.boardId)
  await settle(page)

  const col = page.locator('.col').filter({ hasText: seed.columns[0].name }).first()
  await expect(col).toBeVisible()
  const wide = await col.evaluate((el) => el.getBoundingClientRect().width)
  await expect(col.locator('.card-wrap').first()).toBeVisible()

  await col.locator('.col-collapse').tap()
  await page.waitForTimeout(400)
  await page.screenshot({ path: resolve(SHOTS, 'board-collapsed.png'), fullPage: false })

  // Gate 1: it actually collapses. The floor is generous (80px against the 44px
  // the strip is) so restyling the strip does not make this red, but it is far
  // under any full-width column: the columns stretch to the viewport on a phone,
  // so the broken state measured ~370px here.
  const narrow = await col.evaluate((el) => el.getBoundingClientRect().width)
  expect(
    narrow,
    `колонка не сжалась: ${Math.round(narrow)}px после сворачивания против ${Math.round(wide)}px до`,
  ).toBeLessThan(80)

  // Gate 2: the strip is what fills it. Without this a column narrowed to 44px of
  // hidden cards would pass gate 1 while showing the user a blank sliver — the
  // reported defect at a smaller size.
  const strip = col.locator('.col-strip')
  await expect(strip).toBeVisible()
  await expect(strip).toContainText(seed.columns[0].name)

  // And the cards are hidden rather than rendered as blank spacers: the
  // placeholders stay mounted (SortableJS needs the drop target), so assert on
  // what is actually painted.
  await expect(col.locator('.card-wrap').first()).toBeHidden()

  await expectFitsWidth(page, 'доска со свёрнутой колонкой')

  // Expanding again from the strip brings the cards back — the strip is the only
  // way back, so a strip that does not toggle would strand the column.
  await strip.tap()
  await page.waitForTimeout(400)
  await expect(col.locator('.card-wrap').first()).toBeVisible()
  const back = await col.evaluate((el) => el.getBoundingClientRect().width)
  expect(back, `колонка не развернулась обратно: ${Math.round(back)}px`).toBeGreaterThan(120)
})
