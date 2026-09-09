import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect, openBoard, cardsIn } from '../fixtures.js'
import { expectFitsWidth, expectPaneFitsWidth, scanLayout } from './_layout.js'

// Mobile-layout inventory (#2893) — the whole app at 390×844 with touch.
//
// This tier answers one question per screen: does it fit? Every spec also drops
// a screenshot and prints its scan, because the assertion and the evidence answer
// different questions — a red assertion says "«Документы» шире на 42px", and the
// shot next to it says which control is the one sticking out. That is how the
// three fixes in this branch were found; it is also the only way the *advisory*
// findings (squeezed controls, small tap targets) surface at all, since they cost
// the page no width and no gate here can see them.

const here = dirname(fileURLToPath(import.meta.url))
const SHOTS = resolve(here, '../.auth/mobile-shots')
mkdirSync(SHOTS, { recursive: true })

// settle waits for the mobile shell, not just for the URL. `.page-slot--mobile`
// only exists in AppLayout's `v-else` branch, so seeing it also proves the app
// actually took the mobile path — a viewport that silently rendered the desktop
// layout would make every assertion below meaningless (and mostly passing).
async function settle(page) {
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(400)
}

async function visit(page, path) {
  await page.goto(path)
  await settle(page)
}

// report attaches the raw scan so a failure in CI carries its own evidence, and
// prints the advisory tap-target count. Small tap targets do NOT fail the run:
// see the note in _layout.js — this tier's only hard gate is the page width.
async function report(page, testInfo, label) {
  const scan = await scanLayout(page)
  const shot = resolve(SHOTS, `${label}.png`)
  await page.screenshot({ path: shot, fullPage: true })
  await testInfo.attach(`scan-${label}`, {
    body: JSON.stringify(scan, null, 2),
    contentType: 'application/json',
  })
  console.log(
    `[mobile] ${label}: doc=${scan.docScrollW}/${scan.vw}` +
      ` pane=${scan.pane ? `${scan.pane.scrollW}/${scan.pane.clientW}` : '—'}` +
      ` wide=${scan.wideCount} cut=${scan.truncatedCount} smallTaps=${scan.smallTapCount}`,
  )
  for (const t of scan.truncated)
    console.log(`[mobile]   cut ${t.el} +${t.over}px (${t.scrollW}/${t.clientW})`)
  return scan
}

// Negative control, and it stays in the suite rather than being run once by hand:
// every assertion below passes today, so without this the whole tier is
// indistinguishable from ten `expect(true)`s. Plant a box 200px wider than the
// viewport in the real app and both gates must go red.
test('контроль: подсаженный перебор ширины ловится', async ({ page }) => {
  await visit(page, '/')
  await expectFitsWidth(page, 'контроль до подсадки') // clean beforehand

  await page.evaluate(() => {
    const wide = document.createElement('div')
    wide.id = 'e2e-overflow-probe'
    wide.style.cssText = 'width:600px;height:20px;background:red'
    // Into the scroll container itself, not into `.page-slot--mobile` around it:
    // the outer box clips, so a wide child there changes neither measurement and
    // the control would "fail" for its own reason instead of the gate's.
    const pane = document.querySelector('.page-slot--mobile .n-layout-scroll-container')
    pane.appendChild(wide)
    const cut = document.createElement('div')
    cut.style.cssText = 'overflow-x:hidden;width:100px'
    cut.appendChild(
      Object.assign(document.createElement('div'), { style: 'width:400px;height:10px' }),
    )
    pane.appendChild(cut)
  })

  await expect(expectFitsWidth(page, 'контроль')).rejects.toThrow(/шире экрана|скроллится вбок/)
  const scan = await scanLayout(page)
  expect(scan.pane.scrollW).toBeGreaterThan(scan.pane.clientW)
  expect(scan.truncatedCount).toBeGreaterThan(0)
})

// Screens reachable by URL alone. Each is its own test so one broken screen does
// not hide the state of the eight behind it — the whole point of an inventory.
const ROUTES = [
  { label: 'home', path: '/' },
  { label: 'notes', path: '/notes' },
  { label: 'documents', path: '/documents' },
  { label: 'reminders', path: '/reminders' },
  { label: 'milestones', path: '/milestones' },
  { label: 'settings', path: '/settings' },
  { label: 'conferences', path: '/conferences' },
]

for (const route of ROUTES) {
  test(`мобильная вёрстка: ${route.label} (${route.path})`, async ({ page }, testInfo) => {
    await visit(page, route.path)
    await report(page, testInfo, route.label)
    await expectFitsWidth(page, route.label)
  })
}

// The failure mode the width gates structurally cannot see: a flex row that
// *shrinks* rather than overflows. Squeezing costs the page no width, so every
// gate above stays green while the control becomes unusable — on «Напоминания»
// the date picker was down to 43px of client width, showing «Выбра…» and an
// icon. Asserted on the picker's own box, not on a screenshot, and with a floor
// (150px) well under the 200px the fix sets, so ordinary restyling of the row
// does not make this red.
test('мобильная вёрстка: строка добавления напоминания не сплющена', async ({ page }) => {
  await visit(page, '/reminders')
  const picker = page.locator('.add-row .n-date-picker').first()
  await expect(picker).toBeVisible()
  const w = await picker.evaluate((el) => el.getBoundingClientRect().width)
  expect(w, `поле даты сжато до ${Math.round(w)}px — дату в нём не прочитать`).toBeGreaterThan(150)
})

test('мобильная вёрстка: доска', async ({ page, seed }, testInfo) => {
  await openBoard(page, seed.boardId)
  await settle(page)
  await report(page, testInfo, 'board')
  await expectFitsWidth(page, 'доска')
})

test('мобильная вёрстка: модалка задачи', async ({ page, seed, backend }, testInfo) => {
  const title = `Моб ${Date.now().toString(36)}`
  await backend.createTask(seed.columns[0].id, title, {
    description: [
      'Длинная строка без пробелов, чтобы поймать перенос: ',
      'ААААААААААААААААААААААААААААААААААААААААААААААААААААААААААА',
      '\n\n| колонка один | колонка два | колонка три | колонка четыре |',
      '\n| --- | --- | --- | --- |',
      '\n| значение | значение | значение | значение |',
    ].join(''),
  })
  await openBoard(page, seed.boardId)
  await settle(page)

  await cardsIn(page, seed.columns[0].name).filter({ hasText: title }).first().click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 10000 })
  await page.waitForTimeout(500)

  await report(page, testInfo, 'task-modal')
  // Both levels: the page must not scroll sideways, and the modal's own body must
  // not be wider than the modal. A modal inside a clipping ancestor can cut its
  // content off while the page stays perfectly honest.
  await expectFitsWidth(page, 'модалка задачи')
  await expectPaneFitsWidth(modal, 'модалка задачи')
})

test('мобильная вёрстка: сайдбар в дровере', async ({ page }, testInfo) => {
  await visit(page, '/')
  // Topbar.vue renders the hamburger only in its `mobile` branch, as `.menu-btn`.
  await page.locator('.menu-btn').first().click()
  const drawer = page.locator('.n-drawer')
  await expect(drawer).toBeVisible({ timeout: 10000 })
  await page.waitForTimeout(400)

  await report(page, testInfo, 'sidebar-drawer')
  await expectFitsWidth(page, 'сайдбар в дровере')
  await expectPaneFitsWidth(
    drawer.locator('.n-drawer-body-content-wrapper').first(),
    'сайдбар в дровере',
  )
})
