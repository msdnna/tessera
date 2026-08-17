import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// D7 (#2732). The Go tests drive the route rules (whose turn it is, what closes a
// route, who may cancel) and the unit tests drive the client's mirror of them.
// Neither can show the two things that only exist end to end:
//
//   - a signature landing in someone else's open panel without a reload — the
//     `links` nudge travelling over the document socket;
//   - the link showing up on the *task* side, in a modal on a different route
//     from the one that created it.
//
// Two users, for the same reason as the presence, comments and history specs: a
// sequential route needs a second approver to be a route at all, and one
// person's second tab would not prove the nudge crossed sessions.

async function mateContext(browser, seed, baseURL, suffix) {
  const creds = newCredentials(seed.runId, suffix)
  await register(creds)
  const context = await browser.newContext({ baseURL })
  const page = await context.newPage()
  await signIn(page, creds)
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  return { context, page, creds }
}

test('документ: связь с задачей и протокол согласования на двоих', async ({
  page,
  browser,
  backend,
  seed,
}) => {
  const baseURL = new URL(page.url() || 'http://localhost:4174').origin
  const { context, page: matePage, creds } = await mateContext(browser, seed, baseURL, '-links')
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: creds.email })

  const taskTitle = `Согласовать регламент ${seed.runId}`
  const task = await backend.createTask(seed.columns[0].id, taskTitle)

  try {
    await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
    await page.goto('/documents')
    await page.getByRole('button', { name: /Новый документ/ }).click()
    await expect(page.locator('.ProseMirror')).toBeVisible()
    await expect(page).toHaveURL(/\/documents\/[^/]+$/)
    const docURL = new URL(page.url()).pathname

    // ── linking a task ──
    await page.getByTestId('doc-links-toggle').click()
    const panel = page.getByTestId('doc-links')
    await expect(panel).toBeVisible()
    await expect(panel).toContainText('Связанных задач нет')

    await panel.getByTestId('doc-link-add').click()
    await page.getByPlaceholder('№ или название').fill(taskTitle)
    await page.getByText(taskTitle, { exact: false }).last().click()
    await expect(panel.getByTestId('doc-link')).toHaveCount(1)
    await expect(panel).toContainText(taskTitle)

    // The other end of the same row. This is the assertion that the task modal
    // reads the relation the documents panel wrote — the two sides are separate
    // endpoints over one table, and only this crosses both.
    await page.goto(`/board/${seed.boardId}?task=${task.id}`)
    const modal = page.locator('.n-modal')
    // Naive's tabs are divs, not role="tab" — locate the label itself, scoped to
    // the modal so the sidebar's «Документы» nav link cannot match instead.
    await modal.locator('.n-tabs-tab', { hasText: 'Документы' }).click()
    await expect(modal.getByText('Без названия').first()).toBeVisible()

    // ── the approval route ──
    await page.goto(docURL)
    await page.getByTestId('doc-links-toggle').click()
    await panel.getByTestId('doc-approval-raise').click()
    await page.getByPlaceholder('Что согласуем').fill('Редакция для правления')
    await page.getByTestId('doc-approval-approvers').click()
    // Exact: the document's author is named "E2E <runId>" and the approver
    // "E2E <runId>-links", so a substring match would pick the author — and a
    // route whose only approver is the person who raised it proves nothing.
    await page.locator('.n-base-select-option', { hasText: creds.name }).first().click()
    await page.keyboard.press('Escape')
    // Scoped to the panel: the comments composer carries its own «Отправить»,
    // and an unscoped match resolves to both.
    await panel.getByRole('button', { name: 'Отправить', exact: true }).click()

    const protocol = panel.getByTestId('doc-approval')
    await expect(protocol).toHaveCount(1)
    await expect(protocol).toContainText('На согласовании')
    await expect(protocol).toContainText('0 из 1')
    // Raising a route pins the text being agreed; a protocol that cannot name
    // its revision is a signature on a moving target.
    await expect(protocol).toContainText(/Версия \d+/)

    // The author is not on the route, so no signature is offered to them.
    await expect(panel.getByTestId('doc-approval-sign')).toHaveCount(0)

    // ── the approver signs, and the author's panel updates itself ──
    await matePage.goto(docURL)
    await matePage.getByTestId('doc-links-toggle').click()
    const matePanel = matePage.getByTestId('doc-links')
    await matePanel.getByTestId('doc-approval-sign').click()
    await matePage.getByPlaceholder('Комментарий (необязательно)').fill('Замечаний нет')
    await matePage.getByRole('button', { name: 'Согласовать', exact: true }).click()
    await expect(matePanel.getByTestId('doc-approval')).toContainText('Согласовано')

    // No reload here — this is the whole point of the spec. The author's panel
    // has to move because the socket nudged it, not because the page was
    // fetched again.
    await expect(protocol).toContainText('Согласовано', { timeout: 10000 })
    await expect(protocol).toContainText('1 из 1')
    await expect(protocol).toContainText('Замечаний нет')
  } finally {
    await context.close()
  }
})
