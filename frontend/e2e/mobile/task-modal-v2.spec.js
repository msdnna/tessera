import { test, expect, openBoard } from '../fixtures.js'

// The v2 task modal on a phone (#3113). The tag picker and the menus of the
// editor are sheets here — and they live INSIDE the task modal, whose focus trap
// pulls any focus outside it back to its first element: a sheet in <body> would
// show a search field that can't be typed into. This drives the keyboard
// through the sheet to prove the focus stays.
test('телефон: пикер тегов — лист внутри модалки, поиск принимает ввод', async ({
  page,
  backend,
}) => {
  const { project, board, columns } = await backend.freshBoard('mob-tags')
  await backend.post(`/projects/${project.id}/tags`, { name: 'мобильное', color: '#18a058' })
  const title = `Теги с телефона ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
  })
  await openBoard(page, board.id)
  await page.goto(`/board/${board.id}?task=${task.number}`)
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })

  const open = modal.getByTestId('task-tags-open')
  await open.scrollIntoViewIfNeeded()
  await open.tap()
  const sheet = page.getByTestId('task-tag-picker')
  await expect(sheet).toBeVisible()
  // Inside the modal, not in <body> beside it.
  expect(await sheet.evaluate((el) => !!el.closest('.n-modal-container'))).toBe(true)

  const search = sheet.getByTestId('tag-picker-search')
  await search.tap()
  await expect(search).toBeFocused()
  await page.keyboard.type('моб')
  await expect(search).toHaveValue('моб')
  await page.keyboard.press('Enter')
  await expect
    .poll(async () => ((await backend.get(`/tasks/${task.id}`)).tags || []).map((t) => t.name))
    .toEqual(['мобильное'])
})
