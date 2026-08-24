import { test, expect } from '../fixtures.js'
import { t } from '../i18n.js'

// D9 (#2734). The unit tests cover the importer and the gallery in isolation,
// and the Go tests cover the endpoints; what only exists end to end is the loop
// the feature is for — save the open document as a template, then start a new
// document from it and find the text already there.
//
// Written as one flow rather than three specs on purpose: a template that can be
// created but not used, or used but comes back empty, is the failure this is
// meant to catch, and each half passes on its own.

// One content save, awaited. Autosave is debounced, so typing and asserting
// immediately races the request the assertion depends on.
async function typeAndSave(page, text) {
  const saved = page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
  const editor = page.locator('.ProseMirror')
  await editor.click()
  await page.keyboard.press('Control+End')
  await editor.pressSequentially(text)
  expect((await saved).status()).toBe(200)
}

test('документ: сохранение шаблона и создание документа по нему', async ({ page, seed }) => {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')

  await page.getByTestId('doc-new').click()
  await expect(page.locator('.ProseMirror')).toBeVisible()
  await typeAndSave(page, 'Повестка: сроки, риски, решения')

  // Save as template, then leave the document — the gallery must not depend on
  // the source document being open (or, later, on it existing at all).
  const created = page.waitForResponse(
    (r) => /\/document-templates$/.test(r.url()) && r.request().method() === 'POST',
  )
  await page.getByTestId('doc-save-template').click()
  expect((await created).status()).toBe(201)
  await page.getByTestId('doc-back').click()

  await page.getByTestId('doc-templates').click()
  const tiles = page.getByTestId('tpl-tile')
  // The saved template plus the three built-in starters, which ship with the
  // app and need no workspace row.
  await expect(tiles.first()).toBeVisible()
  expect(await tiles.count()).toBeGreaterThanOrEqual(4)

  const mine = tiles.filter({ hasText: 'Повестка' }).first()
  await mine.getByTestId('tpl-use').click()

  // The new document opens with the template's text already in it — this is the
  // whole point of the feature.
  await expect(page.locator('.ProseMirror')).toContainText('Повестка: сроки, риски, решения')
  await expect(page).toHaveURL(/\/documents\/[^/]+$/)

  // ...and it is a copy: editing it must not write back into the template.
  await typeAndSave(page, ' — дополнено в документе')
  await page.getByTestId('doc-back').click()
  await page.getByTestId('doc-templates').click()
  await expect(
    page.getByTestId('tpl-tile').filter({ hasText: 'дополнено в документе' }),
  ).toHaveCount(0)
})

test('документ: встроенный шаблон создаёт документ с телом', async ({ page, seed }) => {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByTestId('doc-templates').click()

  // A built-in has no row on the server: its body is markdown converted in the
  // browser and written right after the document is created. If that second
  // call were dropped, the document would open empty and look like a template
  // that "did not apply".
  const builtin = page.locator('[data-tpl="builtin:meeting"]').first()
  await builtin.getByTestId('tpl-use').click()

  // The body of a built-in lives in the locale catalog, so the assertion reads
  // its heading from there: on `E2E_LANG=en` the same template arrives as
  // "Meeting notes / Agenda".
  const agenda = t('documents.templates.meeting.body').match(/^## (.+)$/m)[1]
  await expect(page.locator('.ProseMirror')).toContainText(agenda)
  await expect(page.locator('.ProseMirror h1')).toContainText(
    t('documents.templates.meeting.title'),
  )
})
