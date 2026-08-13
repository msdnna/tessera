import { test, expect } from '../fixtures.js'

// The auth flow is the one thing the suite must NOT shortcut: every other spec
// starts from a stored session, so if login broke, only this spec would notice.
test.use({ signedIn: false })

function freshCreds() {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  return { email: `e2e+ui${id}@test.local`, name: `E2E UI ${id}`, password: 'e2e-password-123' }
}

test('регистрация через форму заводит сессию и открывает приложение', async ({ page }) => {
  const creds = freshCreds()
  await page.goto('/register')
  await page.getByTestId('register-name').locator('input').fill(creds.name)
  await page.getByTestId('register-email').locator('input').fill(creds.email)
  await page.getByTestId('register-password').locator('input').fill(creds.password)
  await page.getByTestId('register-submit').click()

  await expect(page.getByTestId('sidebar')).toBeVisible()
  await expect(page).toHaveURL(/\/$/)
})

test('неверный пароль оставляет на странице входа с ошибкой', async ({ page }) => {
  const creds = freshCreds()
  await page.goto('/register')
  await page.getByTestId('register-name').locator('input').fill(creds.name)
  await page.getByTestId('register-email').locator('input').fill(creds.email)
  await page.getByTestId('register-password').locator('input').fill(creds.password)
  await page.getByTestId('register-submit').click()
  await expect(page.getByTestId('sidebar')).toBeVisible()

  await page.getByRole('button', { name: 'Выйти' }).click()
  await expect(page).toHaveURL(/\/login/)

  await page.getByTestId('login-email').locator('input').fill(creds.email)
  await page.getByTestId('login-password').locator('input').fill('definitely-not-the-password')
  await page.getByTestId('login-submit').click()

  await expect(page.locator('.auth-error')).toBeVisible()
  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByTestId('sidebar')).toHaveCount(0)
})

test('сессия переживает перезагрузку страницы (bootstrap по refresh-куке)', async ({ page }) => {
  const creds = freshCreds()
  await page.goto('/register')
  await page.getByTestId('register-name').locator('input').fill(creds.name)
  await page.getByTestId('register-email').locator('input').fill(creds.email)
  await page.getByTestId('register-password').locator('input').fill(creds.password)
  await page.getByTestId('register-submit').click()
  await expect(page.getByTestId('sidebar')).toBeVisible()

  // #2684: the access token lives in memory only, so this reload genuinely
  // re-authenticates from the httpOnly refresh cookie rather than reading a
  // token back out of localStorage.
  await page.reload()
  await expect(page.getByTestId('sidebar')).toBeVisible()
  await expect(page).not.toHaveURL(/\/login/)

  const cookies = await page.context().cookies()
  expect(cookies.some((c) => c.httpOnly && c.path.startsWith('/api/auth'))).toBe(true)
})

test('логаут разлогинивает и защищает приватный маршрут', async ({ page }) => {
  const creds = freshCreds()
  await page.goto('/register')
  await page.getByTestId('register-name').locator('input').fill(creds.name)
  await page.getByTestId('register-email').locator('input').fill(creds.email)
  await page.getByTestId('register-password').locator('input').fill(creds.password)
  await page.getByTestId('register-submit').click()
  await expect(page.getByTestId('sidebar')).toBeVisible()

  await page.getByRole('button', { name: 'Выйти' }).click()
  await expect(page).toHaveURL(/\/login/)

  // The guard must hold after a fresh load too — logout revokes the refresh
  // token server-side, so bootstrap has nothing left to trade.
  await page.goto('/notes')
  await expect(page).toHaveURL(/\/login/)
})
