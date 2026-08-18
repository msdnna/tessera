import { test, expect, column, cardsIn, openBoard } from '../fixtures.js'

// Structural edits (new column, rename, delete) run against a board of their own:
// the DnD and realtime specs share the seeded board, and deleting a column out
// from under them would fail them for the wrong reason.
test('создание колонки и задачи, переименование и удаление колонки', async ({ page, backend }) => {
  const { board } = await backend.freshBoard('crud')
  await openBoard(page, board.id)

  const before = await page.getByTestId('column').count()

  // create a column
  await page.getByRole('button', { name: /Создать колонку/ }).click()
  await page.locator('.add-col input').fill('E2E колонка')
  await page.locator('.add-col input').press('Enter')
  await expect(column(page, 'E2E колонка')).toBeVisible()
  await expect(page.getByTestId('column')).toHaveCount(before + 1)

  // create a task inside it
  const col = column(page, 'E2E колонка')
  await col.getByTestId('add-task-button').click()
  await col.getByTestId('add-task-input').locator('textarea').fill('E2E задача')
  await col.getByTestId('add-task-input').locator('textarea').press('Enter')
  await expect(cardsIn(page, 'E2E колонка').filter({ hasText: 'E2E задача' })).toBeVisible()

  // the card survives a reload → it was really persisted, not just painted
  await page.reload()
  await expect(cardsIn(page, 'E2E колонка').filter({ hasText: 'E2E задача' })).toBeVisible()

  // rename the column (double-click the title, per ColumnHeader.vue)
  await column(page, 'E2E колонка').locator('.col-title').dblclick()
  const rename = column(page, 'E2E колонка').locator('input')
  await rename.fill('E2E переименована')
  await rename.press('Enter')
  await expect(column(page, 'E2E переименована')).toBeVisible()
  await expect(column(page, 'E2E колонка')).toHaveCount(0)

  // delete it (menu → «Удалить колонку» → confirm)
  await column(page, 'E2E переименована').locator('.col-menu').click()
  await page.getByRole('button', { name: /Удалить колонку/ }).click()
  await page.getByRole('button', { name: 'Удалить', exact: true }).click()
  await expect(column(page, 'E2E переименована')).toHaveCount(0)
  await expect(page.getByTestId('column')).toHaveCount(before)
})

test('задача, созданная по API, видна на доске после загрузки', async ({ page, backend, seed }) => {
  const title = `Из API ${Date.now().toString(36)}`
  await backend.createTask(seed.columns[0].id, title)

  await openBoard(page, seed.boardId)
  await expect(cardsIn(page, seed.columns[0].name).filter({ hasText: title })).toBeVisible()
})
