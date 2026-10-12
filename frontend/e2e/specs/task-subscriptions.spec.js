import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// «Подписаны» at the foot of the task window's panel (#2963): the author is in
// on their own, «Отписаться» takes them out and the server agrees, a comment
// brings a commenter in — against a real backend.
test('подписки: автор подписан сам, «Отписаться» / «Подписаться» доходят до сервера', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('subs')
  const title = `Подписки ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
  })
  await openBoard(page, board.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()

  const block = modal.getByTestId('task-subscribers')
  const toggle = block.getByTestId('task-subscribe-toggle')
  await expect(block.getByTestId('task-subscriber')).toHaveCount(1)
  await expect(toggle).toHaveText(/Отписаться|Unfollow/)
  // SHOTS=<dir> saves the frame the visual check compares with TaskModal.dc.
  if (process.env.SHOTS) {
    await page.waitForTimeout(400) // the window's fade-in
    await page.screenshot({ path: `${process.env.SHOTS}/subscribers.png` })
  }

  await toggle.click()
  await expect(block.getByTestId('task-subscriber')).toHaveCount(0)
  await expect(toggle).toHaveText(/Подписаться|Follow/)
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).subscribed).toBe(false)

  await toggle.click()
  await expect(block.getByTestId('task-subscriber')).toHaveCount(1)
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).subscribed).toBe(true)
})
