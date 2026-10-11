import { test, expect, openBoard } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// «Входящие» (#2972): one capture line makes a note or a reminder, the panel
// shows them by group and answers to its keys, the task modal and the card
// show the task's own, and the old section addresses land in the panel.

const stamp = () => Date.now().toString(36)

async function feed(backend, wsId) {
  return backend.get(`/workspaces/${wsId}/inbox`)
}

test('захват: заметка и напоминание из одной строки, лента и её клавиши', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('inbox-capture')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: 'Задача для входящих',
  })
  const s = stamp()
  await openBoard(page, board.id)

  // A note tied to the task by its number.
  await page.keyboard.press('n')
  await page.keyboard.press('m')
  const input = page.getByTestId('capture-input')
  await expect(input).toBeFocused()
  await input.fill(`идея ${s} /${task.number}`)
  await expect(page.getByTestId('capture-task')).toContainText('Задача для входящих')
  await input.press('Enter')
  await expect(input).toHaveCount(0)

  // A moment in the line makes it a reminder.
  await page.keyboard.press('n')
  await page.keyboard.press('m')
  await input.fill(`завтра 10:00 созвон ${s}`)
  await expect(page.getByTestId('capture-when')).toBeVisible()
  await input.press('Enter')
  await expect(input).toHaveCount(0)

  await expect
    .poll(async () => (await feed(backend, ws.id)).reminders.map((r) => r.message))
    .toContain(`Созвон ${s}`)
  const note = (await feed(backend, ws.id)).notes.find((n) => n.title === `Идея ${s}`)
  expect(note.visibility).toBe('private')
  expect(note.task_id).toBe(task.id)

  // G I: the panel, focus in the list, the first record selected.
  await page.keyboard.press('g')
  await page.keyboard.press('i')
  const panel = page.getByTestId('inbox-panel')
  await expect(panel.locator('[data-group="later"]')).toContainText(`Созвон ${s}`)
  await expect(panel.locator('[data-group="notes"]')).toContainText(`Идея ${s}`)
  await expect(panel.getByTestId('inbox-item-keys')).toBeVisible()

  // E completes the reminder (it stays, struck through, until midnight).
  await page.keyboard.press('e')
  await expect
    .poll(
      async () =>
        (await feed(backend, ws.id)).reminders.find((r) => r.message === `Созвон ${s}`)?.done,
    )
    .toBe(true)
  await expect(panel.locator('[data-state="done"]')).toContainText(`Созвон ${s}`)

  // ⌫ deletes at once; the toast brings it back.
  await page.keyboard.press('Backspace')
  await expect.poll(async () => (await feed(backend, ws.id)).reminders.length).toBe(0)
  // The delete's toast lands once the browser has the DELETE's answer, which
  // can trail the poll above; the «done» one stands above it meanwhile. Aim at
  // the delete's own toast, not at whichever is last right now.
  await page
    .getByTestId('undo-toast')
    .filter({ hasText: 'Запись удалена' })
    .getByTestId('undo-toast-undo')
    .click()
  await expect
    .poll(async () => (await feed(backend, ws.id)).reminders.map((r) => r.message))
    .toContain(`Созвон ${s}`)

  // G I again from inside the panel hides it.
  await page.getByTestId('inbox-list').focus()
  await page.keyboard.press('g')
  await page.keyboard.press('i')
  await expect(panel).toHaveCount(0)
})

