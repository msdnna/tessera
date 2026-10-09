import { test, expect } from '../fixtures.js'

// Settings on the v2 layout (#3106): a column of sections with the open one in
// the address, the notification channel and rule editors as popover forms, the
// «⋯» menu's two-step deletion, and the schedule defaults that save themselves.
// Unit tests cover each piece in isolation; what only exists in a browser is
// the round trip — the address, the server writes, the second press.

test('настройки: раздел в адресе, G «,» открывает, ↑↓ идут по разделам', async ({ page }) => {
  // From another loaded section (the shortcut layer is the shell's).
  await page.goto('/documents')
  await expect(page.getByTestId('doc-new')).toBeVisible()
  await page.keyboard.press('g')
  await page.keyboard.press(',')
  await expect(page).toHaveURL(/\/settings$/)
  await expect(page.getByTestId('settings-nav-profile')).toHaveClass(/active/)

  await page.getByTestId('settings-nav-security').click()
  await expect(page).toHaveURL(/\/settings\?s=security$/)
  await expect(page.getByTestId('settings-password')).toBeVisible()

  // The column is a list: arrows move the focus and open the section with it.
  await page.getByTestId('settings-nav-security').focus()
  await page.keyboard.press('ArrowDown')
  await expect(page).toHaveURL(/s=appearance/)
  await expect(page.getByTestId('settings-theme-dark')).toBeVisible()

  // A reload lands on the same section — the address is the state.
  await page.reload()
  await expect(page.getByTestId('settings-nav-appearance')).toHaveClass(/active/)
})

test('уведомления: канал из поповер-формы, удаление двумя нажатиями, расписание без кнопки', async ({
  page,
  seed,
}) => {
  await page.goto('/settings?s=notifications')
  await expect(page.getByTestId('notif-channels')).toBeVisible()

  // A channel through the popover form at the «Добавить канал» card.
  const label = `Почта ${seed.runId}`
  await page.getByTestId('notif-channel-add').click()
  const form = page.getByTestId('notif-channel-form')
  await expect(form).toBeVisible()
  await form.locator('input.kf-input').first().fill(label)
  const created = page.waitForResponse(
    (r) => /\/notification-channels$/.test(r.url()) && r.request().method() === 'POST',
  )
  await page.getByTestId('notif-channel-form-submit').click()
  expect((await created).status()).toBeLessThan(300)
  const card = page.getByTestId('notif-channel').filter({ hasText: label })
  await expect(card).toBeVisible()

  // Deletion is the menu's two-step item: armed first, gone on the second press.
  await card.hover()
  await card.getByTestId('notif-channel-menu').click()
  await page.getByTestId('notif-menu-remove').click()
  await expect(page.getByTestId('notif-menu-remove-armed')).toBeVisible()
  await expect(card).toBeVisible()
  const removed = page.waitForResponse(
    (r) => /\/notification-channels\/[^/]+$/.test(r.url()) && r.request().method() === 'DELETE',
  )
  await page.getByTestId('notif-menu-remove-armed').click()
  expect((await removed).status()).toBeLessThan(300)
  await expect(card).toHaveCount(0)

  // The schedule defaults have no save button: a switch writes on its own.
  const saved = page.waitForResponse(
    (r) => /\/notification-prefs$/.test(r.url()) && r.request().method() === 'PUT',
  )
  await page.getByTestId('notif-quiet').locator('.n-switch').first().click()
  expect((await saved).status()).toBe(200)
  // Put it back the same way, so the shared user leaves as it came.
  const restored = page.waitForResponse(
    (r) => /\/notification-prefs$/.test(r.url()) && r.request().method() === 'PUT',
  )
  await page.getByTestId('notif-quiet').locator('.n-switch').first().click()
  expect((await restored).status()).toBe(200)
})
