import { test, expect, column, cardsIn, dragCard, openBoard } from '../fixtures.js'

// Drag-and-drop is the board's core interaction and the easiest thing to break
// silently: a card can *look* moved and never be persisted, so every assertion
// here is followed by a reload.
test('карточка переносится в соседнюю колонку и остаётся там после перезагрузки', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('dnd')
  const [from, to] = columns
  const title = `Перетащи меня ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: from.id, title })

  await openBoard(page, board.id)
  const card = cardsIn(page, from.name).filter({ hasText: title })
  await expect(card).toBeVisible()

  await dragCard(page, card, column(page, to.name))

  await expect(cardsIn(page, to.name).filter({ hasText: title })).toBeVisible()
  await expect(cardsIn(page, from.name).filter({ hasText: title })).toHaveCount(0)

  // Persisted, not just reordered in the local list.
  await page.reload()
  await expect(cardsIn(page, to.name).filter({ hasText: title })).toBeVisible()
  await expect(cardsIn(page, from.name).filter({ hasText: title })).toHaveCount(0)

  // ...and the server agrees, independently of what the UI paints.
  const tasks = await backend.get(`/boards/${board.id}/tasks`)
  const moved = tasks.find((t) => t.title === title)
  expect(moved.column_id).toBe(to.id)
})
