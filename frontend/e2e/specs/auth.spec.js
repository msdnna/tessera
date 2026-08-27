import { readFileSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
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

  await page.getByTestId('logout').click()
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

  await page.getByTestId('logout').click()
  await expect(page).toHaveURL(/\/login/)

  // The guard must hold after a fresh load too — logout revokes the refresh
  // token server-side, so bootstrap has nothing left to trade.
  await page.goto('/notes')
  await expect(page).toHaveURL(/\/login/)
})

// Language on the auth screens (#2818): the browser's own preference picks the
// first-visit language, and the toggle overrides it.
//
// These assert on both bundles at once, so they read the catalogs directly
// rather than going through e2e/i18n.js — that helper resolves against the run's
// seed language, and the point here is precisely which of the two is on screen.
const localesDir = resolve(dirname(fileURLToPath(import.meta.url)), '../../src/locales')
const loginTitle = Object.fromEntries(
  ['ru', 'en'].map((lang) => [
    lang,
    JSON.parse(readFileSync(resolve(localesDir, lang, 'common.json'), 'utf-8')).auth.login.title,
  ]),
)

const authTitle = (page) => page.locator('.auth-title')
const langToggle = (page) => page.getByTestId('auth-lang-toggle')

// No registration in this block: the backend caps registrations per IP, and the
// toggle is reachable while signed out — which is the case under test.
test.describe('язык экранов входа, системный русский', () => {
  test.use({ locale: 'de-DE' }) // a language we don't ship → the ru default

  test('незнакомый язык браузера открывает вход по-русски', async ({ page }) => {
    await page.goto('/login')
    await expect(authTitle(page)).toHaveText(loginTitle.ru)
    await expect(langToggle(page)).toHaveText('RU')
  })

  test('переключатель меняет язык и переживает перезагрузку', async ({ page }) => {
    await page.goto('/login')
    await expect(authTitle(page)).toHaveText(loginTitle.ru)

    await langToggle(page).click()
    await expect(authTitle(page)).toHaveText(loginTitle.en)
    await expect(langToggle(page)).toHaveText('EN')

    // The choice is a stored preference, not view state: a reload must not send
    // the anonymous visitor back to the browser's guess.
    await page.reload()
    await expect(authTitle(page)).toHaveText(loginTitle.en)
    await expect(langToggle(page)).toHaveText('EN')

    // And it follows them across the auth screens.
    await page.goto('/register')
    await expect(langToggle(page)).toHaveText('EN')
  })
})

test.describe('язык экранов входа, системный английский', () => {
  test.use({ locale: 'en-GB' }) // regional variant → matched on its primary subtag

  test('английский язык браузера открывает вход по-английски', async ({ page }) => {
    await page.goto('/login')
    await expect(authTitle(page)).toHaveText(loginTitle.en)
    await expect(langToggle(page)).toHaveText('EN')
  })
})
