import { test, expect } from '../fixtures.js'
import { minimalPdf } from '../pdfFixture.js'

// D8 (#2733), the "PDF (чтение)" half of item 1 of #2718.
//
// The unit tests cover utils/docPdf.js, which is written free of pdf.js on
// purpose so it stays testable without a canvas. That leaves the actual promise
// of the feature — a PDF is readable inside the document — shown nowhere. This
// spec is the only place it is, so it asserts on rendered pixels rather than on
// the presence of the viewer chrome: a viewer that mounts, reports "1 / 1" and
// paints a blank canvas passes every cheaper check while being exactly the bug
// a reader would report.

async function newDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByTestId('doc-new').click()
  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()
  await editor.click()
  return editor
}

// True when the page canvas has at least one pixel that is not the white the
// viewer clears it to. Reading the bitmap is the point: it is the difference
// between "pdf.js ran" and "pdf.js drew the document".
async function canvasHasInk(page) {
  return page.evaluate(() => {
    const canvas = document.querySelector('[data-pdf-page="1"] canvas')
    if (!canvas || !canvas.width || !canvas.height) return { ok: false, reason: 'нет канваса' }
    const { data } = canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height)
    for (let i = 0; i < data.length; i += 4) {
      if (data[i + 3] !== 0 && (data[i] < 200 || data[i + 1] < 200 || data[i + 2] < 200)) {
        return { ok: true }
      }
    }
    return { ok: false, reason: 'канвас пуст' }
  })
}

test('документ: PDF вставляется, читается и переживает перезагрузку', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)

  await editor.pressSequentially('/pdf')
  await expect(page.locator('.slash-menu')).toBeVisible()

  // The slash entry is `external`: it hands off to the hidden picker instead of
  // inserting a node, so the file is supplied to that input directly.
  await page.keyboard.press('Enter')
  await page.setInputFiles('input[accept="application/pdf,.pdf"]', {
    name: 'fixture.pdf',
    mimeType: 'application/pdf',
    buffer: minimalPdf('Tessera PDF fixture'),
  })

  const viewer = page.getByTestId('doc-pdf')
  await expect(viewer).toBeVisible()
  await expect(page.getByTestId('doc-pdf-error')).toHaveCount(0)
  await expect(page.getByTestId('doc-pdf-pages')).toHaveText(/1\s*\/\s*1/)
  await expect(viewer).toContainText('fixture.pdf')

  await expect.poll(async () => (await canvasHasInk(page)).ok, { timeout: 15000 }).toBe(true)

  // The block has to be part of the document, not a client-side flourish: only
  // a reload proves the server stored the pdfEmbed node and the asset is served
  // back from our own origin.
  await expect(page).toHaveURL(/\/documents\/[^/]+$/)
  await page.reload()
  await expect(page.getByTestId('doc-pdf')).toBeVisible()
  await expect(page.getByTestId('doc-pdf-error')).toHaveCount(0)
  await expect.poll(async () => (await canvasHasInk(page)).ok, { timeout: 15000 }).toBe(true)
})
