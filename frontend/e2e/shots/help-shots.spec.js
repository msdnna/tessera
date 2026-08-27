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

// The language axis (#2816). `make help-shots` still writes the Russian set under
// the plain names; `make help-shots-en` writes `<name>-<scheme>.en.png` next to
// them, which is exactly what helpAssets.js/HelpAssets.kt look for first when the
// reader is on an English article. Russian is the base set, so it keeps the bare
// name — the fallback resolves *to* it, and renaming it would orphan every
// article's <img src>.
const LANG = process.env.TESSERA_SHOTS_LANG || 'ru'
if (!['ru', 'en'].includes(LANG)) {
  throw new Error(`TESSERA_SHOTS_LANG=${LANG}: поддерживаются только ru и en`)
}
const SUFFIX = LANG === 'ru' ? '' : `.${LANG}`

// Captions the shots have to click or wait for. Only *interface* strings live
// here — the demo data («Регламент релиза», «Релиз 2.4», «Мой телеграм») is
// content and stays Russian in both runs, the way a real English-speaking user's
// own workspace would keep its own language. Column selectors are unaffected too:
// `data-column-name` carries the stored name, not the localized caption.
const CAPTIONS = {
  ru: {
    integrations: 'Интеграции',
    jobs: 'Фоновые задания',
    adminUsers: 'Пользователи экземпляра',
    // Substring of the bindings heading («Привязки GitLab → доска»).
    bindingsHead: 'Привязки',
    configureWriteback: /Настроить действия/,
    configureRules: 'Настроить правила',
    notifications: 'Уведомления',
    thisDevice: 'это устройство',
    routesTitle: 'Правила маршрутизации',
    addRule: 'Добавить правило',
    newRule: 'Новое правило',
    events: 'События',
    mention: 'Упоминания',
  },
  en: {
    integrations: 'Integrations',
    jobs: 'Background jobs',
    adminUsers: 'Users on this instance',
    // Substring of «GitLab → board bindings» — lower-case, the match is exact.
    bindingsHead: 'bindings',
    configureWriteback: /Configure actions/,
    configureRules: 'Configure rules',
    notifications: 'Notifications',
    thisDevice: 'this device',
    routesTitle: 'Routing rules',
    addRule: 'Add rule',
    newRule: 'New rule',
    events: 'Events',
    mention: 'Mentions',
  },
}
const L = CAPTIONS[LANG]

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
        // Pin the device id (#2810, wave 4). Every test gets a fresh browser
        // context, and the app registers a "device" notification channel per
        // unseen id — so the channel list grew by one row with each shot, and
        // the light and dark twins of the notifications picture disagreed about
        // how many devices the account has. One fixed id = exactly one
        // «Браузер (Chrome)» row, the same in both.
        localStorage.setItem('tessera_device_id', 'demo-device-help-shots')
        const res = await fetch('/api/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Auth-Mode': 'cookie' },
          body: JSON.stringify({ email: c.email, password: c.password }),
        })
        if (!res.ok) return `login ${res.status}: ${(await res.text()).slice(0, 200)}`
        const data = await res.json()
        localStorage.setItem('tessera_user', JSON.stringify(data.user))
        // Interface language (#2816). localStorage alone is not enough: the theme
        // store's hydrate() overwrites `tessera_prefs` with whatever /auth/me
        // returns, so a purely local switch flips back to Russian one request
        // later. The preference is written the same way the settings screen
        // writes it — a full PUT (the endpoint replaces the row, hence spreading
        // the server's own values) — and mirrored into localStorage so the very
        // first paint is already in the right language.
        const prefs = { ...data.preferences, language: c.lang }
        localStorage.setItem('tessera_prefs', JSON.stringify(prefs))
        if (c.lang !== 'ru') {
          const put = await fetch('/api/users/me/preferences', {
            method: 'PUT',
            headers: {
              'Content-Type': 'application/json',
              // Cookie mode only moves the *refresh* token out of reach; the
              // access token still comes back in the body and protected routes
              // take it as a bearer, the same as for the app itself.
              Authorization: `Bearer ${data.access_token}`,
            },
            body: JSON.stringify(prefs),
          })
          if (!put.ok) return `prefs ${put.status}: ${(await put.text()).slice(0, 200)}`
        }
        return true
      },
      { ...seed.creds, workspaceId: seed.workspaceId, lang: LANG },
    )
    if (ok !== true) throw new Error(`не удалось войти под демо-пользователем: ${ok}`)
    await use(page)
  },
})

