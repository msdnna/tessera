import { test, expect, openBoard } from '../fixtures.js'

// The v2 start screen (#2960). Two claims worth a browser: the quick entry
// reads its line and creates the task with what it read, and my tasks land in
// the section their due date says.

test('быстрый ввод создаёт задачу с приоритетом и сроком из строки', async ({ page, backend }) => {
  const { ws, board } = await backend.freshBoard('home-quick')
  const stamp = Date.now().toString(36)

  // Opening the board makes it the quick entry's target (the last board open).
  await openBoard(page, board.id)
  await page.evaluate((id) => localStorage.setItem('tessera_ws', id), ws.id)
  await page.goto('/')
  await expect(page.getByTestId('home-greeting')).toBeVisible()

  const input = page.getByTestId('home-quick-input')
  await input.fill(`Созвон ${stamp} завтра !высокий`)
  // The preview names what will be created, and where.
  await expect(page.getByTestId('home-quick-preview')).toContainText(`Созвон ${stamp}`)
  await expect(page.getByTestId('home-quick-board')).toContainText(board.name)
  await input.press('Enter')
  await expect(input).toHaveValue('')

  await expect
    .poll(async () => {
      const tasks = await backend.get(`/boards/${board.id}/tasks`)
      const t = tasks.find((x) => x.title === `Созвон ${stamp}`)
      return t ? { priority: t.priority, due: !!t.due_date } : null
    })
    .toEqual({ priority: 3, due: true })
})

test('мои задачи раскладываются по срокам', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('home-sections')
  const me = await backend.get('/auth/me')
  const stamp = Date.now().toString(36)
  const today = new Date()
  const utc = (offset) =>
    new Date(
      Date.UTC(today.getFullYear(), today.getMonth(), today.getDate() + offset),
    ).toISOString()
  for (const [title, due] of [
    [`Сегодня ${stamp}`, utc(0)],
    [`Вчера ${stamp}`, utc(-1)],
  ]) {
    const t = await backend.post(`/boards/${board.id}/tasks`, {
      column_id: columns[0].id,
      title,
      due_date: due,
    })
    await backend.post(`/tasks/${t.id}/assignees`, { user_id: me.user.id })
  }

  await page.goto('/')
  await page.evaluate((id) => localStorage.setItem('tessera_ws', id), ws.id)
  await page.reload()
  await expect(page.getByTestId('home-greeting')).toBeVisible()
  await expect(page.locator('[data-section="today"]')).toContainText(`Сегодня ${stamp}`)
  await expect(page.locator('[data-section="overdue"]')).toContainText(`Вчера ${stamp}`)
  await expect(page.locator('[data-pill="overdue"]')).toBeVisible()
})
