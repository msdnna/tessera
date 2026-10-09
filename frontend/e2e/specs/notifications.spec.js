import { test, expect } from '../fixtures.js'

// The bell's feed on the v2 language (#3107, PackageE «NotifPop»): G N opens it,
// the filters narrow it, the gear leads to the notification settings.
test('колокольчик: G N открывает ленту, фильтры, переход в настройки', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('sidebar')).toBeVisible()

  await page.keyboard.press('g')
  await page.keyboard.press('n')
  const feed = page.getByTestId('notifications-feed')
  await expect(feed).toBeVisible()

  for (const f of ['mentions', 'assigned', 'gitlab', 'all']) {
    await page.getByTestId(`notifications-filter-${f}`).click()
    await expect(page.getByTestId(`notifications-filter-${f}`)).toHaveAttribute(
      'aria-selected',
      'true',
    )
  }

  await page.getByTestId('notifications-settings').click()
  await expect(page).toHaveURL(/\/settings\?s=notifications/)
  await expect(feed).toHaveCount(0)
})
