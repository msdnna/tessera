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

// #2716 — the side-panel layout. The point of the panel (as opposed to the modal)
// is that the board keeps working next to it, so the load-bearing assertion is not
// "the card moved right" but "clicking a board card behind it still works and the
// panel doesn't slam shut". That's what a mask would break.
test('задача открывается панелью справа, доска под ней остаётся живой', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('modal-layout')
  const stamp = Date.now().toString(36)
  const first = `Первая ${stamp}`
  const second = `Вторая ${stamp}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: first })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: second })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: first }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('task-layout-trigger').click()
  await page.getByTestId('task-layout-sidebar').click()

  // Pinned to the right edge of the viewport. Measured with a retrying poll, not a
  // bare boundingBox(): switching the mask off re-runs the modal's enter transition,
  // and a single read lands mid-scale on a card that hasn't settled yet.
  await expect(modal).toHaveClass(/tm-sidebar/)
  const width = page.viewportSize().width
  await expect
    .poll(async () => {
      const box = await modal.boundingBox()
      return Math.abs(box.x + box.width - width)
    })
    .toBeLessThan(2)

  // The board underneath is still interactive: this click opens the other task
  // rather than being eaten by a mask or dismissing the panel.
  await cardsIn(page, columns[0].name).filter({ hasText: second }).click()
  await expect(modal).toBeVisible()
  await expect(modal.locator('.title-input input')).toHaveValue(second)
  await expect(modal).toHaveClass(/tm-sidebar/)

  // The choice is device-level and survives a reload.
  await page.reload()
  await expect(page.getByTestId('column').first()).toBeVisible()
  await cardsIn(page, columns[0].name).filter({ hasText: first }).click()
  await expect(page.getByTestId('task-modal')).toHaveClass(/tm-sidebar/)
})
