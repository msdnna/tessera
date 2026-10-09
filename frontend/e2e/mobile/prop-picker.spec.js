import { test, expect, openBoard } from '../fixtures.js'

// PropPicker on a phone (#3118, PackageJ «Мобильный»): any property picker is
// a bottom sheet — the field's name and «Готово», a 44px search, 48px rows —
// living inside the task modal, whose focus trap would reject a sheet in <body>.
test('телефон: исполнители — лист с «Готово», поиск принимает ввод', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('mob-pp')
  const title = `Лист свойств ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })
  await openBoard(page, board.id)
  await page.goto(`/board/${board.id}?task=${task.number}`)
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })

  const row = modal.getByTestId('task-row-assignees')
  await row.scrollIntoViewIfNeeded()
  await row.tap()
  const sheet = page.getByTestId('task-assignee-picker')
  await expect(sheet).toBeVisible()
  expect(await sheet.evaluate((el) => !!el.closest('.n-modal-container'))).toBe(true)

  const search = sheet.getByTestId('task-assignee-picker-search')
  const box = await search.boundingBox()
  expect(Math.round(box.height)).toBeGreaterThanOrEqual(40)
  await search.tap()
  await expect(search).toBeFocused()
  await page.keyboard.type('зz')
  await expect(search).toHaveValue('зz')
  await search.fill('')

  const self = sheet.getByTestId('prop-assignee-self')
  expect(Math.round((await self.boundingBox()).height)).toBe(48)
  await self.tap()
  await expect
    .poll(async () => ((await backend.get(`/tasks/${task.id}`)).assignees || []).length)
    .toBe(1)

  await sheet.getByTestId('task-assignee-picker-done').tap()
  await expect(sheet).toHaveCount(0)
  await expect(modal).toBeVisible()
})
