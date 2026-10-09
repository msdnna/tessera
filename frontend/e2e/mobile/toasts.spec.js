import { test, expect, openBoard } from '../fixtures.js'
import { api, newCredentials, register, login } from '../api.js'

// The phone's toasts (#3117, PackageI «Мобильный»): one lane over the bottom
// navigation, one toast at a time («1 из N» while others wait), a swipe aside
// closes it, the FAB rides up over it.

async function mate(backend, seed, wsId, suffix) {
  const creds = newCredentials(seed.runId, suffix)
  await register(creds)
  await backend.post(`/workspaces/${wsId}/members`, { email: creds.email })
  return login(creds.email, creds.password)
}

test('телефон: тост над навигацией, очередь, свайп закрывает, FAB поднимается', async ({
  page,
  backend,
  seed,
}) => {
  const me = await backend.get('/auth/me')
  const myId = me.user?.id || me.id
  const { ws, board, columns } = await backend.freshBoard('toasts-phone')
  const m = await mate(backend, seed, ws.id, '-toast-m')
  const tasks = []
  for (const title of ['Телефон 1', 'Телефон 2'])
    tasks.push(
      await api.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title }, seed.token),
    )
  await openBoard(page, board.id)
  const fab = page.getByTestId('quick-fab-button')
  const fabBefore = await fab.boundingBox()

  for (const task of tasks)
    await api.post(`/tasks/${task.id}/assignees`, { user_id: myId }, m.token)

  const lane = page.getByTestId('toast-lane-phone')
  await expect(lane.getByTestId('toast')).toHaveCount(1)
  await expect(page.getByTestId('toast-queue')).toBeVisible()

  // Over the navigation, the screen's width less 12 + 12.
  const vp = page.viewportSize()
  const box = await lane.getByTestId('toast').boundingBox()
  const nav = await page.getByTestId('mobile-tabbar').boundingBox()
  expect(box.y + box.height).toBeLessThanOrEqual(nav.y)
  expect(Math.round(box.width)).toBe(vp.width - 24)
  // No × on a phone.
  await expect(lane.getByTestId('toast-close')).toHaveCount(0)
  // The FAB stands above the toast.
  await expect.poll(async () => (await fab.boundingBox()).y).toBeLessThan(fabBefore.y)

  // A swipe aside past 96px closes it; the next one comes out of the queue.
  const item = lane.locator('.tl-touch')
  const b = await item.boundingBox()
  await page.mouse.move(b.x + 40, b.y + b.height / 2)
  await page.mouse.down()
  await page.mouse.move(b.x + 120, b.y + b.height / 2, { steps: 4 })
  await page.mouse.move(b.x + 200, b.y + b.height / 2, { steps: 4 })
  await page.mouse.up()
  await expect(page.getByTestId('toast-queue')).toHaveCount(0)
  await expect(lane.getByTestId('toast')).toHaveCount(1)
})
