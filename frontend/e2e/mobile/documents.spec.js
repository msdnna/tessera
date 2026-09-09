import { mkdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { test, expect } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// Item 3 of the #2893 rework: «не проработаны Документы на мобилке».
//
// Three separate defects hide behind that sentence, and none of them is a width
// overflow — which is why the first mobile tier, whose only gate is «страница не
// скроллится вбок», passed the document screen without a word:
//
//   1. the outline and the discussion are columns of a flex row, so on a phone
//      they simply stack under the text. The document ends up two panels deep
//      and there is no way to put either of them away;
//   2. the outline's popover is anchored `right: 100%` off a 22px rail. On a
//      wide screen that rail is far from the left edge; on a phone it is AT the
//      left edge, so the list opens at x ≈ -200 — outside the viewport, exactly
//      the second screenshot in the report;
//   3. the toolbar wraps into three rows of ~13 buttons, which pushes the text
//      itself below the fold.
//
// Each gets its own assertion on the box that actually breaks. A page-width gate
// would stay green through all three.

const here = dirname(fileURLToPath(import.meta.url))
const SHOTS = resolve(here, '../.auth/mobile-shots')
mkdirSync(SHOTS, { recursive: true })

function waitForSave(page) {
  return page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
}

// A document with headings, so the outline has rows to draw. The first save is
// awaited on its own: saving an untitled document assigns its slug, and the URL
// change that follows remounts the view — a panel opened before that point is
// thrown away with it (same trap as e2e/specs/doc-toc.spec.js).
async function openDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByTestId('doc-new').click()
  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()
  await editor.click()

  const firstSave = waitForSave(page)
  await editor.pressSequentially('# Введение')
  await page.keyboard.press('Enter')
  expect((await firstSave).status()).toBe(200)

  const saved = waitForSave(page)
  await editor.click()
  await page.keyboard.press('Control+End')
  await editor.pressSequentially('Текст раздела')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('## Заключение')
  await page.keyboard.press('Enter')
  expect((await saved).status()).toBe(200)

  await page.waitForTimeout(300)
  return editor
}

// How many rows the toolbar wrapped into, counted off the buttons themselves
// rather than off a class: wrapping leaves no mark in the DOM, so the only
// honest measure is how many distinct tops the buttons sit at.
function toolbarRows(page) {
  return page.evaluate(() => {
    const tops = new Set()
    for (const b of document.querySelectorAll('.doc-toolbar .doc-tbtn')) {
      tops.add(Math.round(b.getBoundingClientRect().top))
    }
    return tops.size
  })
}

test('мобильные документы: панели не стекаются под текст, оглавление не уезжает за экран', async ({
  page,
  seed,
}) => {
  await openDocument(page, seed)

  // --- 1. On arrival the text is the only thing on screen -------------------
  // Both panels are sheets here, so a document that opened with them on would
  // show none of itself. This is the assertion that fails on the old code: there
  // tocOpen/commentsOpen are `true` from the start and both are in the row.
  await expect(page.getByTestId('doc-toc')).toHaveCount(0)
  await expect(page.locator('.doc-comments')).toHaveCount(0)
  await expectFitsWidth(page, 'документ на 393px')

  // --- 2. The toolbar is one row -------------------------------------------
  const collapsed = await toolbarRows(page)
  expect(collapsed, `тулбар документа занимает ${collapsed} ряда на 393px вместо одного`).toBe(1)
  // The groups behind the toggle are genuinely gone, not merely narrower.
  await expect(page.locator('.doc-toolbar [data-tbtn="table"]')).toHaveCount(0)
  await page.screenshot({ path: resolve(SHOTS, 'doc-mobile-arrival.png'), fullPage: false })

  // ...and it gives them back.
  await page.getByTestId('doc-toolbar-more').tap()
  await page.waitForTimeout(200)
  await expect(page.locator('.doc-toolbar [data-tbtn="table"]')).toBeVisible()
  expect(await toolbarRows(page)).toBeGreaterThan(1)
  await page.getByTestId('doc-toolbar-more').tap()
  await page.waitForTimeout(200)
  expect(await toolbarRows(page)).toBe(1)

  // --- 3. The outline opens ON the screen -----------------------------------
  await page.getByTestId('doc-toc-toggle').tap()
  const flyout = page.getByTestId('doc-toc-flyout')
  await expect(flyout).toBeVisible()
  await page.waitForTimeout(200)

  const vw = page.viewportSize().width
  const box = await flyout.boundingBox()
  expect(
    box.x,
    `оглавление открывается за левым краем экрана: x=${Math.round(box.x)} при вьюпорте ${vw}px`,
  ).toBeGreaterThanOrEqual(0)
  expect(
    Math.round(box.x + box.width),
    `оглавление уходит за правый край: right=${Math.round(box.x + box.width)} при ${vw}px`,
  ).toBeLessThanOrEqual(vw + 1)
  // The entries are reachable, not just the frame.
  await expect(page.getByTestId('doc-toc-entry').first()).toBeVisible()
  await expectFitsWidth(page, 'документ с открытым оглавлением')
  await page.screenshot({ path: resolve(SHOTS, 'doc-mobile-toc.png'), fullPage: false })

  // --- 4. The discussion is a sheet OVER the text, and takes the outline's turn
  await page.getByTestId('doc-comments-toggle').tap()
  await page.waitForTimeout(300)
  await expect(page.getByTestId('doc-toc')).toHaveCount(0)

  const panel = page.locator('.doc-comments')
  await expect(panel).toBeVisible()
  const [panelBox, workBox] = await Promise.all([
    panel.boundingBox(),
    page.locator('.work').boundingBox(),
  ])
  // Measured against `.work` and NOT against `.ProseMirror`: the editor is an A4
  // page inside a scroller, so its layout box runs a thousand pixels past the
  // fold. «Panel starts above the bottom of the editor» is therefore true even
  // when the panel is stacked underneath — the assertion this replaces passed
  // with the fix reverted. The working area is the box that is actually bounded
  // by the phone screen, so covering it is a claim only a sheet can satisfy.
  expect(
    Math.round(panelBox.y - workBox.y),
    `обсуждение стоит под текстом, а не поверх него: панель начинается на ` +
      `${Math.round(panelBox.y - workBox.y)}px ниже верха рабочей области`,
  ).toBeLessThanOrEqual(2)
  expect(
    Math.round(panelBox.height),
    `обсуждение занимает только ${Math.round(panelBox.height)}px из ` +
      `${Math.round(workBox.height)}px рабочей области — это полоса снизу, а не лист поверх текста`,
  ).toBeGreaterThanOrEqual(Math.round(workBox.height) - 2)
  await expectFitsWidth(page, 'документ с открытым обсуждением')
  await page.screenshot({ path: resolve(SHOTS, 'doc-mobile-comments.png'), fullPage: false })

  // --- 5. The sheet can be dismissed from itself ----------------------------
  // The toggle that opened it is in the top bar, which the sheet covers.
  await page.getByTestId('doc-comments-close').tap()
  await page.waitForTimeout(200)
  await expect(page.locator('.doc-comments')).toHaveCount(0)
  await expect(page.locator('.ProseMirror')).toBeVisible()
})
