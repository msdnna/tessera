import { test, expect } from '../fixtures.js'

// D3 (#2728). The unit tests drive the slash plugin and the move command
// directly; what they cannot show is that either is reachable in a real
// browser — the menu has to open from actual keystrokes, and the reordered
// document has to be what the server ends up holding.

async function newDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByRole('button', { name: /Новый документ/ }).click()
  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()
  await editor.click()
  return editor
}

// The debounce is 800ms, so the wait has to be armed before the keystrokes that
// trigger it — see documents.spec.js.
function waitForSave(page) {
  return page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
}

test('документ: slash-меню вставляет блок и уносит с собой набранную команду', async ({
  page,
  seed,
}) => {
  const editor = await newDocument(page, seed)

  await editor.pressSequentially('/табл')
  const menu = page.locator('.slash-menu')
  await expect(menu).toBeVisible()
  await expect(menu.locator('.slash-item')).toHaveCount(1)

  const saved = waitForSave(page)
  await page.keyboard.press('Enter')

  await expect(editor.locator('table')).toBeVisible()
  // The command text itself must not survive — leaving "/табл" in the document
  // is the classic slash-menu bug.
  await expect(editor).not.toContainText('/табл')
  await expect(menu).toBeHidden()
  expect((await saved).status()).toBe(200)
})

test('документ: меню закрывается по Escape и не мешает писать', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)

  await editor.pressSequentially('/таб')
  await expect(page.locator('.slash-menu')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.locator('.slash-menu')).toBeHidden()

  // Typing on must not pop the menu back up, and the text must read as typed.
  await editor.pressSequentially('лица заказов')
  await expect(page.locator('.slash-menu')).toBeHidden()
  await expect(editor).toContainText('/таблица заказов')
})

test('документ: абзац переезжает с клавиатуры и переживает перезагрузку', async ({
  page,
  seed,
}) => {
  const editor = await newDocument(page, seed)

  await editor.pressSequentially('первый')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('второй')

  // Both moves are driven from where the caret already is, and the second one
  // starts from the selection the first move left behind. That is deliberate:
  // an arrow key moves the caret natively and ProseMirror only learns about it
  // from the asynchronous selectionchange, so a chord sent microseconds later
  // (which only a test can do) reads the previous block index and the move
  // looks like it did nothing.
  const saved = waitForSave(page)
  await page.keyboard.press('Alt+Shift+ArrowUp')
  await expect(editor.locator('p')).toHaveText(['второй', 'первый'])
  await page.keyboard.press('Alt+Shift+ArrowDown')
  await expect(editor.locator('p')).toHaveText(['первый', 'второй'])
  await page.keyboard.press('Alt+Shift+ArrowUp')
  await expect(editor.locator('p')).toHaveText(['второй', 'первый'])
  expect((await saved).status()).toBe(200)

  await page.reload()
  await expect(page.locator('.ProseMirror p')).toHaveText(['второй', 'первый'])
})

test('документ: ручка блока появляется по наведению', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)
  await editor.pressSequentially('абзац с ручкой')

  const gutter = page.locator('.doc-gutter')
  await expect(gutter).toBeHidden()
  await editor.locator('p').first().hover()
  await expect(gutter).toBeVisible()
  // draggable is what hands the block to ProseMirror's own drag machinery.
  await expect(gutter.locator('.grip')).toHaveAttribute('draggable', 'true')
})

// The sheet (задача 2727) is the first thing in this editor with a width of its
// own, and it moved the handle: the old `left: 0` anchor was the work area's
// edge, which is nowhere near the text once the sheet is centred. Geometry is
// checked in the browser because that is the only place it exists — the unit
// test can only see that the rules were written.
test('документ: лист центрирован, ручка стоит у текста', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)
  await editor.pressSequentially('абзац на листе')

  const work = await page.locator('.doc-content').boundingBox()
  const sheet = await editor.boundingBox()
  expect(sheet.width).toBeLessThan(work.width)
  // Centred: the margins on both sides match to within a pixel.
  const left = sheet.x - work.x
  const right = work.x + work.width - (sheet.x + sheet.width)
  expect(Math.abs(left - right)).toBeLessThan(2)

  await editor.locator('p').first().hover()
  const handle = await page.locator('.doc-gutter').boundingBox()
  const text = await editor.locator('p').first().boundingBox()
  // Inside the sheet, in the lane its left padding leaves — not out in the work
  // area, and not on top of the first characters.
  expect(handle.x).toBeGreaterThanOrEqual(sheet.x)
  expect(handle.x + handle.width).toBeLessThanOrEqual(text.x)
})
