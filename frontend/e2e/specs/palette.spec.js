import { test, expect, openBoard } from '../fixtures.js'

// ⌘K palette (v2, #2957): find a task by its number from anywhere and open it;
// narrow the palette to a task with Tab and change a field without the modal.
const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

test('палитра находит задачу по номеру и открывает её', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('pal-open')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Палитра ${Date.now().toString(36)}`,
  })

  await openBoard(page, board.id)
  await page.keyboard.press(`${MOD}+k`)
  const input = page.getByTestId('palette-input')
  await expect(input).toBeFocused()
  await input.fill(`#${task.number}`)
  const row = page.locator(`[data-row="task:${task.id}"]`)
  await expect(row).toBeVisible()
  await input.press('Enter')
  await expect(page.getByTestId('command-palette')).toHaveCount(0)
  await expect(page).toHaveURL(new RegExp(`\\?task=${task.number}$`))
})

test('Tab в палитре меняет приоритет задачи', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('pal-prio')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Приоритет ${Date.now().toString(36)}`,
  })

  await openBoard(page, board.id)
  await page.keyboard.press(`${MOD}+k`)
  const input = page.getByTestId('palette-input')
  await input.fill(`#${task.number}`)
  await expect(page.locator(`[data-row="task:${task.id}"]`)).toBeVisible()
  await input.press('Tab')
  await expect(page.locator('[data-row="priority"]')).toBeVisible()
  await input.fill('приор')
  await input.press('Tab')
  await expect(page.locator('[data-row="prio:4"]')).toBeVisible()
  await page.locator('[data-row="prio:4"]').click()

  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).priority).toBe(4)
})
