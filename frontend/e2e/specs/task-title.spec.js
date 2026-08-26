import { test, expect, openBoard } from '../fixtures.js'

// #2813: quick-add submitted on keyup.enter while the textarea inserts its
// newline on keydown, so `.prevent` came too late. `.trim()` hid it whenever the
// caret sat at the end — the bug only showed when Enter was pressed mid-string,
// which is exactly what happens after fixing a letter earlier in the title.
//
// The assertion goes to the API, not the DOM: the card renders the title as HTML
// (a newline reads as an innocent space) and the modal's <input> drops newlines
// from value, so both surfaces would happily show a green test over a broken row.
test('Enter в середине заголовка не оставляет перенос строки', async ({ page, backend }) => {
  const { board } = await backend.freshBoard('title')
  await openBoard(page, board.id)

  const col = page.getByTestId('column').first()
  await col.getByTestId('add-task-button').click()
  const input = col.getByTestId('add-task-input').locator('textarea')
  await input.fill('Окно Что нового')
  // Park the caret right after «Ч», where the old handler wedged the newline in.
  for (let i = 0; i < 'то нового'.length; i++) await input.press('ArrowLeft')
  await input.press('Enter')

  await expect(col.getByTestId('task-card').first()).toBeVisible()
  const tasks = await backend.get(`/boards/${board.id}/tasks`)
  const titles = tasks.map((t) => t.title)
  expect(titles).toContain('Окно Что нового')
  for (const t of titles) expect(t).not.toMatch(/[\n\r\t]/)
})
