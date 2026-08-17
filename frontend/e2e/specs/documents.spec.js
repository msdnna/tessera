import { test, expect } from '../fixtures.js'

// The point of D2 (#2727) is that typing is enough — there is no save button.
// So the check that matters is the round trip through the server: type, let the
// debounce fire, reload the page and find the text again. Asserting on the
// editor's own DOM right after typing would pass even if nothing was persisted.
test('документ: набранный текст сохраняется сам и переживает перезагрузку', async ({
  page,
  seed,
}) => {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')

  await page.getByRole('button', { name: /Новый документ/ }).click()
  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()

  const text = `Автосохранение ${seed.runId}`

  // Arm the wait before typing: the debounce is 800ms, so the request can land
  // before an assertion written after the keystrokes gets a chance to run.
  const saved = page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
  await editor.click()
  await editor.pressSequentially(text)
  expect((await saved).status()).toBe(200)
  await expect(page.getByText('Все изменения сохранены')).toBeVisible()

  // open() rewrites the URL to /documents/<slug>, so the reload also exercises
  // the deep link: the document is resolved by slug, not restored from memory.
  await expect(page).toHaveURL(/\/documents\/[^/]+$/)
  await page.reload()
  await expect(page.locator('.ProseMirror')).toContainText(text)
})
