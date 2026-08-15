import { test, expect, cardsIn, openBoard } from '../fixtures.js'

test('модалка задачи открывается, показывает описание и принимает комментарий', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('modal')
  const title = `Задача с описанием ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    description: '## Заголовок описания\n\nОбычный абзац.',
  })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  // The title is an editable field, so it holds the text as a value — `getByText`
  // would never see it.
  await expect(modal.locator('.title-input input')).toHaveValue(title)
  // The description is stored as Markdown and rendered — the heading must come
  // back as a real <h2>, not as literal "##".
  await expect(modal.locator('h2', { hasText: 'Заголовок описания' })).toBeVisible()

  const body = `Комментарий из e2e ${Date.now().toString(36)}`
  const composer = modal.locator('.comment-add textarea')
  await composer.fill(body)
  await composer.press('Control+Enter')

  await expect(modal.locator('.c-list')).toContainText(body)

  // ...and it really reached the server, not just the local list.
  await expect
    .poll(async () => {
      const comments = await backend.get(`/tasks/${task.id}/comments`)
      return comments.map((c) => c.body)
    })
    .toContain(body)
})

test('ответ в треде остаётся вложенным после перезагрузки', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('threads')
  const title = `Задача с тредом ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
  })
  const root = await backend.post(`/tasks/${task.id}/comments`, { body: 'Корневой комментарий' })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()

  const thread = modal.locator('.c-thread')
  await thread.getByRole('button', { name: 'Ответить' }).click()
  const replyBody = `Ответ из e2e ${Date.now().toString(36)}`
  const composer = modal.locator('.c-reply-add textarea')
  await composer.fill(replyBody)
  await composer.press('Control+Enter')

  // Nested, not appended to the flat list: the reply has to land inside the
  // root's .c-replies branch, which is the whole point of the feature.
  await expect(thread.locator('.c-replies')).toContainText(replyBody)

  // And the nesting is stored, not just rendered — the server has to give the
  // reply back with its parent, or the tree dies on the next page load.
  await expect
    .poll(async () => {
      const comments = await backend.get(`/tasks/${task.id}/comments`)
      return comments.find((c) => c.body === replyBody)?.parent_id
    })
    .toBe(root.id)

  // No second click after the reload: the open task lives in `?task=`, so the
  // modal reopens by itself and would swallow the click on the card behind it.
  await page.reload()
  const reloaded = page.getByTestId('task-modal')
  await expect(reloaded).toBeVisible()
  await expect(reloaded.locator('.c-thread .c-replies')).toContainText(replyBody)
})

test('модалка закрывается по Escape', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('modal-esc')
  const title = `Закрой меня ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  await expect(page.getByTestId('task-modal')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(page.getByTestId('task-modal')).toHaveCount(0)
})
