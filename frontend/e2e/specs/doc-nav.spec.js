import { test, expect } from '../fixtures.js'

// The navigation half of the #2727 rework. Both halves of the bug were about the
// app frame around the document, not about the document — and neither is visible
// to a unit test that mounts the view on its own:
//
//   - opening a document put out the «Документы» light in the sidebar, so the
//     open section was the one the app claimed you were not in;
//   - pressing that item with a document open changed the URL and nothing else:
//     the document stayed on screen, and the only way back to the list was the
//     browser's reload.
//
// cx-router.spec.js asserts the route table that makes the first one possible
// (one record for list and document). What it cannot show is that the item
// actually lights up and that the click actually lands on the list — the first
// depends on the sidebar's link, the second on this view reacting to a route
// change it is no longer remounted by. Hence a browser.

// The document is created and then saved, because the slug only exists after the
// first save: before it the URL is /documents with no param, which is the state
// where the bug cannot be reproduced.
async function openSavedDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByTestId('doc-new').click()

  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()
  const saved = page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
  await editor.click()
  await editor.pressSequentially('Протокол встречи')
  expect((await saved).status()).toBe(200)
  await expect(page).toHaveURL(/\/documents\/[^/]+$/)
  return editor
}

const sidebarDocuments = (page) => page.locator('.nav a[href="/documents"]')

test('документ: раздел в сайдбаре остаётся активным и возвращает к списку', async ({
  page,
  seed,
}) => {
  await openSavedDocument(page, seed)

  // The state the user reported as "раздел Документы становится неактивным".
  await expect(sidebarDocuments(page)).toHaveClass(/router-link-active/)

  await sidebarDocuments(page).click()
  await expect(page).toHaveURL(/\/documents$/)
  // The list, not the document: the URL changing on its own was the bug.
  await expect(page.getByTestId('doc-new')).toBeVisible()
  await expect(page.locator('.ProseMirror')).toHaveCount(0)
})

test('документ: браузерное «назад» тоже возвращает к списку', async ({ page, seed }) => {
  await openSavedDocument(page, seed)

  // Same code path as the sidebar click, reached without a click on the app —
  // the view now has to answer a route change instead of being replaced by one.
  await page.goBack()
  await expect(page).toHaveURL(/\/documents$/)
  await expect(page.getByTestId('doc-new')).toBeVisible()
  await expect(page.locator('.ProseMirror')).toHaveCount(0)
})

test('документ: управление живёт в шапке приложения, а не в рабочей области', async ({
  page,
  seed,
}) => {
  await openSavedDocument(page, seed)

  // «К списку» and the document's controls were asked to move into the header
  // (they teleport there on a wide screen). Asserting the ancestor rather than
  // mere visibility: rendered twice, or left behind in the page, both read as
  // "visible" while being exactly what the move was meant to end.
  const back = page.getByTestId('doc-back')
  await expect(back).toHaveCount(1)
  await expect(page.locator('.topbar').getByTestId('doc-back')).toBeVisible()
  await expect(page.locator('.topbar').getByTestId('doc-export')).toBeVisible()

  // Delete and «Вложенный документ» left the bottom of the page for this menu:
  // they exist only as entries of it, never as buttons in the working area.
  await expect(page.getByTestId('doc-action-remove')).toHaveCount(0)
  await page.locator('.topbar').getByTestId('doc-actions').click()
  await expect(page.getByTestId('doc-action-nested')).toBeVisible()
  await expect(page.getByTestId('doc-action-remove')).toBeVisible()
})
