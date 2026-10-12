import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// The due picker (#3118): words in the field, the presets, the grid by keyboard,
// the start → due range, the repeat and the due notification behind their doors
// — end to end against the real backend, so what the screen shows is what the
// server ended up with. And the repeat has to survive every other edit of the
// task: the update is a full replace, a path that forgets the rule wipes it.
//
// SHOTS=<dir> also saves the frames the visual check compares with the mockups.

const SHOTS = process.env.SHOTS || ''
async function shot(page, name) {
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/${name}.png` })
}

async function openTask(page, backend, label, extra = {}) {
  const { ws, project, board, columns } = await backend.freshBoard(label)
  const title = `Срок ${label} ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    ...extra,
  })
  await openBoard(page, board.id, ws.id)
  const card = cardsIn(page, columns[0].name).filter({ hasText: title })
  await card.click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  return { modal, task, project, board, columns, card, title }
}

// Wall calendar day of a stored instant, read the way the app reads it: a pure
// UTC midnight is a date (UTC), anything else is read in the browser's zone
// (the e2e user has no profile zone).
async function dayOf(page, iso) {
  return page.evaluate((v) => {
    if (!v) return null
    const d = new Date(v)
    const only = d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0
    const p = only
      ? [d.getUTCFullYear(), d.getUTCMonth() + 1, d.getUTCDate()]
      : [d.getFullYear(), d.getMonth() + 1, d.getDate()]
    const clock = only
      ? null
      : `${d.getHours()}`.padStart(2, '0') + ':' + `${d.getMinutes()}`.padStart(2, '0')
    return { day: p.join('-'), clock }
  }, iso)
}
async function todayPlus(page, n) {
  return page.evaluate((k) => {
    const d = new Date()
    d.setDate(d.getDate() + k)
    return [d.getFullYear(), d.getMonth() + 1, d.getDate()].join('-')
  }, n)
}
const stored = async (backend, id) => backend.get(`/tasks/${id}`)
// A poller for the stored day of `due_date` / `start_date` (writes are async).
const storedDay =
  (page, backend, id, f = 'due_date') =>
  async () =>
    (await dayOf(page, (await stored(backend, id))[f]))?.day ?? null

test('слова: «пт 18:00» с превью, «Не понял …» не пишет ничего', async ({ page, backend }) => {
  const { modal, task } = await openTask(page, backend, 'words')
  await page.keyboard.press('d')
  const picker = page.getByTestId('task-due-picker')
  await expect(picker).toBeVisible()
  const field = picker.getByTestId('due-input')
  await expect(field).toBeFocused()
  await shot(page, 'modal-empty')

  await field.fill('послезавтрашний')
  await expect(picker.getByTestId('due-bad')).toContainText('послезавтрашний')
  await field.press('Enter')
  await expect(picker).toBeVisible()
  expect((await stored(backend, task.id)).due_date).toBeFalsy()
  await shot(page, 'modal-bad')

  await field.fill('пт 18:00')
  await expect(picker.getByTestId('due-preview')).toContainText('18:00')
  await shot(page, 'modal-ok')
  await field.press('Enter')
  await expect(picker).toHaveCount(0)
  await expect(modal.getByTestId('task-due-open')).toContainText('18:00')
  const fri = await page.evaluate(() => new Date().getDay())
  const want = { day: await todayPlus(page, (5 - fri + 7) % 7), clock: '18:00' }
  await expect
    .poll(async () => dayOf(page, (await stored(backend, task.id)).due_date))
    .toEqual(want)
})

test('пресеты с клавиатуры, сетка стрелками, Tab — начало, ⌫ — очистить', async ({
  page,
  backend,
}) => {
  const { modal, task } = await openTask(page, backend, 'keys')
  await page.keyboard.press('d')
  const picker = page.getByTestId('task-due-picker')
  const field = picker.getByTestId('due-input')
  await expect(field).toBeFocused()

  // ↓ into the presets, 2 — tomorrow, a date without a time; the picker closes.
  await field.press('ArrowDown')
  await page.keyboard.press('2')
  await expect(picker).toHaveCount(0)
  await expect.poll(storedDay(page, backend, task.id)).toBe(await todayPlus(page, 1))
  expect((await dayOf(page, (await stored(backend, task.id)).due_date)).clock).toBe(null)

  // The grid: ↓ to the presets, → into the grid (on tomorrow), → once more, ↵.
  await page.keyboard.press('d')
  await expect(field).toBeFocused()
  await field.press('ArrowDown')
  await page.keyboard.press('ArrowRight')
  await page.keyboard.press('ArrowRight')
  await expect(picker.getByTestId('due-preview')).toBeVisible()
  await shot(page, 'modal-grid')
  await page.keyboard.press('Enter')
  await expect(picker).toHaveCount(0)
  await expect.poll(storedDay(page, backend, task.id)).toBe(await todayPlus(page, 2))

  // Tab — «Начало»: today as the start, the range shows between the two.
  await page.keyboard.press('d')
  await expect(field).toBeFocused()
  await field.press('Tab')
  await expect(picker.locator('[data-seg="start"]')).toHaveClass(/on/)
  await field.press('ArrowDown')
  await page.keyboard.press('1')
  await expect.poll(storedDay(page, backend, task.id, 'start_date')).toBe(await todayPlus(page, 0))
  expect(await storedDay(page, backend, task.id)()).toBe(await todayPlus(page, 2))

  // ⌫ on the empty field clears the endpoint being edited (the due).
  await page.keyboard.press('d')
  await expect(field).toBeFocused()
  await expect(picker.getByTestId('due-hint')).toContainText('3') // «· 3 дня»
  await shot(page, 'modal-range')
  await field.press('Backspace')
  await expect.poll(async () => (await stored(backend, task.id)).due_date).toBeFalsy()
  await field.press('Escape')
  await expect(picker).toHaveCount(0)
  await expect(modal).toBeVisible() // the first Esc is the picker's
})

