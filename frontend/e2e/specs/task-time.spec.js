import { test, expect, openBoard } from '../fixtures.js'

// Учёт времени (#2965): «+ время» at the estimate row logs time, the row then
// reads «· потрачено …», and the entry can be removed in two presses.
test('учёт времени: записать и удалить из строки оценки', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('task-time')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Время ${Date.now().toString(36)}`,
  })
  await openBoard(page, board.id, ws.id)
  await page.goto(`/board/${board.id}?task=${task.number}`)
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })

  await modal.getByTestId('task-row-estimate').hover()
  await modal.getByTestId('task-spent').click()
  await page.getByTestId('time-log-amount').fill('1ч 30м')
  await page.getByTestId('time-log-note').fill('вёрстка')
  await page.getByTestId('time-log-amount').press('Enter')
  await expect(page.getByTestId('time-log-entry')).toHaveCount(1)
  await expect(modal.getByTestId('task-spent')).toContainText('1ч 30м')
  if (process.env.F2965_SHOT) await page.screenshot({ path: process.env.F2965_SHOT })
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).spent).toBe(90)

  const remove = page.getByTestId('time-log-entry').getByTestId('time-log-remove')
  await remove.click()
  await page
    .getByTestId('time-log-entry')
    .getByTestId(/time-log-remove/)
    .click()
  await expect(page.getByTestId('time-log-entry')).toHaveCount(0)
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).spent).toBe(0)
})
