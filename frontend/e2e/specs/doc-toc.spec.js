import { test, expect } from '../fixtures.js'

// D8 (#2733), the "оглавление и внутренние ссылки" half of item 1 of #2718.
//
// The unit tests cover what the outline is derived from (utils/docToc.js) and
// what a click on an internal link does to the editor state
// (ut-docInternalLink.spec.js). Neither can show the promise the feature is
// made of: that pressing an entry actually brings that part of the document
// into view. A panel that lists the headings, highlights the right one and
// scrolls nowhere passes every cheaper check while being exactly the bug a
// reader would report — so this spec asserts on the viewport, not on the
// chrome.

function waitForSave(page) {
  return page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
}

async function newDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByRole('button', { name: /Новый документ/ }).click()
  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()
  await editor.click()
  return editor
}

// A document tall enough for the second heading to start off-screen. The
// viewport is shrunk instead of typing hundreds of lines: what has to be true
// is "the target was not visible and then it was", and that holds at any height.
//
// The first save is awaited on its own before the rest is typed. Saving a
// still-untitled document gives it its slug, and the URL change that follows
// remounts this view — a panel opened before that point is thrown away with it,
// and the failure reads as "the outline does not open".
async function typeLongDocument(page) {
  await page.setViewportSize({ width: 1000, height: 400 })
  const editor = page.locator('.ProseMirror')

  const firstSave = waitForSave(page)
  await editor.pressSequentially('# Введение')
  await page.keyboard.press('Enter')
  expect((await firstSave).status()).toBe(200)

  const saved = waitForSave(page)
  await editor.click()
  await page.keyboard.press('Control+End')
  for (let i = 1; i <= 14; i += 1) {
    await editor.pressSequentially(`Пункт ${i}`)
    await page.keyboard.press('Enter')
  }
  await editor.pressSequentially('## Заключение')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('Итоговый абзац')
  expect((await saved).status()).toBe(200)
  return editor
}

test('документ: оглавление собирается из заголовков и уносит к нужному разделу', async ({
  page,
  seed,
}) => {
  await newDocument(page, seed)
  const editor = await typeLongDocument(page)

  await page.getByTestId('doc-toc-toggle').click()

  // Since #2728 the outline is a rail of ticks that opens on hover, so the
  // titles are behind a hover rather than in a column of their own. The ticks
  // are asserted first: they are the collapsed state a reader actually sees,
  // and a rail that draws nothing would otherwise leave nothing to hover.
  await expect(page.getByTestId('doc-toc-tick')).toHaveCount(2)
  await page.getByTestId('doc-toc').hover()

  const entries = page.getByTestId('doc-toc-entry')
  await expect(entries).toHaveCount(2)
  await expect(entries.nth(0)).toHaveText('Введение')
  await expect(entries.nth(1)).toHaveText('Заключение')

  // The caret is in the last paragraph, which belongs to «Заключение» — the
  // outline says where you are, not only where you can go.
  await expect(entries.nth(1)).toHaveClass(/active/)

  const intro = editor.locator('h1', { hasText: 'Введение' })
  const conclusion = editor.locator('h2', { hasText: 'Заключение' })
  await expect(intro).not.toBeInViewport()

  await entries.nth(0).click()
  // This is the assertion the whole feature comes down to.
  await expect(intro).toBeInViewport()
  // …and the arrival is marked, so a scroll that lands mid-page still says what
  // it landed on.
  await expect(intro).toHaveClass(/doc-block-target/)

  await entries.nth(1).click()
  await expect(conclusion).toBeInViewport()
})

test('документ: внутренняя ссылка на раздел ведёт внутрь документа', async ({ page, seed }) => {
  await newDocument(page, seed)
  const editor = await typeLongDocument(page)

  // Link the first item to «Заключение» through the heading picker — the only
  // path in the UI that produces an internal link. Clicking the paragraph also
  // scrolls the page back to the top, which is what puts the target out of view
  // for the assertion below.
  await editor.locator('p', { hasText: /^Пункт 1$/ }).click()
  await page.keyboard.press('Home')
  await page.keyboard.press('Shift+End')
  await page.locator('.doc-tbtn[title="Ссылка"]').click()
  await page.getByTestId('doc-link-heading').click()
  await page.locator('.n-base-select-option', { hasText: 'Заключение' }).click()
  // The picker fills the URL field rather than applying straight away, so the
  // chosen target is visible before it is committed.
  await expect(page.getByTestId('doc-link-href').locator('input')).toHaveValue(/^#.+/)
  await page.getByTitle('Применить').click()

  const link = editor.locator('a[href^="#"]').first()
  await expect(link).toBeVisible()

  const conclusion = editor.locator('h2', { hasText: 'Заключение' })
  await expect(conclusion).not.toBeInViewport()

  // Ctrl+click: inside the editor a plain click has to keep placing the caret,
  // because the link is text somebody is still writing.
  await link.click({ modifiers: ['Control'] })
  await expect(conclusion).toBeInViewport()
  await expect(conclusion).toHaveClass(/doc-block-target/)

  // The jump must stay inside the document: a bare "#id" that the browser was
  // allowed to follow would push a fragment onto the SPA's URL.
  expect(new URL(page.url()).hash).toBe('')
})
