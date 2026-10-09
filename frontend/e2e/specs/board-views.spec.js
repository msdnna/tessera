import { test, expect, openBoard } from '../fixtures.js'

// The board's «Вид доски» panel and saved views (v2, #2970): ⇧V opens the panel,
// ⌘⇧S saves the board as a view, a change marks it «изменено» (the dot on the
// chip), ⌘S updates it, ⌫ on the row deletes it at once and the toast takes it
// back.
const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

test('представление: сохранить, «изменено», обновить, удалить и вернуть', async ({
  page,
  backend,
}) => {
  const { board } = await backend.freshBoard('views')
  await openBoard(page, board.id)
  await expect(page.locator('[data-testid="column"]').first()).toBeVisible()

  // ⇧V — the panel, docked next to the board.
  await page.keyboard.press('Shift+V')
  const panel = page.getByTestId('board-customize')
  await expect(panel).toBeVisible()

  // ⌘⇧S — «Сохранить текущее как…» right in the list.
  await page.keyboard.press(`${MOD}+Shift+S`)
  const field = page.getByTestId('views-save-input').locator('input')
  await expect(field).toBeFocused()
  const name = `Вид ${Date.now().toString(36)}`
  await field.fill(name)
  await field.press('Enter')
  const chip = page.getByTestId('view-chip')
  await expect(chip).toContainText(name)
  await expect(page.getByTestId('view-row-1')).toContainText(name)
  await expect.poll(async () => (await backend.get(`/boards/${board.id}/views`)).length).toBe(1)

  // ⇧C changes the card size → the loaded view reads «изменено».
  await page.mouse.click(10, 880) // off any field: the keys go to the board
  await page.keyboard.press('Shift+C')
  await expect(page.getByTestId('view-chip-changed')).toBeVisible()
  await expect(page.getByTestId('views-changed')).toBeVisible()

  // ⌘S updates the view; the dot goes.
  await page.keyboard.press(`${MOD}+S`)
  await expect(page.getByTestId('view-chip-changed')).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/boards/${board.id}/views`))[0].config.cardSize)
    .toBe('large')

  // ⌫ on the row — gone at once, with an undo toast that brings it back.
  await page.getByTestId('view-row-1').focus()
  await page.keyboard.press('Backspace')
  await expect(page.getByTestId('view-row-1')).toHaveCount(0)
  await expect(chip).toHaveCount(0)
  await page.getByTestId('undo-toast-undo').click()
  await expect(page.getByTestId('view-row-1')).toContainText(name)
  await expect.poll(async () => (await backend.get(`/boards/${board.id}/views`)).length).toBe(1)
})