test('теги и упоминания: «Созвон #релиз @Анна завтра 10:00» — пилл тега и упоминание в ленте', async ({
  page,
  backend,
  seed,
}) => {
  const { ws, board } = await backend.freshBoard('inbox-tags')
  const s = stamp()
  // A second member named Анна — the handle in the line names her.
  const creds = { ...newCredentials(seed.runId, `-anna-${s}`), name: `Анна Ким ${s}` }
  const anna = await register(creds)
  await backend.post(`/workspaces/${ws.id}/members`, { email: creds.email, role: 'member' })
  await openBoard(page, board.id)

  await page.keyboard.press('n')
  await page.keyboard.press('m')
  const input = page.getByTestId('capture-input')
  await expect(input).toBeFocused()
  await input.fill('Созвон #релиз @Анна завтра 10:00')
  await expect(page.getByTestId('capture-when')).toBeVisible()
  await expect(page.getByTestId('capture-tag')).toHaveText('релиз')
  await expect(page.getByTestId('capture-mention')).toContainText(`@Анна Ким ${s}`)
  await expect(page.getByTestId('capture-title')).toHaveText('Созвон')
  await input.press('Enter')
  await expect(input).toHaveCount(0)

  await expect
    .poll(async () => (await feed(backend, ws.id)).reminders.find((r) => r.message === 'Созвон'))
    .toMatchObject({ tags: ['релиз'], mentions: [anna.user.id] })

  await page.keyboard.press('g')
  await page.keyboard.press('i')
  const panel = page.getByTestId('inbox-panel')
  const item = panel.getByTestId('inbox-item').filter({ hasText: 'Созвон' })
  await expect(item.getByTestId('inbox-item-tag')).toHaveText('релиз')
  await expect(item.getByTestId('inbox-item-mention')).toHaveText('@Анна')

  // A note without the tag, then the tag pill narrows the feed to the tagged one.
  await backend.post(`/workspaces/${ws.id}/notes`, {
    title: `Без тега ${s}`,
    visibility: 'private',
  })
  await expect(panel.getByTestId('inbox-item')).toHaveCount(2)
  await panel
    .getByTestId('inbox-item')
    .filter({ hasText: 'Созвон' })
    .getByTestId('inbox-item-tag')
    .click()
  await expect(panel.getByTestId('inbox-tag-filter')).toContainText('#релиз')
  await expect(panel.getByTestId('inbox-item')).toHaveCount(1)
  await panel.getByTestId('inbox-tag-filter').getByRole('button').click()
  await expect(panel.getByTestId('inbox-item')).toHaveCount(2)
})

test('модалка задачи: ⇧R и цифра ставят напоминание, на карточке — колокольчик', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('inbox-task')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Напомнить про ${stamp()}`,
  })
  await page.goto(`/board/${board.id}?task=${task.number}`)
  await expect(page.getByTestId('task-inbox')).toBeVisible()

  await page.keyboard.press('Shift+R')
  await expect(page.getByTestId('task-inbox-opts')).toBeVisible()
  await page.keyboard.press('1')
  await expect(page.getByTestId('task-inbox-row')).toHaveCount(1)
  const mine = await backend.get(`/tasks/${task.id}/reminders`)
  expect(mine).toHaveLength(1)
  expect(mine[0].workspace_id).toBe(ws.id)

  await page.keyboard.press('Escape')
  await expect(
    page.locator(`[data-task-id="${task.id}"]`).getByTestId('card-reminder'),
  ).toBeVisible()
})

test('старые адреса /notes и /reminders открывают панель', async ({ page }) => {
  await page.goto('/notes')
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByTestId('inbox-panel')).toBeVisible()
  await expect(page.locator('[data-filter="notes"]')).toHaveAttribute('aria-selected', 'true')

  await page.goto('/reminders')
  await expect(page.locator('[data-filter="reminders"]')).toHaveAttribute('aria-selected', 'true')
})

// Offline capture (#3042): without a connection the capture waits on the device
// («ждёт связи» in the panel) and goes out by itself once the connection is back.
test('захват без связи: запись ждёт на устройстве и уходит при появлении связи', async ({
  page,
  backend,
}) => {
  const { ws, board } = await backend.freshBoard('inbox-offline')
  const s = stamp()
  await openBoard(page, board.id)

  await page.context().setOffline(true)
  await page.keyboard.press('n')
  await page.keyboard.press('m')
  const input = page.getByTestId('capture-input')
  await input.fill(`без связи ${s}`)
  await input.press('Enter')
  await expect(input).toHaveCount(0)
  await page.keyboard.press('g')
  await page.keyboard.press('i')
  await expect(page.getByTestId('inbox-pending')).toBeVisible()
  expect((await feed(backend, ws.id)).notes.map((n) => n.title)).not.toContain(`Без связи ${s}`)

  await page.context().setOffline(false)
  await expect(page.getByTestId('inbox-pending')).toHaveCount(0)
  await expect
    .poll(async () =>
      (await feed(backend, ws.id)).notes.filter((n) => n.title === `Без связи ${s}`),
    )
    .toHaveLength(1)
  await expect(page.getByTestId('inbox-panel').locator('[data-group="notes"]')).toContainText(
    `Без связи ${s}`,
  )
})
