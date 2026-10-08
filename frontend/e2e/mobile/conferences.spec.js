import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// The conference room at 390×844 (#2893, round 2 — item 4 of the report).
//
// Two breakages, and neither is a wide element the inventory tier could have
// caught by walking the routes: you have to be *inside* a room to see them, and
// getting there means joining a call. The media transport is expected to fail
// here (there is no SFU on this box) — that is fine and deliberate: the roster,
// the rail and the header are drawn by our own socket and the REST detail, and
// they are all this spec measures. Nothing below asserts on video.

const here = dirname(fileURLToPath(import.meta.url))
const SHOTS = resolve(here, '../.auth/mobile-shots')
mkdirSync(SHOTS, { recursive: true })

// Opens a freshly created conference and joins it. The workspace goes into
// localStorage before the first navigation for the same reason the desktop spec
// does it: without it the view has no scope, decides the conference is not
// visible and bounces to the list — which reads as "the room never rendered".
async function openRoom(page, backend, workspaceId, title) {
  const conf = await backend.post(`/workspaces/${workspaceId}/conferences`, { title })
  const id = conf.conference?.id || conf.id
  await page.addInitScript((wsId) => localStorage.setItem('tessera_ws', wsId), workspaceId)
  await page.goto(`/conferences/${id}`)
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.getByTestId('conference-join').click()
  await expect(page.getByTestId('conference-leave')).toBeVisible()
  await expect(page.getByTestId('conference-participants')).toBeVisible()
  return id
}

// Box of the first match, in viewport coordinates.
async function box(page, selector) {
  const b = await page.locator(selector).first().boundingBox()
  expect(b, `не найден элемент ${selector}`).not.toBeNull()
  return b
}

test('мобильная комната: шапка звонка в одну строку', async ({ page, seed, backend }) => {
  await openRoom(page, backend, seed.workspaceId, `Моб конф ${seed.runId}-head`)
  await page.screenshot({ path: resolve(SHOTS, 'conference-room.png'), fullPage: true })

  // v2 (#3107): the header of a call is back, the call's name and «Завершить»;
  // «Выйти» moved to the dock at the bottom. On a phone the header falls back
  // out of the topbar, and it has to stay ONE row (#2893, round 2: back above
  // the title and the buttons below it was the «слиплись» of the report).
  const back = await box(page, '[data-testid="conference-back"]')
  const title = await box(page, '.crumb-t')
  const end = await box(page, '[data-testid="conference-end"]')

  const mid = (b) => b.y + b.height / 2
  for (const [name, b] of [
    ['название', title],
    ['«Завершить»', end],
  ]) {
    expect(
      Math.abs(mid(b) - mid(back)),
      `${name} не на одной строке со стрелкой назад: центры разъехались на ` +
        `${Math.round(Math.abs(mid(b) - mid(back)))}px`,
    ).toBeLessThanOrEqual(6)
  }

  // …a row of separate controls, not a strip of touching ones…
  const inOrder = [back, title, end].sort((a, b) => a.x - b.x)
  for (let i = 1; i < inOrder.length; i++) {
    const gap = inOrder[i].x - (inOrder[i - 1].x + inOrder[i - 1].width)
    expect(
      gap,
      `соседние контролы в шапке слиплись: между ними ${Math.round(gap)}px`,
    ).toBeGreaterThanOrEqual(4)
  }
  expect(end.x + end.width, 'шапка звонка не помещается в экран').toBeLessThanOrEqual(393)

  // …and «Выйти» is on the dock, inside the screen.
  const leave = await box(page, '[data-testid="conference-leave"]')
  expect(leave.y + leave.height, '«Выйти» ниже экрана').toBeLessThanOrEqual(844)
  expect(leave.x + leave.width, '«Выйти» за правым краем').toBeLessThanOrEqual(393)
})

test('мобильная комната: ростер и сцена по ширине рабочей области', async ({
  page,
  seed,
  backend,
}) => {
  await openRoom(page, backend, seed.workspaceId, `Моб конф ${seed.runId}-body`)

  // The panel that «drifted right»: `width: 100%` plus its own padding and
  // border, with no global box-sizing to absorb them, made the rail 387px wide
  // inside a 369px column and pushed the pane to 399px.
  const m = await page.evaluate(() => {
    const pane = document.querySelector('.page-slot--mobile .n-layout-scroll-container')
    const r = document.querySelector('.rail').getBoundingClientRect()
    const body = document.querySelector('.body').getBoundingClientRect()
    const stage = document.querySelector('.stage-col').getBoundingClientRect()
    return {
      paneRight: pane.getBoundingClientRect().left + pane.clientWidth,
      railRight: r.right,
      railWidth: r.width,
      bodyWidth: body.width,
      stageWidth: stage.width,
    }
  })
  expect(
    Math.round(m.railRight),
    `панель участников уезжает за правый край рабочей области: ${Math.round(m.railRight)} ` +
      `при ${Math.round(m.paneRight)}`,
  ).toBeLessThanOrEqual(Math.round(m.paneRight) + 1)

  // The other half of the same `align-items: flex-start`: in a column it makes
  // the stage shrink to its content (126px next to a full-width rail) instead of
  // taking the width the phone has.
  expect(
    Math.round(m.stageWidth),
    `сцена сжата до ${Math.round(m.stageWidth)}px при ширине комнаты ${Math.round(m.bodyWidth)}px`,
  ).toBeGreaterThanOrEqual(Math.round(m.bodyWidth) - 1)

  await expectFitsWidth(page, 'комната конференции')
})
