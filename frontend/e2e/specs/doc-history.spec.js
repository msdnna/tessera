import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'
import { t } from '../i18n.js'

// D6 (#2731). The unit tests drive the diff and the panel in isolation, and the
// Go tests drive coalescing, retention and the restore endpoint; none of them can
// show the part that only exists end to end — that a rollback performed in one
// browser replaces the body under a second one that has the document open, rather
// than leaving it typing on top of a tree the server no longer has.
//
// Two users for the same reason as the presence and comments specs: the content
// nudge travels over the document socket, and a second tab of one person would
// not prove it crossed sessions.

async function mateContext(browser, seed, baseURL) {
  const creds = newCredentials(seed.runId, '-hist')
  await register(creds)
  const context = await browser.newContext({ baseURL })
  const page = await context.newPage()
  await signIn(page, creds)
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  return { context, page, creds }
}

// One content save, awaited. Autosave is debounced, so typing and asserting
// immediately races the request the assertion depends on.
//
// The text is appended inside the existing paragraph rather than replacing the
// document: select-all-delete destroys the paragraph node, and the block that
// comes back has a new id — the journal would then honestly report "+1 −1"
// instead of the edited-in-place case this spec is about.
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

test('документ: журнал версий, сравнение и откат, который видит сосед', async ({
  page,
  browser,
  backend,
  seed,
}) => {
  const baseURL = new URL(page.url() || 'http://localhost:4174').origin
  const { context, page: matePage, creds } = await mateContext(browser, seed, baseURL)
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: creds.email })

  try {
    await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
    await page.goto('/documents')
    await page.getByTestId('doc-new').click()
    await expect(page.locator('.ProseMirror')).toBeVisible()

    await typeAndSave(page, 'Первая редакция регламента')
    await expect(page).toHaveURL(/\/documents\/[^/]+$/)
    const docURL = new URL(page.url()).pathname

    // 1. The journal is lazy: it is fetched when the panel is opened, not on
    // every document that gets read.
    const journal = page.waitForResponse(
      (r) => /\/documents\/[^/]+\/versions$/.test(r.url()) && r.request().method() === 'GET',
    )
    await page.getByTestId('doc-history-toggle').click()
    await journal
    const history = page.locator('.doc-history')
    await expect(history).toBeVisible()

    // 2. A named snapshot is the milestone the user asked to keep.
    await history.getByTestId('doc-snapshot').click()
    await history.getByTestId('doc-snapshot-label').locator('input').fill('Согласованная')
    await history.getByTestId('doc-snapshot-save').click()
    await expect(history.locator('.entry.milestone')).toHaveCount(1)
    await expect(history.getByText('Согласованная')).toBeVisible()

    // 3. Edit further. The milestone must not absorb the new text — that is the
    // whole point of pressing the button, and coalescing is what would break it.
    await typeAndSave(page, ' с поправками')
    await expect(history.locator('.entry.milestone .preview')).toContainText(
      'Первая редакция регламента',
    )
    await expect(history.locator('.entry.milestone .preview')).not.toContainText('с поправками')

    // 4. Comparison is block-level and says what changed, not "файл отличается".
    await history.locator('.entry.milestone').click()
    await expect(history.locator('.diff .counts')).toContainText(
      t('documents.history.changed', { count: 1 }),
    )
    await expect(history.locator('.diff .block.changed .was')).toContainText('Первая редакция')

    // 5. The mate opens the document and sees the current text, not the version
    // being previewed on the other side.
    await matePage.goto(docURL)
    await expect(matePage.locator('.ProseMirror')).toContainText('с поправками')

    // 6. Roll back. The state being replaced is snapshotted first, so the
    // rollback is itself undoable.
    await history.getByTestId('doc-restore').click()
    await page.getByTestId('doc-restore-confirm').click()
    await expect(page.locator('.ProseMirror')).not.toContainText('с поправками')
    await expect(page.locator('.ProseMirror')).toContainText('Первая редакция регламента')

    // 7. …and it reaches the mate without a reload. This is the socket nudge and
    // the reason this spec exists: without it the mate's next keystroke would
    // save the reverted text straight back.
    await expect(matePage.locator('.ProseMirror')).not.toContainText('с поправками')
    await expect(matePage.locator('.ProseMirror')).toContainText('Первая редакция регламента')

    // 8. The journal records the rollback as something that happened rather than
    // rewinding the counter: "откат к версии N" is itself an entry.
    await expect(history.getByText(/Откат к версии/)).toBeVisible()

    // …and the state the rollback pushed aside is still reachable, which is what
    // makes the rollback undoable. It needs no "перед откатом" entry of its own:
    // every save already journals, so the newest version was holding exactly that
    // text — adding a second copy would put a duplicate pair in every journal.
    await expect(history.locator('.entry .preview', { hasText: 'с поправками' })).toHaveCount(1)
  } finally {
    await context.close()
  }
})
