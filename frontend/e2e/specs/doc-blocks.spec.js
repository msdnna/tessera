import { test, expect } from '../fixtures.js'
import { t } from '../i18n.js'

// D3 (#2728). The unit tests drive the slash plugin and the move command
// directly; what they cannot show is that either is reachable in a real
// browser — the menu has to open from actual keystrokes, and the reordered
// document has to be what the server ends up holding.

async function newDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByTestId('doc-new').click()
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

// The /rework on #2728: the handle was anchored to the top of the block's box.
// On a paragraph that is within a pixel of the first line, and a 1px CSS nudge
// hid the rest; on a heading the box opens a 14px margin above the text, and the
// handle visibly floated. A spec without the heading would be decorative — the
// paragraph passed before the fix too.
test('документ: ручка блока центрирована по первой строке и абзаца, и заголовка', async ({
  page,
  seed,
}) => {
  const editor = await newDocument(page, seed)
  // One word: the slash query stops at the first space (slashMenu.js), so
  // "/заголовок 1" would close the menu instead of narrowing it. "заголовок"
  // leaves h1..h3, and h1 is the highlighted one.
  await editor.pressSequentially('/заголовок')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('заголовок документа')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('обычный абзац')

  const gutter = page.locator('.doc-gutter')

  for (const selector of ['h1', 'p']) {
    const block = editor.locator(selector).first()
    await block.hover()
    await expect(gutter).toBeVisible()
    // The handle now slides between blocks over 120ms (the /rework on #2728), so
    // a single measurement catches it mid-flight on every block but the first.
    // Polled, not slept through: the claim is about where the handle SETTLES, and
    // a handle that settles off the line still fails — it just gets 5s to arrive.
    await expect
      .poll(async () => {
        const row = await gutter.boundingBox()
        // The first line's own box, not the block's: a heading's box starts above
        // its text, which is exactly what used to throw the handle off.
        const line = await block.evaluate((el) => {
          const r = el.getClientRects()[0]
          return { top: r.top, bottom: r.bottom }
        })
        return Math.abs(row.y + row.height / 2 - (line.top + line.bottom) / 2)
      })
      .toBeLessThan(2)
  }
})

test('документ: чекбокс стоит на одной линии со своим текстом', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)
  // "чекбокс" is a keyword of the task list and of nothing else, and it is one
  // word — the slash query ends at the first space.
  await editor.pressSequentially('/чекбокс')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('пункт списка задач')

  const box = editor.locator('ul[data-type="taskList"] li input[type="checkbox"]')
  const text = editor.locator('ul[data-type="taskList"] li p').first()
  const b = await box.boundingBox()
  const t = await text.boundingBox()
  expect(Math.abs(b.y + b.height / 2 - (t.y + t.height / 2))).toBeLessThan(2)
})

// The hint used to fight the label for one line and win, truncating it to
// "Список с то…". Stacked, both are whole — and the groups are what the /rework
// asked for on top of that.
test('документ: меню вставки сгруппировано и не обрезает названия', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)
  await editor.pressSequentially('/')

  const menu = page.locator('.slash-menu')
  await expect(menu).toBeVisible()
  await expect(menu.locator('.slash-group-title')).toHaveText([
    t('documents.slash.group.text'),
    t('documents.slash.group.lists'),
    t('documents.slash.group.insert'),
    t('documents.slash.group.upload'),
  ])

  const label = menu.locator('[data-slash="bulletList"] .slash-label')
  await expect(label).toBeVisible()
  const clipped = await label.evaluate((el) => el.scrollWidth > el.clientWidth + 1)
  expect(clipped).toBe(false)

  // Filtering leaves only the groups that still have entries in them. The query
  // is a keyword rather than a piece of the label: keywords are deliberately not
  // translated (docSlash.js) — they are the typing aliases, so "list" narrows the
  // menu in either language and the spec does not need a per-locale query.
  await editor.pressSequentially('list')
  await expect(menu.locator('.slash-group-title')).toHaveText([t('documents.slash.group.lists')])
})
