import { test, expect, openBoard } from '../fixtures.js'

// Favourite boards on a phone (#3115): the header has no room for the crumbs'
// star, so it is the first row of the board's menu; «Избранное» sits on top of
// the tree in the drawer, and a row there opens the board.

async function settle(page) {
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(300)
}

test('телефон: звезда в меню доски, «Избранное» в дровере', async ({ page, backend }) => {
  const { ws, project, board } = await backend.freshBoard('m-fav')
  const other = await backend.post(`/projects/${project.id}/boards`, { name: 'Другая доска' })
  const favs = async () =>
    (await backend.get(`/workspaces/${ws.id}/favorites`)).map((f) => f.board_id)

  await openBoard(page, board.id)
  await settle(page)
  await page.getByTestId('board-mobile-menu').tap()
  const star = page.getByTestId('board-action-favorite')
  await expect(star).toBeVisible()
  await page.waitForTimeout(350) // the slide-up
  await star.tap()
  await expect(page.getByTestId('board-mobile-menu-list')).toHaveCount(0)
  await expect.poll(favs).toEqual([board.id])

  // From another board, the drawer's «Избранное» leads back.
  await openBoard(page, other.id)
  await settle(page)
  await page.getByTestId('mobile-board-switch').tap()
  const drawer = page.locator('.n-drawer')
  await expect(drawer).toBeVisible()
  const rows = drawer.getByTestId('favorite-row')
  await expect(rows).toHaveCount(1)
  await expect(rows.first()).toContainText(board.name)
  await rows.first().tap()
  await expect(page).toHaveURL(new RegExp(`/board/${board.slug}$`))
  await expect(drawer).toHaveCount(0)

  // The menu row now takes the star off.
  await settle(page)
  await page.getByTestId('board-mobile-menu').tap()
  await page.waitForTimeout(350)
  await page.getByTestId('board-action-favorite').tap()
  await expect.poll(favs).toEqual([])
})
