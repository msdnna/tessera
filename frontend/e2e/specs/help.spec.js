import { test, expect } from '../fixtures.js'

// The help centre on the v2 language (#3107, PackageE «HelpWin»): F1 opens and
// closes it, «/» puts the cursor in its search, ↵ opens a hit, and the article
// asks «Статья помогла?» once.
test('справка: F1, поиск с клавиатуры, «Статья помогла?»', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('sidebar')).toBeVisible()

  await page.keyboard.press('F1')
  const win = page.locator('[data-help-center]')
  await expect(win).toBeVisible()
  await expect(page.locator('.h-article')).toBeVisible()

  // «/» from anywhere in the window → the search; ↵ opens the first hit.
  await page.locator('.hc-h1').click()
  await page.keyboard.press('/')
  await expect(page.locator('[data-help-search]')).toBeFocused()
  await page.keyboard.type('конференц')
  await expect(page.locator('[data-help-results] .h-result').first()).toBeVisible()
  await page.keyboard.press('Enter')
  await expect(page.locator('[data-help-results]')).toHaveCount(0)
  await expect(page.locator('.hc-h1')).not.toHaveText('')

  // One answer per article: after «Да» the question turns into thanks.
  await page.getByTestId('help-feedback-yes').click()
  await expect(page.getByTestId('help-feedback-thanks')).toBeVisible()
  await expect(page.getByTestId('help-feedback-yes')).toHaveCount(0)

  // F1 closes it again.
  await page.keyboard.press('F1')
  await expect(win).toHaveCount(0)
})
