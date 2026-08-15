import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// D4 (#2729). The unit tests drive the presence composable and the lock plugin
// directly, and the Go tests drive the room; none of them can show that the two
// halves meet in a browser. What only a real pair of sessions proves: the second
// person shows up in the header, the block they take is painted for everyone
// else, and the refusal reaches the person who tried to type into it.
//
// Two *different* users, not two tabs of one: the roster deliberately collapses
// one person's tabs into a single viewer and hides the viewer that is you, so a
// same-user pair would assert nothing.

// mateContext registers a second member of the seeded workspace and returns a
// signed-in page for them.
async function mateContext(browser, seed, baseURL) {
  const creds = newCredentials(seed.runId, '-mate')
  await register(creds)
  const context = await browser.newContext({ baseURL })
  const page = await context.newPage()
  await signIn(page, creds)
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  return { context, page, creds }
}

test('документ: второй участник виден в шапке, а занятый блок закрыт для правки', async ({
  page,
  browser,
  backend,
  seed,
}) => {
  const baseURL = new URL(page.url() || 'http://localhost:4174').origin

  // The mate has to be a member before they can open the document at all — the
  // socket checks workspace membership before it upgrades.
  const { context, page: matePage, creds } = await mateContext(browser, seed, baseURL)
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: creds.email })

  try {
    await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
    await page.goto('/documents')
    await page.getByRole('button', { name: /Новый документ/ }).click()

    const editor = page.locator('.ProseMirror')
    await expect(editor).toBeVisible()

    // Arm the save before typing: the autosave debounce is 800ms.
    const saved = page.waitForResponse(
      (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
    )
    await editor.click()
    await editor.pressSequentially('Общий абзац')
    expect((await saved).status()).toBe(200)
    await expect(page).toHaveURL(/\/documents\/[^/]+$/)
    const docURL = new URL(page.url()).pathname

    // The author is holding the paragraph right now (the caret is in it), so
    // release it before the mate arrives — otherwise the test would be asserting
    // on whichever claim happened to land first.
    await page.getByRole('button', { name: /К списку/ }).click()
    await page.goto(docURL)
    await expect(page.locator('.ProseMirror')).toContainText('Общий абзац')

    await matePage.goto(docURL)
    await expect(matePage.locator('.ProseMirror')).toContainText('Общий абзац')

    // 1. Presence: the author sees an avatar for the mate.
    const avatar = page.locator('.viewer')
    await expect(avatar).toHaveCount(1)

    // 2. The mate puts the caret in the paragraph, which claims it.
    await matePage.locator('.ProseMirror p').first().click()

    // The author's copy of that block is painted, and the avatar switches from
    // "reading" to "editing".
    await expect(page.locator('.ProseMirror .doc-block-locked')).toHaveCount(1)
    await expect(avatar).toHaveClass(/editing/)

    // 3. The refusal is visible, not silent: a caret that just stops responding
    // reads as a broken editor.
    await page.locator('.ProseMirror p').first().click()
    await page.keyboard.type('нельзя')
    await expect(page.getByText(/Блок редактирует/)).toBeVisible()
    await expect(page.locator('.ProseMirror')).not.toContainText('нельзя')

    // 4. Leaving frees the block immediately rather than at the TTL — the whole
    // point of releasing on disconnect instead of waiting 30 seconds.
    await matePage.close()
    await expect(page.locator('.ProseMirror .doc-block-locked')).toHaveCount(0)
    await expect(page.locator('.viewer')).toHaveCount(0)
  } finally {
    await context.close()
  }
})
