import { test, expect } from '../fixtures.js'
import { t } from '../i18n.js'

// The phone's tour (#3000): «Ещё → Обучение» opens the phone's «Основы» as a
// bottom sheet standing on the bottom navigation — the tabs it talks about
// stay in sight — with «Назад» and «Прервать?» like on a desktop.

test('«Обучение» в «Ещё» — тур нижним листом над навигацией', async ({ page }) => {
  await page.goto('/more')
  await page.getByTestId('more-tour').click()

  const sheet = page.getByTestId('tour-sheet')
  await expect(sheet).toBeVisible({ timeout: 10000 })
  const pop = sheet.getByTestId('tour-pop')
  await expect(pop).toContainText(t('tour.steps.m-nav.title'))

  // The sheet ends where the navigation begins (once it has slid in).
  const bar = await page.getByTestId('mobile-tabbar').boundingBox()
  await expect
    .poll(async () => {
      const box = await sheet.boundingBox()
      return Math.round(box.y + box.height) - Math.round(bar.y)
    })
    .toBeLessThanOrEqual(1)

  await page.getByTestId('tour-next').click()
  await expect(pop).toContainText(t('tour.steps.m-board.title'))
  await page.getByTestId('tour-back').click()
  await expect(pop).toContainText(t('tour.steps.m-nav.title'))

  await page.getByTestId('tour-skip').click()
  await expect(pop).toHaveAttribute('data-kind', 'confirm')
  await page.getByTestId('tour-finish').click()
  await expect(sheet).toHaveCount(0)
})
