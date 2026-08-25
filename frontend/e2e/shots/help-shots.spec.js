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

    // Admin screens (#2810). They exist only for a global admin, and the backend
    // hands that to the first account of an instance — so these run on a clean
    // database and skip on a shared one instead of shooting a redirect to the
    // board. Re-taking them therefore means a fresh E2E_DB_URL, not a re-run.
    test('администрирование', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      await page.goto('/admin')
      await expect(page.getByText('Пользователи экземпляра')).toBeVisible()
      // The user rows arrive in a second request; without this the shutter can
      // catch the OAuth card above an empty list.
      await expect(page.locator('.urow').first()).toBeVisible()
      // The OAuth card alone is taller than the viewport, so the accounts — what
      // this shot is of — start below the fold. Scrolled to the search field:
      // the list keeps its own heading in frame instead of floating loose.
      await page.locator('.admin .search').scrollIntoViewIfNeeded()
      await shoot(page, scheme, 'admin-users')
    })

    test('вход через GitLab (OAuth)', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      // The card is half again as tall as the standard viewport, and an element
      // screenshot that has to scroll to stitch comes back half-painted (the app
      // footer lands in the middle of it). Growing the window instead keeps the
      // whole card on one rendered page; the shot is cropped to the card anyway,
      // so the wider window changes nothing else in the picture.
      await page.setViewportSize({ width: 1440, height: 1600 })
      await page.goto('/admin')
      const card = page.locator('.oauth-card')
      await expect(card).toBeVisible()
      // The values come from the seed (demo.js), so the form is documented
      // filled in, not as a column of empty boxes.
      await expect(card.locator('.oauth-mono textarea')).toHaveValue(/demo-group/)
      await shoot(page, scheme, 'admin-oauth', card)
    })

    // The GitLab modal is two shots of one screen: the account block on top and
    // the binding fields below it, which do not fit the card together (the pane
    // scrolls at 76vh). The article shows each next to the text that explains it.
    async function openGitlab(page) {
      await openBoard(page)
      await page.getByRole('button', { name: 'Интеграции' }).click()
      // Naive UI's dropdown options are plain divs, not menu items with a role,
      // and the GitLab one renders its label through a render function (it can
      // carry a conflict badge) — so it is matched by text, not by role.
      await page.locator('.n-dropdown-option', { hasText: 'GitLab' }).click()
      const card = page.locator('.gl-card')
      await expect(card).toBeVisible()
      // The bindings arrive in their own request and the first one is selected
      // on load; waiting for its project path keeps the shutter off a blank form.
      await expect(card.getByPlaceholder('group/project')).toHaveValue('demo-group/demo-project')
      return card
    }

    test('GitLab: аккаунт', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const card = await openGitlab(page)
      await shoot(page, scheme, 'gitlab-account', card)
    })

    test('GitLab: привязка проекта к доске', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const card = await openGitlab(page)
      // Scroll the left pane, not the page: the card itself stays put, only its
      // content moves. Playwright's own scrollIntoViewIfNeeded is no good here —
      // it walks up to the nearest scrollable ancestor and finds the *page*,
      // which does not scroll behind a modal, so the pane never moved and this
      // shot came back a byte-for-byte twin of the account one. Scrolling the
      // pane by hand, anchored on the bindings heading, puts the whole set of
      // fields in frame instead.
      await card.locator('.gl-left').evaluate((pane) => {
        const head = [...pane.querySelectorAll('.gl-h')].find((h) =>
          h.textContent.includes('Привязки'),
        )
        pane.scrollTop += head.getBoundingClientRect().top - pane.getBoundingClientRect().top
      })
      await shoot(page, scheme, 'gitlab-bindings', card)
    })

    test('фоновые задания', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      await openBoard(page)
      await page.getByRole('button', { name: 'Фоновые задания' }).click()
      const modal = page.locator('.bj-modal')
      await expect(modal).toBeVisible()
      // The panel selects its first job on load; waiting for the detail pane
      // keeps the shot from showing «Выберите задание».
      await expect(modal.locator('.bj-detail-name')).toBeVisible()
      await shoot(page, scheme, 'background-jobs', modal)
    })

    test('справочный центр', async ({ page }) => {
      // The centre is a modal over the current screen (#2792), so the shot is
      // taken the way a reader gets there: from the board, through the footer's
      // «Помощь» menu — the board stays behind it, which is the whole point.
      await page.goto(`/board/${seed.boardId}`)
      await page.locator('[data-tour="footer-help"]').click()
      await page.locator('[data-help-menu="center"]').click()
      await expect(page.locator('.h-article')).toBeVisible()
      await shoot(page, scheme, 'help')
    })
  })
}