test('повтор и напоминание сохраняются и переживают другие правки задачи', async ({
  page,
  backend,
}) => {
  const { modal, task, card } = await openTask(page, backend, 'rep')
  await page.keyboard.press('d')
  const picker = page.getByTestId('task-due-picker')
  await expect(picker.getByTestId('due-input')).toBeFocused()

  await picker.getByTestId('due-door-rep').click()
  const rep = picker.getByTestId('due-repeat')
  await rep.locator('[data-freq="weekly"]').click()
  await rep.locator('[data-wd="1"]').click()
  await rep.locator('[data-wd="4"]').click()
  await rep.getByTestId('due-rep-more').click()
  await rep.locator('[data-switch="dup"]').click()
  await expect(rep.getByTestId('due-rep-summary')).toContainText('2')
  await shot(page, 'modal-rep')
  await expect
    .poll(async () => (await stored(backend, task.id)).recurrence)
    .toMatchObject({ freq: 'weekly', interval: 2, weekdays: [1, 4], create_new: true })
  // A repeat without a due counts from today.
  expect((await stored(backend, task.id)).due_date).toBeTruthy()

  // Esc — back to the main panel, the door tells the rule in words.
  await page.keyboard.press('Escape')
  await expect(picker.getByTestId('due-door-rep')).toContainText('2')
  await picker.getByTestId('due-door-notify').click()
  const not = picker.getByTestId('due-notify')
  await not.locator('[data-mode="on"]').click()
  await not.locator('[data-lead="180"]').click()
  await not.locator('[data-repeat="60"]').click()
  await shot(page, 'modal-notify')
  await expect
    .poll(async () => {
      const t = await stored(backend, task.id)
      return [t.due_notify_enabled, t.due_lead_minutes, t.due_repeat_minutes]
    })
    .toEqual([true, 180, 60])
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)

  // Other edits of the task: the priority from the modal, the title, then the
  // card's own due pill — none of them may drop the rule.
  await page.keyboard.press('p')
  await page.keyboard.press('3')
  await expect.poll(async () => (await stored(backend, task.id)).priority).toBe(3)
  await page.keyboard.press('Escape')
  await expect(modal).toHaveCount(0)
  await card.getByTestId('card-due-pill').click()
  const cardPicker = page.getByTestId('card-due-picker')
  await expect(cardPicker).toBeVisible()
  await shot(page, 'card-due')
  await cardPicker.getByTestId('due-input').fill('через 3 дня')
  await cardPicker.getByTestId('due-input').press('Enter')
  await expect(cardPicker).toHaveCount(0)
  await expect.poll(storedDay(page, backend, task.id)).toBe(await todayPlus(page, 3))
  const t = await stored(backend, task.id)
  expect(t.recurrence).toMatchObject({ freq: 'weekly', interval: 2, weekdays: [1, 4] })
  expect([t.due_notify_enabled, t.due_lead_minutes, t.due_repeat_minutes]).toEqual([true, 180, 60])

  // The pill tells the rule on hover.
  await card.getByTestId('card-due-pill').hover()
  await expect(page.locator('.n-tooltip')).toContainText('2')

  // ⇧⌫ in the repeat panel drops the rule.
  await card.getByTestId('card-due-pill').click()
  await cardPicker.getByTestId('due-door-rep').click()
  await page.keyboard.press('Shift+Backspace')
  await expect.poll(async () => (await stored(backend, task.id)).recurrence).toBeFalsy()
})

test('палитра D: слова и пресеты, Tab — календарь под полем', async ({ page, backend }) => {
  const { modal, task } = await openTask(page, backend, 'pal', { priority: 1 })
  await page.keyboard.press('Escape')
  await expect(modal).toHaveCount(0)
  // ⌘K → the task → Tab → «Срок» (the card's D lands on the same list).
  await page.keyboard.press(`${process.platform === 'darwin' ? 'Meta' : 'Control'}+k`)
  const input = page.getByTestId('palette-input')
  await input.fill(`#${task.number}`)
  await expect(page.locator(`[data-row="task:${task.id}"]`)).toBeVisible()
  await input.press('Tab')
  await page.locator('[data-row="due"]').click()
  const pal = page.getByTestId('command-palette')
  await expect(pal.getByTestId('palette-field-chip')).toContainText(/Срок|Due/)

  await input.fill('завтра 9:00')
  await expect(pal.locator('[data-row="due:typed"]')).toBeVisible()
  await shot(page, 'palette-words')
  await input.fill('')
  await input.press('Tab')
  await expect(pal.getByTestId('palette-due-cal')).toBeVisible()
  await shot(page, 'palette-cal')
  await input.press('ArrowRight')
  await input.press('Enter')
  await expect.poll(storedDay(page, backend, task.id)).toBe(await todayPlus(page, 1))
  expect((await stored(backend, task.id)).priority).toBe(1)
})
