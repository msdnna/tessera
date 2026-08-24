import { readFileSync, mkdirSync, writeFileSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect, cardsIn, openBoard } from '../fixtures.js'
import { SEED_FILE } from '../global-setup.js'

// Visual pass over both locales (#2800, step 4). English is the longer language:
// "Средний" is 7 characters, "Medium" 6, but "In progress" is half again as wide
// as "В процессе", and every fixed-width shell in the app — priority pills,
// settings grids, column headers — is sized around the Russian string. The run
// screenshots the same seeded data in whichever language global-setup seeded
// (E2E_LANG), so the two runs produce comparable frames, and it fails on measured
// clipping rather than leaving it to the eye.
const here = dirname(fileURLToPath(import.meta.url))
const seed = JSON.parse(readFileSync(SEED_FILE, 'utf-8'))
const LANG = seed.language || 'ru'
const OUT = resolve(here, '../.auth/locale-shots')

mkdirSync(OUT, { recursive: true })

const shot = async (target, name) => {
  await target.screenshot({ path: resolve(OUT, `${name}-${LANG}.png`) })
}

// Text that is cut off by its own box. `scrollWidth > clientWidth` is the browser
// telling us the string did not fit — with `text-overflow: ellipsis` that is a
// deliberate truncation, without it the glyphs are simply clipped. Only the
// second kind is reported: an ellipsis on a long task title is by design, a
// clipped "In progress" in a column header is the bug this pass looks for.
const clipped = (page, root) =>
  page.evaluate((sel) => {
    const out = []
    for (const el of document.querySelector(sel).querySelectorAll('*')) {
      if (el.scrollWidth <= el.clientWidth + 1 || !el.clientWidth) continue
      const st = getComputedStyle(el)
      if (st.textOverflow === 'ellipsis' || st.overflowX === 'auto' || st.overflowX === 'scroll')
        continue
      const text = (el.textContent || '').trim().slice(0, 60)
      if (!text) continue
      out.push({ text, tag: el.tagName.toLowerCase(), cls: el.className?.toString().slice(0, 40) })
    }
    return out
  }, root)

const report = []

test.afterAll(() => {
  writeFileSync(resolve(OUT, `clipping-${LANG}.json`), JSON.stringify(report, null, 2))
})

test('доска: карточки с пилюлями приоритета, срока и оценки', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard(`locale-${LANG}`)
  const due = new Date(Date.now() + 3 * 864e5).toISOString().slice(0, 10)
  // One card per priority so the widest pill of each locale lands in frame; the
  // estimate and the due date share the same row and are what pushes it over.
  const cards = [
    { title: 'Critical path review', priority: 4, estimate: 8 },
    { title: 'Refactor the sync worker', priority: 3, estimate: 13 },
    { title: 'Update onboarding copy', priority: 2, estimate: 3 },
    { title: 'Chase flaky screenshot test', priority: 1, estimate: 1 },
  ]
  for (const c of cards) {
    // Posted to the fresh board directly: `backend.createTask` is bound to the
    // shared seed board, and its column ids belong to a different board.
    await backend.post(`/boards/${board.id}/tasks`, {
      column_id: columns[0].id,
      title: c.title,
      priority: c.priority,
      estimate: c.estimate,
      due_date: `${due}T12:00:00Z`,
    })
  }

  await openBoard(page, board.id)
  await expect(cardsIn(page, columns[0].name).first()).toBeVisible()
  await shot(page, 'board')
  report.push({ screen: 'board', clipped: await clipped(page, '.kanban, [data-testid="column"]') })
})

test('модалка задачи: поля и подписи', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard(`locale-modal-${LANG}`)
  const title = 'Investigate duplicated notifications'
  await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    priority: 3,
    estimate: 5,
    description: 'Steps to reproduce:\n\n1. Open two sessions\n2. Comment from one of them',
  })

  await openBoard(page, board.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  // The panel slides in; a screenshot taken mid-transform catches it in flight.
  await expect.poll(() => modal.evaluate((el) => getComputedStyle(el).transform)).toBe('none')
  await shot(modal, 'task-modal')
  report.push({ screen: 'task-modal', clipped: await clipped(page, '[data-testid="task-modal"]') })
})

test('настройки: гриды профиля, оформления и локализации', async ({ page }) => {
  await page.goto('/settings')
  await expect(page.locator('.settings h1')).toBeVisible()
  // Selects render their label asynchronously (Naive UI resolves the option list
  // after mount); without this the localisation card screenshots empty.
  await expect(page.locator('.settings .n-base-selection-label').first()).not.toBeEmpty()
  await shot(page, 'settings')
  report.push({ screen: 'settings', clipped: await clipped(page, '.settings') })

  const cards = page.locator('.settings section.card')
  await cards.last().scrollIntoViewIfNeeded()
  await shot(cards.filter({ has: page.locator('.grid2') }).last(), 'settings-localization')
})
