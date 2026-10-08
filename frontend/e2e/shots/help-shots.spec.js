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
    thisDevice: 'это устройство',
    events: 'События',
    mention: 'Упоминания',
  },
  en: {
    thisDevice: 'this device',
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
      // The grouping control switches status ⇄ tag columns — the feature the board
      // is built around, and the one screenshot that has to be driven, not just
      // navigated to.
      await page.getByTestId('group-control').click()
      await page.getByTestId('facet-group-tag').click()
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
      await expect(page.getByTestId('column-actions-delete')).toBeVisible()
      await shoot(page, scheme, 'board-columns')
    })

    // «Вид доски» (#2986): the panel docked next to the board, «Карточки» open.
    test('настройка вида', async ({ page }) => {
      await openBoard(page)
      await page.locator('[data-tour="board-customize"]').click()
      await expect(page.getByTestId('board-customize')).toBeVisible()
      await page.getByTestId('customize-sec-cards').click()
      await shoot(page, scheme, 'board-customize')
    })

    // Saved views (#2987): an empty list would picture the empty state, not the
    // feature — so the shot saves a view first (the field in the panel's list),
    // then opens the chip's menu with it.
    test('сохранённые представления', async ({ page }) => {
      await openBoard(page)
      await page.locator('[data-tour="board-customize"]').click()
      await expect(page.getByTestId('board-customize')).toBeVisible()
      // The light run saves it; the dark one (a fresh browser) applies it.
      const saved = page.getByTestId('view-row-1')
      if (await saved.count()) await saved.click()
      else {
        await page.getByTestId('views-save-as').click()
        const field = page.getByTestId('views-save-input').locator('input')
        await field.fill('Мой спринт')
        await field.press('Enter')
      }
      await page.getByTestId('view-chip').click()
      await expect(page.getByTestId('view-chip-menu')).toBeVisible()
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

    // Every segment of the switcher names its view (`data-layout`) — steadier
    // than an index or a caption.
    const LAYOUTS = [
      { key: 'list', name: 'board-list', ready: '.list-view' },
      { key: 'calendar', name: 'board-calendar', ready: '.cal-cell' },
      { key: 'timeline', name: 'board-timeline', ready: '[data-testid="chart-controls"]' },
      { key: 'gantt', name: 'board-gantt', ready: '[data-testid="chart-controls"]' },
      { key: 'matrix', name: 'board-matrix', ready: '.m-colhead' },
    ]
    for (const layout of LAYOUTS) {
      test(`визуализация: ${layout.name}`, async ({ page }) => {
        await openBoard(page)
        await page.locator(`[data-tour="board-layout"] [data-layout="${layout.key}"]`).click()
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
      await expect(page.getByTestId('doc-tree')).toBeVisible()
      await expect(page.locator('.tile', { hasText: 'Регламент релиза' })).toBeVisible()
      await shoot(page, scheme, 'documents')
    })

    // The open document with the tree beside it (v2, #3106): the head row with
    // the crumbs and tools, the title, the sheet and the discussion card.
    test('документ открыт', async ({ page }) => {
      // Wide enough for the tree to stay beside an open document (it folds away
      // under ~1400px, see DocumentsView TREE_BESIDE_DOC).
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto('/documents')
      await page.locator('.tile', { hasText: 'Регламент релиза' }).click()
      await expect(page.locator('.ProseMirror')).toBeVisible()
      await expect(page.getByTestId('doc-save-state')).toHaveAttribute('data-state', 'saved')
      await page.waitForTimeout(400)
      await shoot(page, scheme, 'documents-doc')
    })

    test('шаблоны документов', async ({ page }) => {
      await page.goto('/documents')
      await page.getByTestId('doc-templates').click()
      await expect(page.getByTestId('tpl-tile').first()).toBeVisible()
      await page.waitForTimeout(300)
      await shoot(page, scheme, 'documents-templates')
    })

    // «Входящие» (#2972) — the panel next to the board, and the capture field
    // with a line read into a reminder tied to a task.
    test('входящие', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('inbox-button').click()
      await expect(page.getByTestId('inbox-panel')).toContainText('Созвон по релизу 2.4')
      await shoot(page, scheme, 'inbox')
    })

    test('запись во входящие', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('quick-fab-button').click()
      await page.locator('.fab-item[data-item="note"]').click()
      // «/пустых» finds «Ревизия пустых состояний» by a word of its title.
      await page.getByTestId('capture-input').fill('завтра 10:00 созвон по текстам /пустых')
      await expect(page.getByTestId('capture-task')).toBeVisible()
      await shoot(page, scheme, 'inbox-capture')
    })

    test('этапы', async ({ page }) => {
      await page.goto('/milestones')
      await expect(page.getByText('Релиз 2.4')).toBeVisible()
      await shoot(page, scheme, 'milestones')
    })

    // ── conferences (#2876) ──

    test('конференции', async ({ page }) => {
      await page.goto('/conferences')
      await expect(
        page.getByTestId('conference-row').filter({ hasText: 'Ежедневная летучка' }),
      ).toBeVisible()
      await shoot(page, scheme, 'conferences')
    })

    test('комната конференции', async ({ page }) => {
      // The lobby, not a live call: joining would need an SFU behind the backend
      // and this pipeline deliberately runs without one. It is also the state a
      // reader actually arrives at — the article walks them from here.
      await page.goto(`/conferences/${seed.conferenceId}`)
      await expect(page.getByTestId('conference-join')).toBeVisible()
      await shoot(page, scheme, 'conference-room')
    })

    // ── the «Проект» window, the stage page and strip (#2971) ──

    test('окно проекта', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('board-tags').click()
      await expect(page.getByTestId('project-window')).toBeVisible()
      await page.getByTestId('project-tab-general').click()
      await expect(page.getByTestId('project-general')).toBeVisible()
      await shoot(page, scheme, 'project-window')
    })

    test('управление тегами', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('board-tags').click()
      const tab = page.getByTestId('project-tags')
      await expect(tab.getByTestId('tag-row').first()).toBeVisible()
      // Two rows picked: the selection bar is what the article walks through.
      await tab.getByTestId('tag-check').nth(0).click()
      await tab.getByTestId('tag-check').nth(1).click()
      await expect(page.getByTestId('tag-selection-bar')).toBeVisible()
      await shoot(page, scheme, 'project-tags')
    })

    test('управление этапами', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('board-stages').click()
      await expect(page.getByTestId('stage-row').first()).toBeVisible()
      await shoot(page, scheme, 'project-stages')
    })

    test('страница этапа', async ({ page }) => {
      await page.goto('/milestones')
      await page.locator('.ms-row', { hasText: 'Релиз 2.4' }).first().click()
      await expect(page.getByTestId('stage-progress')).toBeVisible()
      await shoot(page, scheme, 'stage-page')
    })

    test('полоса этапа', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('stage-select').click()
      await page.getByTestId('stage-option').first().click()
      const strip = page.getByTestId('stage-strip')
      await expect(strip).toBeVisible()
      if ((await strip.getAttribute('data-open')) !== '1') {
        await page.getByTestId('stage-strip-toggle').click()
      }
      await expect(strip).toHaveAttribute('data-open', '1')
      await shoot(page, scheme, 'stage-strip')
    })

    // Admin screens (#2810). They exist only for a global admin, and the backend
    // hands that to the first account of an instance — so these run on a clean
    // database and skip on a shared one instead of shooting a redirect to the
    // board. Re-taking them therefore means a fresh E2E_DB_URL, not a re-run.
    test('администрирование', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      await page.goto('/admin')
      // The user rows arrive in their own request; without this the shutter can
      // catch the section head over an empty table.
      await expect(page.getByTestId('admin-user').first()).toBeVisible()
      await page.waitForTimeout(300)
      await shoot(page, scheme, 'admin-users')
    })

    test('вход через GitLab (OAuth)', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      // The card is taller than the standard viewport, and an element screenshot
      // that has to scroll to stitch comes back half-painted. Growing the window
      // keeps the whole card on one rendered page; the shot is cropped to it.
      await page.setViewportSize({ width: 1440, height: 1600 })
      await page.goto('/admin?s=integrations')
      const card = page.getByTestId('admin-oauth')
      await expect(card).toBeVisible()
      // The values come from the seed (demo.js), so the form is documented
      // filled in, not as a column of empty boxes.
      await expect(card.locator('.av-mono textarea')).toHaveValue(/demo-group/)
      await shoot(page, scheme, 'admin-oauth', card)
    })

    // The GitLab window (v2 #2975, kit/LargeWindow): one shot per section the
    // articles explain — «Подключения» from the top (the account and the
    // binding cards), the same section scrolled to the picked binding's fields,
    // «Обратная запись» and «Правила лейблов». Opened through its store, as
    // every door of the workspace menu does; the shot is the window.
    async function openGitlab(page) {
      await openBoard(page)
      await page.evaluate(() => {
        const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
        pinia._s.get('wsDialogs').open('gitlab')
      })
      const win = page.getByTestId('gitlab-window')
      await expect(win).toBeVisible()
      // The bindings arrive in their own request and the first one is picked on
      // load; waiting for its card keeps the shutter off an empty section.
      await expect(page.getByTestId('gitlab-binding-card').first()).toBeVisible()
      await page.waitForTimeout(300)
      return win
    }

    test('GitLab: аккаунт', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const win = await openGitlab(page)
      await shoot(page, scheme, 'gitlab-account', win)
    })

    test('GitLab: привязка проекта к доске', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const win = await openGitlab(page)
      // The section scrolls inside the window, not the page: bring the picked
      // binding's heading to the top of the pane by hand.
      await page.getByTestId('gitlab-connections').evaluate((tab) => {
        const pane = tab.querySelector('.glw-scroll')
        const form = tab.querySelector('[data-testid="gitlab-binding-form"]')
        pane.scrollTop += form.getBoundingClientRect().top - pane.getBoundingClientRect().top - 44
      })
      await page.waitForTimeout(200)
      await shoot(page, scheme, 'gitlab-bindings', win)
    })

    test('GitLab: действия обратной записи', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const win = await openGitlab(page)
      await page.getByTestId('gitlab-tab-writeback').click()
      // The pairs come from the seed; the first row keeps the shutter off the
      // section's empty frame.
      await expect(page.getByTestId('gitlab-wb-row').first()).toBeVisible()
      await shoot(page, scheme, 'gitlab-writeback', win)
    })

    test('GitLab: правила разбора меток', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      const win = await openGitlab(page)
      await page.getByTestId('gitlab-tab-rules').click()
      // The rules are the backend's defaults (S:/P:/M:) — the first one opened
      // shows its value table.
      const row = page.getByTestId('gitlab-rule-row').first()
      await expect(row).toBeVisible()
      await row.locator('.gr-sum').click()
      await expect(page.getByTestId('gitlab-rule-edit')).toBeVisible()
      await shoot(page, scheme, 'gitlab-tags', win)
    })

    test('фоновые задания', async ({ page }) => {
      test.skip(!seed.isAdmin, 'демо-пользователь не админ — нужна чистая база (E2E_DB_URL)')
      await openBoard(page)
      // G J — the window's own key (#2975).
      await page.keyboard.press('g')
      await page.keyboard.press('j')
      const win = page.getByTestId('jobs-window')
      await expect(win).toBeVisible()
      // The window picks its first job on load; waiting for the detail keeps
      // the shot from showing «Выберите задание».
      await expect(win.locator('.bj-detail-name')).toBeVisible()
      await page.waitForTimeout(300)
      await shoot(page, scheme, 'background-jobs', win)
    })

    // Notification settings (#2810, wave 4; v2 #3106 — their own section of
    // the settings screen, `?s=notifications`).
    async function openNotificationSettings(page) {
      // Without the permission the device row carries a red «Уведомления
      // запрещены в браузере» line — a picture of a headless browser's default,
      // not of the product.
      await page.context().grantPermissions(['notifications'])
      // The client registers this browser as a device channel on boot, but only
      // after the tour/what's-new chain ahead of it — later than the settings
      // screen fetches the channel list, so the row is missing on a cold load.
      // Waiting for that POST and then reloading makes the row deterministic.
      const registered = page
        .waitForResponse((r) => r.url().includes('/notification-devices'), { timeout: 15000 })
        .catch(() => null)
      await page.goto('/settings?s=notifications')
      await registered
      await page.reload()
      const section = page.getByTestId('notification-settings')
      await expect(section).toBeVisible()
      await expect(section.getByText(L.thisDevice)).toBeVisible()
      // Exact: the channel's own card, not the rule rows that list it as a
      // delivery target («→ Рабочая почта, Мой телеграм · …»).
      await expect(section.getByText('Мой телеграм', { exact: true })).toBeVisible()
      return section
    }

    test('уведомления: каналы доставки', async ({ page }) => {
      await openNotificationSettings(page)
      await page.waitForTimeout(300)
      await shoot(page, scheme, 'notifications-channels')
    })

    test('уведомления: правила маршрутизации', async ({ page }) => {
      const section = await openNotificationSettings(page)
      // The rule editor next to the rule list: the picture has to show both the
      // order of the rules (first match wins) and what a rule is made of.
      const block = section.getByTestId('notif-routes')
      await block.evaluate((el) => el.scrollIntoView({ block: 'start', behavior: 'instant' }))
      await section.getByTestId('notif-route-add').click()
      const form = page.getByTestId('notif-route-form')
      await expect(form).toBeVisible()
      // Pick the events so the form is documented in use, not empty. The
      // options render in a detached dropdown layer, outside the form.
      await form.locator('.kf', { hasText: L.events }).locator('.n-base-selection').click()
      await page.locator('.n-base-select-option', { hasText: L.mention }).first().click()
      // Close the dropdown with a click on the form's title — Esc would close
      // the form itself.
      await form.locator('.pf-title').click()
      await expect(form.locator('.n-tag', { hasText: L.mention })).toBeVisible()
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

// ── the phone (#2976 — package G) ──
// The mobile web at 390×844 with touch: the bottom navigation, the board, the
// tree in its drawer and «Ещё». Scale 1, like the rest of the set — the article
// shows them at their own size.
for (const scheme of ['light', 'dark']) {
  test.describe(`телефон · ${scheme}`, () => {
    test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })

    test('телефон: доска', async ({ page }) => {
      await openBoard(page)
      await expect(page.getByTestId('mobile-tabbar')).toBeVisible()
      await shoot(page, scheme, 'mobile-board')
    })

    test('телефон: дерево в дровере', async ({ page }) => {
      await openBoard(page)
      await page.getByTestId('mobile-board-switch').tap()
      await expect(page.locator('.n-drawer').getByTestId('sidebar')).toBeVisible()
      await page.waitForTimeout(400)
      await shoot(page, scheme, 'mobile-drawer')
    })

    test('телефон: мои задачи', async ({ page }) => {
      await page.goto('/')
      await expect(page.getByTestId('home')).toBeVisible()
      await page.waitForLoadState('networkidle').catch(() => {})
      await shoot(page, scheme, 'mobile-home')
    })

    test('телефон: ещё', async ({ page }) => {
      await page.goto('/more')
      await expect(page.getByTestId('more-page')).toBeVisible()
      await shoot(page, scheme, 'mobile-more')
    })
  })
}
