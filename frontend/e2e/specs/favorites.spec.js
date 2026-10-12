import { test, expect, openBoard, dragRow } from '../fixtures.js'

// Favourite boards (#3115): the star after the crumbs (and ⇧F) puts the board
// into «Избранное» on top of the sidebar tree; a row there opens the board,
// drags to a new place and comes off from its menu with an undo. The palette
// lists the favourites as a group of their own while nothing is typed.
const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

test('избранное: звезда → строка в сайдбаре → переход → перестановка → убрать', async ({
  page,
  backend,
}) => {
  const { ws, project, board } = await backend.freshBoard('fav')
  const second = await backend.post(`/projects/${project.id}/boards`, { name: 'Вторая доска' })
  const favs = async () =>
    (await backend.get(`/workspaces/${ws.id}/favorites`)).map((f) => f.board_id)

  await openBoard(page, board.id)
  const section = page.getByTestId('sidebar-favorites')
  await expect(section).toHaveCount(0)

  // The star — the board goes into «Избранное», the star fills.
  const star = page.getByTestId('crumb-favorite')
  await expect(star).toHaveAttribute('aria-pressed', 'false')
  await star.click()
  await expect(star).toHaveAttribute('aria-pressed', 'true')
  await expect(section).toBeVisible()
  const rows = section.getByTestId('favorite-row')
  await expect(rows).toHaveCount(1)
  await expect(rows.first()).toContainText(board.name)
  await expect(rows.first().getByTestId('favorite-project')).toHaveText(project.name)
  await expect.poll(favs).toEqual([board.id])

  // ⇧F does the same: off, and on again.
  await page.mouse.click(10, 880) // off any field: the keys go to the board
  await page.keyboard.press('Shift+F')
  await expect(star).toHaveAttribute('aria-pressed', 'false')
  await expect(section).toHaveCount(0)
  await expect.poll(favs).toEqual([])
  await page.keyboard.press('Shift+F')
  await expect(star).toHaveAttribute('aria-pressed', 'true')
  await expect.poll(favs).toEqual([board.id])

  // The second board, starred from its own crumbs.
  await openBoard(page, second.id)
  await page.getByTestId('crumb-favorite').click()
  await expect(rows).toHaveCount(2)
  await expect.poll(favs).toEqual([board.id, second.id])
  await expect(rows.nth(1)).toHaveClass(/active/)

  // A row opens its board.
  await rows.first().click()
  await expect(page).toHaveURL(new RegExp(`/board/${board.slug}$`))
  await expect(rows.first()).toHaveClass(/active/)

  // Drag the second above the first.
  await dragRow(page, rows.nth(1), rows.first())
  await expect.poll(favs).toEqual([second.id, board.id])
  await expect(rows.first()).toContainText('Вторая доска')

  // Off from the row's menu — at once, and the toast takes it back in place.
  await rows.first().hover()
  await rows.first().getByTestId('favorite-menu').click()
  await page.getByTestId('favorite-remove').click()
  await expect(rows).toHaveCount(1)
  await expect.poll(favs).toEqual([board.id])
  await page.getByTestId('undo-toast-undo').click()
  await expect(rows).toHaveCount(2)
  await expect.poll(favs).toEqual([second.id, board.id])
  await expect(rows.first()).toContainText('Вторая доска')
})

test('палитра: группа «Избранное» без запроса, избранные — первыми в найденных', async ({
  page,
  backend,
}) => {
  const { project, board } = await backend.freshBoard('fav-pal')
  const tag = Date.now().toString(36)
  const plain = await backend.post(`/projects/${project.id}/boards`, { name: `Доска ${tag} А` })
  const starred = await backend.post(`/projects/${project.id}/boards`, { name: `Доска ${tag} Б` })
  await backend.put(`/boards/${starred.id}/favorite`, null)

  await openBoard(page, board.id)
  await page.keyboard.press(`${MOD}+k`)
  const input = page.getByTestId('palette-input')
  await expect(input).toBeFocused()
  const palette = page.getByTestId('command-palette')
  await expect(palette.locator('[data-group="favorites"]')).toBeVisible()
  await expect(palette.locator(`[data-row="fav:${starred.id}"]`)).toBeVisible()
  await expect(palette.locator(`[data-row="fav:${plain.id}"]`)).toHaveCount(0)

  // Typed: both boards found, the favourite first.
  await input.fill(tag)
  const found = palette.locator('[data-row^="board:"]')
  await expect(found).toHaveCount(2)
  await expect(found.first()).toHaveAttribute('data-row', `board:${starred.id}`)

  // ↵ on it opens the board.
  await input.fill('')
  await expect(palette.locator(`[data-row="fav:${starred.id}"]`)).toBeVisible()
  await palette.locator(`[data-row="fav:${starred.id}"]`).click()
  await expect(page).toHaveURL(new RegExp(`/board/${starred.slug}$`))
})
