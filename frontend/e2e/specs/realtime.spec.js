import { test, expect, cardsIn, openBoard, signIn } from '../fixtures.js'

// These are the only specs that exercise /api/ws end to end. They are also the
// reason `preview.proxy` had to be added to vite.config.js: without it the
// WebSocket upgrade never reaches the backend, and the board silently stops
// updating — which no amount of HTTP-level testing would catch.
test('правка «со стороны» по API прилетает на открытую доску без перезагрузки', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('rt-api')
  await openBoard(page, board.id, ws.id)

  const title = `Прилетело по WS ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })

  // No reload anywhere in this test: if the card shows up, the event travelled
  // backend → WS → proxy → store → DOM.
  await expect(cardsIn(page, columns[0].name).filter({ hasText: title })).toBeVisible()
})

test('две вкладки: карточка, созданная в одной, появляется во второй', async ({
  page,
  browser,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('rt-tabs')
  await openBoard(page, board.id, ws.id)

  // The second tab needs its own sign-in: the refresh token rotates, so the two
  // contexts cannot share one captured session (see fixtures.js).
  const other = await browser.newContext({ baseURL: page.url().split('/project')[0] })
  const otherPage = await other.newPage()
  try {
    await signIn(otherPage)
    await openBoard(otherPage, board.id, ws.id)

    const title = `Из соседней вкладки ${Date.now().toString(36)}`
    const col = cardsIn(page, columns[0].name)
    await expect(col).toHaveCount(0)

    const target = page.locator(`[data-column-name="${columns[0].name}"]`)
    await target.getByTestId('add-task-button').click()
    await target.getByTestId('add-task-input').locator('textarea').fill(title)
    await target.getByTestId('add-task-input').locator('textarea').press('Enter')

    await expect(cardsIn(page, columns[0].name).filter({ hasText: title })).toBeVisible()
    await expect(cardsIn(otherPage, columns[0].name).filter({ hasText: title })).toBeVisible()
  } finally {
    await other.close()
  }
})
