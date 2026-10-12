import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// PropPicker (#3118, package J): every property of a task through one picker —
// from the modal's «Свойства» by key, from a card's pills, from the ⌘K
// palette's field lists — against a real backend, so what the screen shows is
// what the server ended up with.

async function openTask(page, backend, label, extra = {}) {
  const ctx = await backend.freshBoard(label)
  const { board, columns } = ctx
  const title = `Свойства ${label} ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    ...extra,
  })
  await openBoard(page, board.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  // The title field takes the focus on open; the field keys live outside it.
  await page.locator('.tm-meta').click()
  return { ...ctx, modal, task, title }
}
const card = (page, id) => page.getByTestId(`${id}-card`)

test('статус · S: цифра переносит в колонку, повтор S — следующее поле', async ({
  page,
  backend,
}) => {
  const { modal, task, columns } = await openTask(page, backend, 'pp-status')
  await page.keyboard.press('s')
  const picker = card(page, 'task-status-picker')
  await expect(picker).toBeVisible()
  // The row the picker stands at is drawn active; the cursor is on the current value.
  await expect(modal.getByTestId('task-row-status')).toHaveClass(/active/)
  await expect(picker.locator('.pp-row.on')).toHaveAttribute('data-row', `col:${columns[0].id}`)

  await page.keyboard.press('2')
  await expect(picker).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).column_id)
    .toBe(columns[1].id)
  // Esc closed nothing else: the modal is still there.
  await expect(modal).toBeVisible()

  // S again with the picker open walks to the next field: «Исполнители».
  await page.keyboard.press('s')
  await expect(picker).toBeVisible()
  await page.keyboard.press('s')
  await expect(card(page, 'task-assignee-picker')).toBeVisible()
  await expect(picker).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(card(page, 'task-assignee-picker')).toHaveCount(0)
  await expect(modal).toBeVisible()
})

test('приоритет · P: цифры 4…0, Esc закрывает только пикер', async ({ page, backend }) => {
  const { modal, task } = await openTask(page, backend, 'pp-prio')
  await page.keyboard.press('p')
  const picker = card(page, 'task-priority-picker')
  await expect(picker).toBeVisible({ timeout: 3000 })
  await expect(picker.locator('.pp-row')).toHaveCount(5)
  await page.keyboard.press('4')
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).priority).toBe(4)

  await page.keyboard.press('p')
  await expect(picker).toBeVisible({ timeout: 3000 })
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  await expect(modal).toBeVisible()
})

test('исполнители · A: «Назначить себя», ⇧⌫ снимает всех', async ({ page, backend }) => {
  const { task } = await openTask(page, backend, 'pp-asg')
  await page.keyboard.press('a')
  const picker = card(page, 'task-assignee-picker')
  await expect(picker).toBeVisible()
  await expect(picker.getByTestId('task-assignee-picker-search')).toBeFocused()
  await picker.getByTestId('prop-assignee-self').click()
  await expect
    .poll(async () => ((await backend.get(`/tasks/${task.id}`)).assignees || []).length)
    .toBe(1)
  // Several people at once: the picker stays open; the person moved to «Назначены».
  await expect(picker).toBeVisible()
  await expect(picker.getByTestId('prop-assignee-self')).toHaveCount(0)
  await expect(picker.locator('[aria-checked="true"]')).toHaveCount(1)

  await page.keyboard.press('Shift+Backspace')
  await expect
    .poll(async () => ((await backend.get(`/tasks/${task.id}`)).assignees || []).length)
    .toBe(0)
})

test('оценка · O: ввод «1д 2ч» с разбором, ошибка — под полем', async ({ page, backend }) => {
  const { task } = await openTask(page, backend, 'pp-est')
  await page.keyboard.press('o')
  const picker = card(page, 'task-estimate-picker')
  await expect(picker).toBeVisible()
  const input = picker.getByTestId('task-estimate-picker-input')
  await expect(input).toBeFocused()

  await input.fill('abc')
  await input.press('Enter')
  await expect(picker.getByTestId('task-estimate-picker-error')).toBeVisible()
  await expect(picker).toBeVisible()

  await input.fill('1д 2ч')
  await expect(picker.getByTestId('task-estimate-picker-preview')).toBeVisible()
  await input.press('Enter')
  await expect(picker).toHaveCount(0)
  // 1 working day of 8 h + 2 h, in minutes.
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).estimate).toBe(600)
})

test('этап · M: выбор строкой и ⌘↵ — новый этап поповер-формой', async ({ page, backend }) => {
  const { task, project } = await openTask(page, backend, 'pp-stage')
  const ms = await backend.post(`/projects/${project.id}/milestones`, { title: 'Спринт e2e' })
  // The board reads its stages on load: reopen it so the list has the new one.
  await page.reload()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })
  await page.locator('.tm-meta').click()

  await page.keyboard.press('m')
  const picker = card(page, 'task-milestone-picker')
  await expect(picker).toBeVisible()
  await picker.getByTestId(`prop-milestone-${ms.id}`).click()
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).milestone_id).toBe(ms.id)

  await page.keyboard.press('m')
  await expect(picker).toBeVisible()
  await page.keyboard.type('Релиз e2e')
  await page.keyboard.press('Control+Enter')
  const form = page.getByTestId('task-milestone-picker-form-card')
  await expect(form).toBeVisible()
  await expect(form.getByTestId('task-milestone-picker-form-title')).toHaveValue('Релиз e2e')
  await form.getByTestId('task-milestone-picker-form-title').press('Enter')
  await expect(form).toHaveCount(0)
  await expect
    .poll(async () => {
      const id = (await backend.get(`/tasks/${task.id}`)).milestone_id
      const list = await backend.get(`/projects/${project.id}/milestones`)
      return list.find((m) => m.id === id)?.title
    })
    .toBe('Релиз e2e')
})

test('родитель · U: поиск по доске, ⌫ открепляет', async ({ page, backend }) => {
  const { task, board, columns } = await openTask(page, backend, 'pp-parent')
  const other = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[1].id,
    title: 'Родительская для e2e',
  })
  await page.keyboard.press('u')
  const picker = card(page, 'task-parent-picker')
  await expect(picker).toBeVisible()
  await page.keyboard.type('Родительская')
  await expect(picker.getByTestId(`prop-parent-${other.id}`)).toBeVisible()
  await page.keyboard.press('Enter')
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).parent_id).toBe(other.id)
  // The modal stays on the task, now a subtask.
  await expect(page.getByTestId('task-modal')).toBeVisible()

  await page.keyboard.press('u')
  await expect(picker).toBeVisible()
  await expect(picker.getByTestId('prop-parent-current')).toBeVisible()
  await page.keyboard.press('Backspace')
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).parent_id).toBe(null)
})

test('связь · ⇧L: поповер-форма, ⌥2 — вид, ↵ связывает и остаётся', async ({ page, backend }) => {
  const { modal, task, board, columns } = await openTask(page, backend, 'pp-rel')
  const a = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: 'Первая для связи',
  })
  const b = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: 'Вторая для связи',
  })
  await page.keyboard.press('Shift+L')
  const form = page.getByTestId('rel-form-card')
  await expect(form).toBeVisible()
  const input = form.getByTestId('rel-number')
  await expect(input).toBeFocused()
  await page.keyboard.press('Alt+2')
  await expect(form.getByTestId('rel-kind-blocks')).toHaveAttribute('aria-checked', 'true')
  await input.fill(`#${a.number}`)
  await input.press('Enter')
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}/relations`)).length).toBe(1)
  // Still open, ready for the next link.
  await expect(form).toBeVisible()
  await expect(input).toHaveValue('')
  await input.fill('Вторая для')
  await input.press('Enter')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}/relations`)).map((r) => r.kind).sort())
    .toEqual(['blocks', 'blocks'])
  await expect(form.getByTestId('rel-linked')).toHaveCount(2)
  void b
  await page.keyboard.press('Escape')
  await expect(form).toHaveCount(0)
  await expect(modal).toBeVisible()
})