// shoot writes <name>-<scheme>.png (<name>-<scheme>.<lang>.png outside Russian).
// `animations: 'disabled'` freezes the loader
// and the card transitions, so re-running the pipeline on unchanged UI produces
// byte-identical files instead of a diff on every run.
async function shoot(page, scheme, name, target) {
  await page.emulateMedia({ colorScheme: scheme })
  // One frame for the theme swap to paint before the shutter.
  await page.waitForTimeout(150)
  await (target || page).screenshot({
    path: resolve(ASSETS, `${name}-${scheme}${SUFFIX}.png`),
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

    // ── the board's own controls (#2823) ──
    // These six are driven by structure, not by captions: the composer chips, the
    // column menu and the layout switcher all carry translated labels, so every
    // locator below is a class, a testid or an index into a fixed list. That is
    // what lets the same spec produce both the Russian and the English set.

    test('панель доски: меню добавления', async ({ page }) => {
      await openBoard(page)
      // The add menu renders in a detached dropdown layer, so the frame is the
      // whole page — cropping to the bar would cut the menu the shot is about.
      await page.locator('.facet-add').click()
      await expect(page.locator('.n-dropdown-option').first()).toBeVisible()
      await shoot(page, scheme, 'board-composer')
    })

    test('меню колонки', async ({ page }) => {
      await openBoard(page)
      await page.locator('[data-testid="column"]').first().locator('.col-menu').click()
      await expect(page.getByTestId('column-delete')).toBeVisible()
      await shoot(page, scheme, 'board-columns')
    })

    test('настройка вида', async ({ page }) => {
      await openBoard(page)
      await page.locator('[data-tour="board-customize"]').click()
      await expect(page.locator('.n-drawer')).toBeVisible()
      await shoot(page, scheme, 'board-customize')
    })

    test('сохранённые представления', async ({ page }) => {
      await openBoard(page)
      // An empty list would picture the empty state, not the feature — so the
      // shot saves a view first, then opens the list that now has it. The two
      // buttons are the last pair of `.bar-tools` (folder, then disk).
      const tools = page.locator('.bar-tools .bar-btn')
      await tools.nth(2).click()
      await page.locator('.views-pop input').fill('Мой спринт')
      await page.locator('.views-pop input').press('Enter')
      await tools.nth(1).click()
      await expect(page.locator('.views-pop .view-name')).toBeVisible()
      await shoot(page, scheme, 'board-views')
    })

    test('архив доски', async ({ page }) => {
      await openBoard(page)
      // The archive is a scope of the same board, reached by a query parameter —
      // no caption to click, and the amber scope chip is what has to be in frame.
      await page.goto(`${new URL(page.url()).pathname}?archived=1`)
      await expect(page.locator('.facet-archive')).toBeVisible()
      await shoot(page, scheme, 'board-archive')
    })

    // The switcher lists the visualizations in a fixed order (board, list,
    // calendar, timeline, gantt, matrix), so an index beats a caption here.
    const LAYOUTS = [
      { idx: 1, name: 'board-list', ready: '.list-view' },
      { idx: 2, name: 'board-calendar', ready: '.cal-cell' },
      { idx: 3, name: 'board-timeline', ready: '.tl-toolbar' },
      { idx: 4, name: 'board-gantt', ready: '.tl-toolbar' },
      { idx: 5, name: 'board-matrix', ready: '.m-colhead' },
    ]
    for (const layout of LAYOUTS) {
      test(`визуализация: ${layout.name}`, async ({ page }) => {
        await openBoard(page)
        await page.locator('[data-tour="board-layout"] button').nth(layout.idx).click()
        await expect(page.locator(layout.ready).first()).toBeVisible()
        await shoot(page, scheme, layout.name)
      })
    }

    // ── the task window and its tabs (#2823, wave 2) ──
    // openTask picks a card by its title so each shot lands on the task that has
    // something to show: the push task carries the thread, the document link and
    // a journal, the empty-states pass carries the only blocking relation.
    async function openTask(page, title) {
      await openBoard(page)
      await page.getByTestId('task-card').filter({ hasText: title }).first().click()
      const modal = page.getByTestId('task-modal')
      await expect(modal).toBeVisible()
      return modal
    }
    const PUSH_TASK = 'Push-уведомления о напоминаниях'

    test('окно задачи', async ({ page }) => {
      await openTask(page, PUSH_TASK)
      await shoot(page, scheme, 'task-modal')
    })

    test('полноэкранный редактор', async ({ page }) => {
      await openTask(page, PUSH_TASK)
      // The description toolbar sits in the section header. Its buttons are
      // icon-only and their count depends on the mode — a saved description opens
      // in preview, where the image and mermaid buttons are not rendered at all,
      // so an index into the row points at a different button than it does while
      // writing. Hence a testid: the titles are translated and can't anchor the
      // English run either.
      await page.getByTestId('desc-fullscreen').click()
      // The split editor is what the article is about: text left, live preview
      // right, so the shot has to wait for the preview pane, not just the modal.
      await expect(page.locator('.mdfs .md2-preview-side')).toBeVisible()
      await shoot(page, scheme, 'task-markdown-editor')
    })

    test('комментарии', async ({ page }) => {
      await openTask(page, PUSH_TASK)
      await page.getByTestId('tab-comments').click()
      // Wait for the reply, not the root: the thread is the picture, and the root
      // renders one request earlier.
      await expect(page.locator('.c-reply').first()).toBeVisible()
      await shoot(page, scheme, 'task-comments')
    })

    test('связи задачи', async ({ page }) => {
      await openTask(page, 'Ревизия пустых состояний')
      await page.getByTestId('tab-relations').click()
      await expect(page.locator('.relrow').first()).toBeVisible()
      await shoot(page, scheme, 'task-relations')
    })

    test('документы задачи', async ({ page }) => {
      await openTask(page, PUSH_TASK)
      await page.getByTestId('tab-documents').click()
      await expect(page.getByTestId('task-doc-link').first()).toBeVisible()
      await shoot(page, scheme, 'task-documents')
    })

    test('история задачи', async ({ page }) => {
      await openTask(page, PUSH_TASK)
      await page.getByTestId('tab-history').click()
      await expect(page.locator('.histrow').first()).toBeVisible()
      await shoot(page, scheme, 'task-history')
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
      await expect(page.getByText(L.adminUsers)).toBeVisible()
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
      await page.getByRole('button', { name: L.integrations }).click()
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
      await card.locator('.gl-left').evaluate((pane, head0) => {
        const head = [...pane.querySelectorAll('.gl-h')].find((h) => h.textContent.includes(head0))
        pane.scrollTop += head.getBoundingClientRect().top - pane.getBoundingClientRect().top
      }, L.bindingsHead)
      await shoot(page, scheme, 'gitlab-bindings', card)
    })

    // The two right-pane editors (#2810, wave 3). Both live below the fold of the
    // left pane, so the pane is scrolled to the button row before clicking; the
    // shot is of the whole card, because the point of these pictures is that the
    // modal *expands* into a second pane next to the settings it belongs to.
    async function openRightPane(page, buttonName) {
      const card = await openGitlab(page)
      await card.locator('.gl-left').evaluate((pane) => {
        pane.scrollTop = pane.scrollHeight
      })
      await card.getByRole('button', { name: buttonName }).click()
      await expect(card.locator('.gl-right')).toBeVisible()
      return card
    }

    test('GitLab: действия обратной записи', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const card = await openRightPane(page, L.configureWriteback)
      // The bindings come from the seed; waiting for the first row's summary keeps
      // the shutter off the pane's own loading frame.
      await expect(card.locator('.gl-rcard').first()).toBeVisible()
      await shoot(page, scheme, 'gitlab-writeback', card)
    })

    test('GitLab: правила разбора меток', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const card = await openRightPane(page, L.configureRules)
      // The rules are the backend's defaults (S:/P:/M:), so the value maps are
      // filled — waiting for the first one avoids shooting three empty cards.
      await expect(card.locator('.gl-rmap .gl-rule').first()).toBeVisible()
      await shoot(page, scheme, 'gitlab-tags', card)
    })

    test('фоновые задания', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      await openBoard(page)
      await page.getByRole('button', { name: L.jobs }).click()
      const modal = page.locator('.bj-modal')
      await expect(modal).toBeVisible()
      // The panel selects its first job on load; waiting for the detail pane
      // keeps the shot from showing «Выберите задание».
      await expect(modal.locator('.bj-detail-name')).toBeVisible()
      await shoot(page, scheme, 'background-jobs', modal)
    })

    // Notification settings (#2810, wave 4). They live in a long settings page,
    // so both shots are of the viewport scrolled to the block in question rather
    // than of the section element — the section is taller than any sane window,
    // and an element screenshot that has to stitch comes back half-painted.
    async function openNotificationSettings(page) {
      // Without the permission the device row carries a red «Уведомления
      // запрещены в браузере» line — a picture of a headless browser's default,
      // not of the product.
      await page.context().grantPermissions(['notifications'])
      // The client registers this browser as a device channel on boot, but only
      // after the tour/what's-new chain ahead of it — later than the settings
      // screen fetches the channel list, so the row is missing on a cold load.
      // Waiting for that POST and then reloading makes the row deterministic:
      // without it the light shot (the first test to run) had no device row and
      // its dark twin, running after the channel already existed, had one.
      const registered = page
        .waitForResponse((r) => r.url().includes('/notification-devices'), { timeout: 15000 })
        .catch(() => null)
      await page.goto('/settings')
      await registered
      await page.reload()
      const section = page
        .locator('section.card')
        .filter({ has: page.getByRole('heading', { name: L.notifications }) })
      await expect(section).toBeVisible()
      await expect(section.getByText(L.thisDevice)).toBeVisible()
      // Channels arrive in their own request; the seeded one keeps the shutter
      // off the spinner and off the «Каналов пока нет» empty state.
      // Exact: the channel's own row, not the two rule rows that list it as a
      // delivery target («→ Рабочая почта, Мой телеграм · …»).
      await expect(section.getByText('Мой телеграм', { exact: true })).toBeVisible()
      return section
    }

    // scrollIntoViewIfNeeded only scrolls until the element is *somewhere* in the
    // viewport, which leaves the settings screen showing the tail of the security
    // card above the block being documented. `block: 'start'` pins it to the top.
    async function scrollToTop(locator) {
      await locator.evaluate((el) => el.scrollIntoView({ block: 'start', behavior: 'instant' }))
    }

    test('уведомления: каналы доставки', async ({ page }) => {
      const section = await openNotificationSettings(page)
      await scrollToTop(section)
      await shoot(page, scheme, 'notifications-channels')
    })

    test('уведомления: правила маршрутизации', async ({ page }) => {
      const section = await openNotificationSettings(page)
      // The rule editor over the rule list: the picture has to show both the
      // order of the rules (first match wins) and what a rule is made of, so the
      // list is scrolled up first and the form opened on top of it.
      const block = section.locator('.block', { hasText: L.routesTitle }).first()
      await scrollToTop(block)
      await section.getByRole('button', { name: L.addRule }).click()
      const modal = page.locator('.n-card', { hasText: L.newRule })
      await expect(modal).toBeVisible()
      // Pick the events so the form is documented in use, not empty. The label is
      // a <span> above the select, so the click has to land on the selection box
      // itself; the options then render in a detached dropdown layer, outside the
      // modal — hence the page-level locator for them.
      await modal.locator('.field', { hasText: L.events }).locator('.n-base-selection').click()
      await page.locator('.n-base-select-option', { hasText: L.mention }).first().click()
      await page.keyboard.press('Escape') // close the dropdown, keep the modal
      // The pick lands as a tag inside the select — that is what has to be in frame.
      await expect(modal.locator('.n-tag', { hasText: L.mention })).toBeVisible()
      await shoot(page, scheme, 'notifications-routes')
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
