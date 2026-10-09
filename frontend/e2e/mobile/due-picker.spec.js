import { test, expect, openBoard } from '../fixtures.js'

// The due picker on a phone (#3118): a sheet the full height of the screen,
// inside the task modal (its focus trap would kill a field in <body>); words,
// presets, and the doors — the calendar, the repeat — as the sheet's next pages.
//
// SHOTS=<dir> also saves the frames the visual check compares with the mockups.
const SHOTS = process.env.SHOTS || ''
async function shot(page, name) {
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/${name}.png` })
}

test('телефон: срок — лист на всю высоту, слова, календарь и повтор страницами', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('mob-due')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Срок с телефона ${Date.now().toString(36)}`,
  })
  await openBoard(page, board.id)
  await page.goto(`/board/${board.id}?task=${task.number}`)
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })

  const open = modal.getByTestId('task-due-open')
  await open.scrollIntoViewIfNeeded()
  await open.tap()
  const sheet = page.getByTestId('task-due-picker')
  await expect(sheet).toBeVisible()
  expect(await sheet.evaluate((el) => !!el.closest('.n-modal-container'))).toBe(true)

  // Words: the field takes the typing (the modal's trap keeps the focus).
  const field = sheet.getByTestId('due-input')
  await field.tap()
  await expect(field).toBeFocused()
  await page.keyboard.type('завтра 18:00')
  await expect(sheet.getByTestId('due-preview')).toContainText('18:00')
  await shot(page, 'mobile-main')
  await page.keyboard.press('Enter')
  await expect(sheet).toHaveCount(0)
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).due_date).toBeTruthy()

  // The calendar page: a day tap moves the due, «Готово» closes.
  await open.tap()
  await sheet.getByTestId('due-page-cal').tap()
  await expect(sheet.getByTestId('due-cal')).toBeVisible()
  await shot(page, 'mobile-cal')
  const before = (await backend.get(`/tasks/${task.id}`)).due_date
  await sheet.locator('[data-testid="due-cal"] .dc-day:not(.out):not(.sel)').first().tap()
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}`)).due_date).not.toBe(before)

  // Back, then the repeat page: daily.
  await sheet
    .getByRole('button', { name: /Срок|Due/ })
    .first()
    .tap()
  await sheet.getByTestId('due-door-rep').tap()
  await sheet.locator('[data-freq="daily"]').tap()
  await shot(page, 'mobile-rep')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).recurrence?.freq)
    .toBe('daily')
  await sheet.getByTestId('due-sheet-done').tap()
  await expect(sheet).toHaveCount(0)
})
