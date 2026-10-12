import { test, expect, openBoard, column } from '../fixtures.js'
import { api, newCredentials, register, login } from '../api.js'

// The toasts of v2 (#3117, PackageI): two lanes — the answers to my own actions
// (and the undo toasts) at the bottom, what came from outside under the bell —
// fed by the same socket record as the bell's feed; a burst of board activity
// folds into one toast; «Не беспокоить» silences the top lane; the settings card
// is saved with the rest of the notification prefs.

// A second person in the workspace, acting over the API.
async function mate(backend, seed, wsId, suffix) {
  const creds = newCredentials(seed.runId, suffix)
  await register(creds)
  await backend.post(`/workspaces/${wsId}/members`, { email: creds.email })
  const { token, user } = await login(creds.email, creds.password)
  return { creds, token, user }
}
async function myId(backend) {
  const me = await backend.get('/auth/me')
  return me.user?.id || me.id
}
// The bell's count; no badge at all reads as 0.
async function unread(page) {
  const n = page.getByTestId('notifications-unread')
  return (await n.count()) ? Number(await n.textContent()) || 0 : 0
}
const top = (page) => page.getByTestId('toast-lane-top')
const bottom = (page) => page.getByTestId('toast-lane-bottom')

test('тосты: «про меня» сверху от действия коллеги, undo снизу, F6 и Esc', async ({
  page,
  backend,
  seed,
}) => {
  const me = { id: await myId(backend) }
  const { ws, board, columns } = await backend.freshBoard('toasts-lanes')
  const m = await mate(backend, seed, ws.id, '-toast-a')
  const task = await api.post(
    `/boards/${board.id}/tasks`,
    { column_id: columns[0].id, title: 'Тост про меня' },
    seed.token,
  )
  await openBoard(page, board.id)

  // Somebody else's action about me → the top lane, under the bell.
  await api.post(`/tasks/${task.id}/assignees`, { user_id: me.id }, m.token)
  const mine = top(page).getByTestId('toast')
  await expect(mine).toHaveCount(1)
  await expect(mine).toHaveAttribute('data-kind', 'assign')
  await expect(mine).toContainText(m.creds.name)
  await expect(mine).toContainText('Тост про меня')

  const vp = page.viewportSize()
  const t = await top(page).boundingBox()
  expect(t.y).toBeLessThan(120)
  expect(t.x + t.width).toBeGreaterThan(vp.width - 40)

  // My own action → the bottom lane: deleting a column offers its undo there
  // (a column out from under the top lane's corner).
  const name = columns[1].name
  await column(page, name).locator('.col-menu').click()
  await page.getByTestId('column-actions-delete').click()
  await expect(bottom(page).getByTestId('undo-toast')).toBeVisible()
  await expect(mine).toBeVisible()
  const b = await bottom(page).getByTestId('undo-toast').boundingBox()
  expect(b.y + b.height).toBeGreaterThan(vp.height - 120)
  await bottom(page).getByTestId('undo-toast-undo').click()
  await expect(column(page, name)).toBeVisible()

  // F6 puts the focus into a lane; Esc closes the focused toast.
  await page.locator('body').click({ position: { x: 5, y: 400 } })
  await page.keyboard.press('F6')
  await expect
    .poll(() =>
      page.evaluate(() => !!document.activeElement?.closest?.('[data-testid="toast-lane-top"]')),
    )
    .toBe(true)
  await page.keyboard.press('Escape')
  await expect(top(page).getByTestId('toast')).toHaveCount(0)
})

test('тосты: пачка чужих переносов складывается в один тост с числом', async ({
  page,
  backend,
  seed,
}) => {
  const { ws, board, columns } = await backend.freshBoard('toasts-batch')
  const m = await mate(backend, seed, ws.id, '-toast-b')
  // The mate's own tasks: their moves are not «про меня» for the reader.
  const tasks = []
  for (const title of ['Пачка 1', 'Пачка 2', 'Пачка 3'])
    tasks.push(
      await api.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title }, m.token),
    )
  await openBoard(page, board.id)

  for (const task of tasks)
    await api.patch(`/tasks/${task.id}/move`, { column_id: columns[1].id }, m.token)

  const act = top(page).locator('[data-kind="activity"]')
  await expect(act).toHaveCount(1)
  await expect(act.getByTestId('toast-count')).toHaveText('3')
  await expect(act).toContainText(m.creds.name.split(' ')[0])
})

test('тосты: «Не беспокоить» глушит верхнюю полосу, счётчик растёт', async ({
  page,
  backend,
  seed,
}) => {
  const me = { id: await myId(backend) }
  const { ws, board, columns } = await backend.freshBoard('toasts-dnd')
  const m = await mate(backend, seed, ws.id, '-toast-c')
  const task = await api.post(
    `/boards/${board.id}/tasks`,
    { column_id: columns[0].id, title: 'Тихо' },
    seed.token,
  )
  await openBoard(page, board.id)
  try {
    await page.getByTestId('notifications-bell').click()
    await page.getByTestId('notifications-dnd').click()
    await page.getByTestId('dnd-menu-hour').click()
    await expect(page.getByTestId('notifications-dnd-banner')).toBeVisible()
    await expect(page.getByTestId('notifications-dnd-moon')).toBeVisible()
    await page.keyboard.press('Escape')

    const before = await unread(page)
    await api.post(`/tasks/${task.id}/assignees`, { user_id: me.id }, m.token)
    await expect.poll(() => unread(page)).toBeGreaterThan(before)
    await page.waitForTimeout(800)
    await expect(top(page).getByTestId('toast')).toHaveCount(0)

    const prefs = await backend.get('/notification-prefs')
    expect(prefs.dnd_until).toBeTruthy()
  } finally {
    await api.put('/notification-prefs/dnd', { until: null }, seed.token)
  }
})

test('тосты: карточка «Всплывающие в приложении» сохраняется', async ({ page, backend, seed }) => {
  await page.goto('/settings?s=notifications')
  const card = page.getByTestId('notif-toasts')
  await expect(card).toBeVisible()
  try {
    await card.getByTestId('notif-toasts-me-comments').click()
    await card.getByTestId('notif-toasts-batch-more').click()
    await expect(card.getByTestId('notif-toasts-batch-min')).toHaveText('4')
    await expect
      .poll(async () => (await backend.get('/notification-prefs')).toasts?.batchMin)
      .toBe(4)
    const saved = await backend.get('/notification-prefs')
    expect(saved.toasts.me.comments).toBe(false)

    await page.reload()
    await expect(card.getByTestId('notif-toasts-batch-min')).toHaveText('4')
    await expect(card.getByTestId('notif-toasts-me-comments')).not.toHaveClass(/checked/)
  } finally {
    const p = await backend.get('/notification-prefs')
    await api.put('/notification-prefs', { ...p, toasts: {} }, seed.token)
  }
})
