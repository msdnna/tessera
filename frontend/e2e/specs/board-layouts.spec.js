import { test, expect, openBoard } from '../fixtures.js'

// The board's other views on the real server (#2973, #3009): the list's keys
// complete a row and rename one in place; the calendar moves a due by drag and
// keeps it date-only (the same day after a reload).
async function switchTo(page, layout) {
  await page.locator(`[data-tour="board-layout"] [data-layout="${layout}"]`).click()
}

test('список: стрелки и E выполняют строку, F2 переименовывает на месте', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('lv-keys')
  const a = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: 'Строка один',
  })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: 'Строка два' })

  await openBoard(page, board.id)
  await switchTo(page, 'list')
  await expect(page.getByTestId('list-row')).toHaveCount(2)

  await page.keyboard.press('ArrowDown')
  await expect(
    page.locator(`[data-testid="list-row"][data-task-id="${a.id}"].cursor`),
  ).toBeVisible()
  await page.keyboard.press('F2')
  const input = page.getByTestId('list-row-rename').locator('input')
  await expect(input).toBeFocused()
  await input.fill('Строка один — новое имя')
  await input.press('Enter')
  await expect
    .poll(async () => (await backend.get(`/tasks/${a.id}`)).title)
    .toBe('Строка один — новое имя')

  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowUp')
  await page.keyboard.press('e')
  await expect.poll(async () => !!(await backend.get(`/tasks/${a.id}`)).completed_at).toBe(true)
})

test('календарь: задача, перетащенная на другой день, получает этот срок', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('cal-dnd')
  const now = new Date()
  const y = now.getFullYear()
  const m = now.getMonth()
  // A date-only due on the 10th, dragged to the 20th of the same month.
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: 'Перенос срока',
    due_date: new Date(Date.UTC(y, m, 10)).toISOString(),
  })

  await openBoard(page, board.id)
  await switchTo(page, 'calendar')
  const pill = page.locator(`[data-testid="calendar-pill"][data-task-id="${task.id}"]`)
  await expect(pill).toBeVisible()
  const target = page.locator(`[data-day="${y}-${m}-20"]`)
  const a = await pill.boundingBox()
  const b = await target.boundingBox()
  await page.mouse.move(a.x + 10, a.y + 8)
  await page.mouse.down()
  await page.mouse.move(a.x + 30, a.y + 20, { steps: 4 })
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 10 })
  await expect(target).toHaveClass(/drop/)
  await page.mouse.up()

  // The instant, not the string: the server writes timestamptz with its own
  // offset (…T03:00:00+03:00 is the same UTC midnight).
  await expect
    .poll(async () => Date.parse((await backend.get(`/tasks/${task.id}`)).due_date))
    .toBe(Date.UTC(y, m, 20))
  await page.reload()
  await expect(page.locator(`[data-day="${y}-${m}-20"] [data-testid="calendar-pill"]`)).toHaveCount(
    1,
  )
})
