import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// The Get Started guide (#2753) as a first-time user actually meets it: nobody
// opens it, it opens itself on the first load of a freshly registered account.
//
// This spec is the one place in the suite where that happens — api.js register()
// opts every other user out, because the guide's mask and popover would land on
// top of whatever that spec is about to click.
//
// Scope is the entry/exit machinery of #2760: autostart, «Понятно» advancing,
// «Пропустить» ending it for good (ack written → not back after a reload) and
// «Обучение» in the footer bringing it back on demand. Walking the whole
// twelve-point scenario is deliberately out: those steps wait on real projects,
// boards and tasks being created, which the board specs already cover.

test.use({ signedIn: false })

// A fresh account per test: the guide's autostart is once-per-account by design,
// so a shared user would only ever see it in whichever test ran first.
async function freshUser() {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const creds = newCredentials(id, '-tour')
  await register(creds, { guide: true }) // the one user in the suite that keeps it
  return creds
}

test('guide autostarts for a new account and advances on «Понятно»', async ({ page }) => {
  await signIn(page, await freshUser())
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await expect(pop).toContainText('Пространства')

  // The first step is informational: both buttons, and «Понятно» moves on.
  await expect(page.getByTestId('tour-next')).toBeVisible()
  await page.getByTestId('tour-next').click()
  await expect(pop).toContainText('Дерево проектов')

  // Step 2 asks the user to act, so «Понятно» is gone and only «Пропустить» is
  // left — the author's call: no way to skip past an action step.
  await expect(page.getByTestId('tour-next')).toHaveCount(0)
  await expect(page.getByTestId('tour-skip')).toBeVisible()
})

test('«Пропустить» ends the guide for good', async ({ page }) => {
  await signIn(page, await freshUser())
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-skip').click()
  await expect(pop).toHaveCount(0)

  // The ack is server-side, so it survives a reload — this is the check that the
  // e2e suite's own opt-out works, not just an in-memory flag.
  await page.reload()
  await expect(page.locator('.sb-footer')).toBeVisible() // app is up again
  await expect(pop).toHaveCount(0)
})

test('«Обучение» in the sidebar footer restarts the guide', async ({ page }) => {
  await signIn(page, await freshUser())
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-skip').click()
  await expect(pop).toHaveCount(0)

  await page.locator('[data-tour="footer-tour"]').click()
  await expect(pop).toBeVisible()
  await expect(pop).toContainText('Пространства')
})
