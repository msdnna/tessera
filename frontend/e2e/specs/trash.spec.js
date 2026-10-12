import { test, expect, openBoard, cardsIn } from '../fixtures.js'

// The trash and the project archive (#3001: #3004, #3005, #3006, #3120). A
// delete is immediate and reversible — the undo is in the toast, the trash keeps
// the rest for 30 days; an archived project leaves the tree and opens on view
// only. Each test runs on a board of its own: they delete and archive.

test('задача: удалить → тост → отменить; корзина доски — вернуть и удалить навсегда', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('trash-task')
  const todo = columns[0]
  const title = `В корзину ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, { column_id: todo.id, title })
  await openBoard(page, board.id)
  const card = () => cardsIn(page, todo.name).filter({ hasText: title })
  await expect(card()).toBeVisible()

  // «Удалить» in the card's menu: one press, the card is gone, the toast offers
  // the undo — and the undo brings it back.
  const del = async () => {
    await card().click({ button: 'right' })
    await page.getByTestId('task-menu-delete').click()
    await expect(card()).toHaveCount(0)
  }
  await del()
  const toast = page.getByTestId('undo-toast').filter({ hasText: `#${task.number}` })
  await expect(toast).toBeVisible()
  await toast.getByTestId('undo-toast-undo').click()
  await expect(card()).toBeVisible()

  // Again, and this time it is brought back from the board's «Корзина».
  await del()
  await page.getByTestId('board-more').click()
  await page.getByTestId('board-more-menu-trash').click()
  await expect(page).toHaveURL(/[?&]trash=1/)
  const panel = page.getByTestId('board-trash')
  const row = panel.getByTestId('trash-row').filter({ hasText: title })
  await expect(row).toBeVisible()
  await row.getByTestId('trash-restore').click()
  await expect(row).toHaveCount(0)
  await expect(card()).toBeVisible()

  // And once more, out of the trash for good: «Удалить навсегда» is two-step.
  await del()
  await expect(row).toBeVisible()
  await row.getByTestId('trash-purge').click()
  await row.getByTestId('trash-purge-armed').click()
  await expect(row).toHaveCount(0)
  await expect(panel.getByText(/Корзина пуста|The trash is empty/)).toBeVisible()
  const gone = await backend.get(`/tasks/${task.id}`).catch((e) => e)
  expect(String(gone)).toMatch(/→ 404/)
})

test('тост «про меня» об удалённой задаче ведёт в корзину доски (#3120)', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('trash-toast')
  const title = `Подсветить ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })
  await backend.del(`/tasks/${task.id}`)
  // The deep link the toast's «В корзину» opens: the panel with the row lit.
  await page.goto(`/board/${board.id}`)
  await page.waitForURL(/\/project\/[^/]+\/board\//)
  const url = new URL(page.url())
  await page.goto(`${url.pathname}?trash=1&hl=${task.number}`)
  const row = page.getByTestId('board-trash').getByTestId('trash-row').filter({ hasText: title })
  await expect(row).toBeVisible()
  await expect(row).toHaveClass(/lit/)
})

test('проект: в архив → пропал из дерева, доска только для просмотра → восстановить', async ({
  page,
  backend,
}) => {
  const label = `arch-${Date.now().toString(36)}`
  const { board, project, columns } = await backend.freshBoard(label)
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: 'Архивная' })
  await openBoard(page, board.id)
  const tree = page.locator('.project-row').filter({ hasText: project.name })
  await expect(tree).toBeVisible()

  // «Опасная зона» of the «Проект» window: «Архивировать» at once, with a toast.
  await page.getByTestId('board-more').click()
  await page.getByTestId('board-more-menu-project').click()
  await page.getByTestId('project-archive').click()
  await expect(page.getByTestId('undo-toast').filter({ hasText: project.name })).toBeVisible()
  await page.keyboard.press('Escape')

  await expect(tree).toHaveCount(0)
  await expect(page.getByTestId('project-archived-chip')).toBeVisible()
  // On view only: no «+ колонка», no «Создать задачу» in the lanes.
  await expect(page.getByTestId('add-column')).toHaveCount(0)

  // «Архив проектов» at the foot of the tree lists it; «Восстановить» puts it back.
  await page.getByTestId('archived-projects').click()
  const menu = page.getByTestId('archived-projects-menu')
  await menu.getByText(project.name).click()
  await menu.getByText(/^(Восстановить|Restore)$/).click()
  await expect(tree).toBeVisible()
  await expect(page.getByTestId('project-archived-chip')).toHaveCount(0)
})

test('документы: удалить → корзина в дереве → вернуть и удалить навсегда', async ({
  page,
  backend,
}) => {
  const { board, ws } = await backend.freshBoard('trash-docs')
  const title = `Регламент ${Date.now().toString(36)}`
  const doc = await backend.post(`/workspaces/${ws.id}/documents`, { title })
  // The board puts the app into its workspace; the documents section follows.
  await openBoard(page, board.id)
  await page.goto(`/documents/${doc.slug}`)
  await expect(page.getByTestId('doc-head')).toBeVisible()

  await page.getByTestId('doc-actions').click()
  await page.getByTestId('doc-action-remove').click()
  await expect(page).toHaveURL(/\/documents$/)
  const toast = page.getByTestId('undo-toast').filter({ hasText: title })
  await expect(toast).toBeVisible()

  await page.getByTestId('doc-tree-trash').click()
  const row = page.getByTestId('doc-trash-row').filter({ hasText: title })
  await expect(row).toBeVisible()
  await row.getByTestId('doc-trash-restore').click()
  await expect(row).toHaveCount(0)
  await page.getByTestId('doc-trash-back').click()
  await expect(page.locator('.tile').filter({ hasText: title })).toBeVisible()

  // For good: delete, then purge from the trash.
  await backend.del(`/documents/${doc.id}`)
  await page.getByTestId('doc-tree-trash').click()
  await expect(row).toBeVisible()
  await row.getByTestId('doc-trash-purge').click()
  await row.getByTestId('doc-trash-purge-armed').click()
  await expect(row).toHaveCount(0)
})
