import { test, expect, openBoard, column, cardsIn } from '../fixtures.js'

// Keyboard on the board (v2, #2956): arrows select a card, E completes it
// through the done column; ? opens the cheat sheet; N, T starts the «К работе»
// quick-add.
test('стрелки выбирают карточку, E переносит её в «Готово»', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('kb-done')
  const title = `Клавиатура ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })

  await openBoard(page, board.id)
  await expect(cardsIn(page, columns[0].name)).toHaveCount(1)
  await page.keyboard.press('ArrowDown')
  await expect(page.locator(`[data-task-id="${task.id}"].selected`)).toBeVisible()
  await page.keyboard.press('e')

  const done = columns[columns.length - 1]
  await expect(cardsIn(page, done.name).filter({ hasText: title })).toBeVisible()
  await expect.poll(async () => !!(await backend.get(`/tasks/${task.id}`)).completed_at).toBe(true)
})

test('? открывает шпаргалку, N затем T — быструю задачу в «К работе»', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('kb-quick')
  await openBoard(page, board.id)

  await page.keyboard.press('Shift+Slash')
  await expect(page.getByTestId('shortcuts-sheet')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('shortcuts-sheet')).toHaveCount(0)

  await page.keyboard.press('n')
  await page.keyboard.press('t')
  const input = column(page, columns[0].name).getByTestId('add-task-input').locator('textarea')
  await expect(input).toBeFocused()
  const title = `Быстрая ${Date.now().toString(36)}`
  await input.fill(title)
  await input.press('Enter')
  await expect(cardsIn(page, columns[0].name).filter({ hasText: title })).toBeVisible()
})