test('карточка: пилл приоритета и людей открывают тот же пикер', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('pp-card')
  const title = `Пиллы ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })
  await openBoard(page, board.id)
  const c = cardsIn(page, columns[0].name).filter({ hasText: title })
  await c.getByTestId('card-pill-priority').click()
  const picker = card(page, 'card-priority-picker')
  await expect(picker).toBeVisible()
  await expect(c.getByTestId('card-pill-priority')).toHaveClass(/pp-open/)
  await picker.getByTestId('prop-priority-4').click()
  await expect(picker).toHaveCount(0)
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).priority).toBe(4)
  // A click on the pill doesn't open the task.
  await expect(page.getByTestId('task-modal')).toHaveCount(0)

  await c.getByTestId('card-pill-assignees').click()
  const people = card(page, 'card-assignee-picker')
  await expect(people).toBeVisible()
  await people.getByTestId('prop-assignee-self').click()
  await expect
    .poll(async () => ((await backend.get(`/tasks/${task.id}`)).assignees || []).length)
    .toBe(1)
  await page.keyboard.press('Escape')
  await expect(people).toHaveCount(0)
})

test('палитра ⌘K: Tab на «Статус» — строки PropPicker, ↵ применяет', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('pp-pal')
  const title = `Палитра ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })
  await openBoard(page, board.id)
  await page.keyboard.press('Control+k')
  const input = page.getByTestId('palette-input')
  await expect(input).toBeFocused()
  await input.fill(`#${task.number}`)
  await expect(page.locator(`[data-row="task:${task.id}"]`)).toBeVisible()
  await input.press('Tab')
  await expect(page.locator('[data-row="status"]')).toBeVisible()
  await input.press('Tab')
  const rows = page.getByTestId('palette-picker-card')
  await expect(rows).toBeVisible()
  await expect(page.getByTestId('palette-field-chip')).toBeVisible()
  // The cursor starts on the current column; ↓ is the next one.
  await expect(rows.locator('.pp-row.on')).toHaveAttribute('data-row', `col:${columns[0].id}`)
  await input.press('ArrowDown')
  await input.press('Enter')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).column_id)
    .toBe(columns[1].id)
})
