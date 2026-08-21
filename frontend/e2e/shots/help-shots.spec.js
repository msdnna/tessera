import { test as base, expect } from '@playwright/test'
import { readFileSync, mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { SHOTS_SEED_FILE } from './global-setup.js'

// Screenshots for the help centre (#2793). Not an e2e test: nothing here asserts
// product behaviour, it drives the app to a presentable state and writes PNGs
// straight into docs/help/assets, where the articles link them. It lives outside
// e2e/specs and runs from its own config (playwright.docs.config.js) so
// `make test-e2e-frontend` never picks it up.
//
// Every shot is taken twice — light and dark — from the same page state. The
// theme comes from `emulateMedia({ colorScheme })` because the app's default
// theme mode is "system"; nothing has to be clicked or seeded server-side.
//
// The dark twin is what makes this worth automating: theme regressions in
// Tessera have historically only shown up in dark (see the accent-gradient and
// ProseMirror notes in CLAUDE.md), and hand-taken pictures were always light.

const here = dirname(fileURLToPath(import.meta.url))
const ASSETS = resolve(here, '../../../docs/help/assets')
const seed = JSON.parse(readFileSync(SHOTS_SEED_FILE, 'utf-8'))

mkdirSync(ASSETS, { recursive: true })

const test = base.extend({
  // Signed in and pointed at the demo workspace before every shot. Same fetch
  // login as the e2e fixtures (the refresh cookie is single-use, so a shared
  // storageState would be spent by the first test — see e2e/fixtures.js).
  page: async ({ page }, use) => {
    await page.goto('/login')
    const ok = await page.evaluate(
      async (c) => {
        // The demo workspace is not the account's first one (registration makes a
        // personal workspace of its own), and the workspace-scoped screens —
        // документы, заметки, этапы — read the *current* workspace, not the board's.
        // Without this they render the empty personal workspace; the board shots
        // never noticed because a board deep link switches workspace by itself.
        localStorage.setItem('tessera_ws', c.workspaceId)
        const res = await fetch('/api/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Auth-Mode': 'cookie' },
          body: JSON.stringify({ email: c.email, password: c.password }),
        })
        if (!res.ok) return `login ${res.status}: ${(await res.text()).slice(0, 200)}`
        const data = await res.json()
        localStorage.setItem('tessera_user', JSON.stringify(data.user))
        return true
      },
      { ...seed.creds, workspaceId: seed.workspaceId },
    )
    if (ok !== true) throw new Error(`не удалось войти под демо-пользователем: ${ok}`)
    await use(page)
  },
})

// shoot writes <name>-<scheme>.png. `animations: 'disabled'` freezes the loader
// and the card transitions, so re-running the pipeline on unchanged UI produces
// byte-identical files instead of a diff on every run.
async function shoot(page, scheme, name, target) {
  await page.emulateMedia({ colorScheme: scheme })
  // One frame for the theme swap to paint before the shutter.
  await page.waitForTimeout(150)
  await (target || page).screenshot({
    path: resolve(ASSETS, `${name}-${scheme}.png`),
    animations: 'disabled',
  })
}

async function openBoard(page) {
  await page.goto(`/board/${seed.boardId}`)
  await page.waitForURL(/\/project\/[^/]+\/board\//, { timeout: 15000 })
  await expect(page.getByTestId('column').first()).toBeVisible()
  // Cards arrive with the board payload; waiting for one keeps the shutter from
  // catching an empty column.
  await expect(page.getByTestId('task-card').first()).toBeVisible()
}

for (const scheme of ['light', 'dark']) {
  test.describe(`${scheme}`, () => {
    test('доска', async ({ page }) => {
      await openBoard(page)
      await shoot(page, scheme, 'board')
    })

    test('доска, сгруппированная по тегам', async ({ page }) => {
      await openBoard(page)
      // The grouping chip toggles status ⇄ tag columns — the feature the board
      // is built around, and the one screenshot that has to be driven, not just
      // navigated to.
      await page.locator('.facet.group').click()
      await expect(page.getByTestId('column').first()).toBeVisible()
      await shoot(page, scheme, 'board-tags')
    })

    test('окно задачи', async ({ page }) => {
      await openBoard(page)
      await page
        .locator('[data-testid="column"][data-column-name="В процессе"]')
        .getByTestId('task-card')
        .first()
        .click()
      const modal = page.getByTestId('task-modal')
      await expect(modal).toBeVisible()
      await shoot(page, scheme, 'task-modal')
    })

    test('документы', async ({ page }) => {
      await page.goto('/documents')
      await expect(page.getByText('Регламент релиза')).toBeVisible()
      await shoot(page, scheme, 'documents')
    })

    test('заметки', async ({ page }) => {
      await page.goto('/notes')
      // Open one: the list alone leaves two thirds of the screen saying
      // «Выберите заметку», which is a picture of nothing.
      await page.locator('.note-item', { hasText: 'Итоги планирования' }).click()
      // The body opens in a textarea, so it is a value, not page text.
      await expect(page.locator('.editor textarea')).toHaveValue(/Импорт CSV/)
      await shoot(page, scheme, 'notes')
    })

    test('напоминания', async ({ page }) => {
      await page.goto('/reminders')
      await expect(page.getByText('Созвон по релизу 2.4 в 11:00')).toBeVisible()
      await shoot(page, scheme, 'reminders')
    })

    test('этапы', async ({ page }) => {
      await page.goto('/milestones')
      await expect(page.getByText('Релиз 2.4')).toBeVisible()
      await shoot(page, scheme, 'milestones')
    })

    test('справочный центр', async ({ page }) => {
      await page.goto('/help')
      await expect(page.locator('.h-article')).toBeVisible()
      await shoot(page, scheme, 'help')
    })
  })
}
