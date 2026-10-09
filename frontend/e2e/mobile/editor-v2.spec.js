import { test, expect, openBoard } from '../fixtures.js'

// The comment composer on a phone (#3118, PackageJ «Композер, клавиатура и
// Aa»): «Aa» opens one row of 40px targets, the send button is the 44px arrow,
// and a comment goes out from the keyboard. The fullscreen editor starts on
// «Написать» here — two panes side by side don't fit a phone.
test('телефон: композер — «Aa», крупные цели, отправка; полноэкранный с «Написать»', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('mob-editor')
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Композер с телефона ${Date.now().toString(36)}`,
  })
  await openBoard(page, board.id)
  await page.goto(`/board/${board.id}?task=${task.number}`)
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })

  const field = modal.locator('.comment-add textarea')
  await field.scrollIntoViewIfNeeded()
  await field.tap()
  await modal.getByTestId('composer-format').tap()
  const strip = modal.getByTestId('md2-strip')
  await expect(strip).toBeVisible()
  const bold = strip.getByTestId('md2-tool-bold')
  expect((await bold.boundingBox()).height).toBeGreaterThanOrEqual(40)
  const send = modal.getByTestId('composer-send')
  expect((await send.boundingBox()).width).toBeGreaterThanOrEqual(44)

  await field.fill('Посмотрю на светлой после обеда')
  await send.tap()
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}/comments`)).map((c) => c.body))
    .toContain('Посмотрю на светлой после обеда')

  // Leave the strip off for whoever signs in next.
  await modal.getByTestId('composer-format').tap()
  await expect(strip).toHaveCount(0)

  // The description's «На весь экран»: the window takes the whole phone and
  // starts on «Написать».
  const add = modal.getByTestId('desc-add')
  await add.scrollIntoViewIfNeeded()
  await add.tap()
  await modal.locator('.tm-desc').getByTestId('md2-tool-expand').tap()
  const win = page.getByTestId('md-fullscreen')
  await expect(win).toBeVisible()
  await expect(win.getByTestId('fs-mode-write')).toHaveAttribute('aria-selected', 'true')
  const box = await win.boundingBox()
  expect(box.width).toBeGreaterThanOrEqual(388)
  await win.getByTestId('md-fullscreen-close').tap()
  await expect(win).toHaveCount(0)
})
