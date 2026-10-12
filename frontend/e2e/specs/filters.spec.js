import { test, expect, column, cardsIn, openBoard } from '../fixtures.js'

// Search filters the cards already on the board — no reload, no refetch — so the
// assertion is about what the header search (on a board it is the board's title
// filter, v2 #2954) does to the rendered lists.
test('поиск по названию оставляет на доске только совпавшие карточки', async ({
  page,
  backend,
}) => {
  const { board, columns } = await backend.freshBoard('flt-q')
  const stamp = Date.now().toString(36)
  const keep = `Найди меня ${stamp}`
  const drop = `Мимо ${stamp}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: keep })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: drop })

  await openBoard(page, board.id)
  await expect(cardsIn(page, columns[0].name)).toHaveCount(2)

  const search = page.getByTestId('header-search')
  await search.fill('Найди меня')
  await expect(cardsIn(page, columns[0].name).filter({ hasText: keep })).toBeVisible()
  await expect(cardsIn(page, columns[0].name).filter({ hasText: drop })).toHaveCount(0)

  // Clearing restores the full board: the filter is a view, it deletes nothing.
  await search.fill('')
  await expect(cardsIn(page, columns[0].name)).toHaveCount(2)
})

// Nothing on this board matches → the header search widens to the workspace,
// and picking a task from another board goes there (v2, #2954).
test('поиск без совпадений на доске показывает задачи пространства и ведёт на их доску', async ({
  page,
  backend,
}) => {
  const { project, board } = await backend.freshBoard('flt-ws')
  const other = await backend.post(`/projects/${project.id}/boards`, { name: 'Соседняя доска' })
  const otherCols = await backend.get(`/boards/${other.id}/columns`)
  const title = `Где-то рядом ${Date.now().toString(36)}`
  await backend.post(`/boards/${other.id}/tasks`, { column_id: otherCols[0].id, title })

  await openBoard(page, board.id)
  await page.getByTestId('header-search').fill(title)

  const hit = page.locator('.search .panel .row').filter({ hasText: title })
  await expect(hit).toBeVisible()
  await hit.click()
  await expect(page).toHaveURL(new RegExp(`/board/${other.slug}\\?task=`))
})

// Grouping the board by tags — columns become tags — is the feature Tessera has
// and its alternatives don't (CLAUDE.md). If it ever silently falls back to
// status columns, this is the spec that says so.
test('группировка по тегам: колонки становятся тегами (killer-фича)', async ({ page, backend }) => {
  const { project, board, columns } = await backend.freshBoard('flt-tags')
  const stamp = Date.now().toString(36)
  const tagName = `срочно-${stamp}`
  const tag = await backend.post(`/projects/${project.id}/tags`, { name: tagName })

  const tagged = `С тегом ${stamp}`
  const plain = `Без тега ${stamp}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: tagged,
  })
  await backend.post(`/tasks/${task.id}/tags`, { tag_id: tag.id })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: plain })

  await openBoard(page, board.id)

  // Composer → «+» → Группировка → По тегам (все).
  await page.locator('.facet-add').click()
  await page.getByTestId('facet-group').click()
  await page.getByTestId('facet-group-tag').click()

  // The tag now IS a column, and the tagged card lives in it.
  await expect(column(page, tagName)).toBeVisible()
  await expect(cardsIn(page, tagName).filter({ hasText: tagged })).toBeVisible()
  await expect(cardsIn(page, tagName).filter({ hasText: plain })).toHaveCount(0)

  // The status columns are gone — this really is a different grouping, not an
  // extra column bolted onto the old layout.
  await expect(column(page, columns[0].name)).toHaveCount(0)
})
