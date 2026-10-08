import { test, expect, openBoard } from '../fixtures.js'

// The «Проект» window, the stage page and the stage strip (v2, #2971).
const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

test('окно «Проект»: теги — создать, переименовать, слить, удалить и вернуть', async ({
  page,
  backend,
}) => {
  const { board, project } = await backend.freshBoard('pw-tags')
  const [todo] = await backend.get(`/boards/${board.id}/columns`)
  const a = await backend.post(`/projects/${project.id}/tags`, { name: 'design', color: '#eb2f96' })
  const b = await backend.post(`/projects/${project.id}/tags`, { name: 'дизайн', color: '#eb2f96' })
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: todo.id,
    title: 'Макет',
  })
  await backend.post(`/tasks/${task.id}/tags`, { tag_id: b.id })

  await openBoard(page, board.id)
  await page.getByTestId('board-more').click()
  await page.getByTestId('board-more-menu-tags').click()
  const tab = page.getByTestId('project-tags')
  await expect(tab).toBeVisible()

  // N — the new-tag field; «префикс::имя» lands in its group.
  await page.keyboard.press('n')
  await expect(page.getByTestId('tag-new-input')).toBeFocused()
  await page.getByTestId('tag-new-input').fill('app::web')
  await page.getByTestId('tag-new-input').press('Enter')
  await expect(tab.locator('[data-tag="app::web"]')).toBeVisible()

  // A click on the name renames — no double click.
  await tab.locator('[data-tag="design"] .tt-pill-btn').click()
  const field = tab.locator('[data-tag="design"] input')
  await field.fill('Design')
  await field.press('Enter')
  await expect(tab.locator('[data-tag="Design"]')).toBeVisible()

  // Two duplicates selected → merged into the most used one («дизайн»).
  await tab.locator('[data-tag="Design"] [data-testid="tag-check"]').click()
  await tab.locator('[data-tag="дизайн"] [data-testid="tag-check"]').click()
  await page.getByTestId('tag-merge').click()
  await page.getByTestId('tag-merge-form-submit').click()
  await expect(tab.locator('[data-tag="Design"]')).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/projects/${project.id}/tags`)).map((x) => x.id))
    .not.toContain(a.id)

  // ⌫ on a selection — gone at once; the toast brings it back with its task.
  await tab.locator('[data-tag="дизайн"] [data-testid="tag-check"]').click()
  await page.getByTestId('tag-delete-selected').click()
  await expect(tab.locator('[data-tag="дизайн"]')).toHaveCount(0)
  await page.getByTestId('undo-toast-undo').click()
  await expect(tab.locator('[data-tag="дизайн"]')).toBeVisible()
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).tags?.map((x) => x.id))
    .toEqual([b.id])

  // Esc closes the window.
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('project-window')).toHaveCount(0)
})

test('этапы: создать в окне, страница этапа с целями, доска этапа с полосой', async ({
  page,
  backend,
}) => {
  const { board, project } = await backend.freshBoard('pw-stages')
  const [todo] = await backend.get(`/boards/${board.id}/columns`)
  await backend.post(`/boards/${board.id}/tasks`, { column_id: todo.id, title: 'Вне этапа' })

  await openBoard(page, board.id)
  await page.getByTestId('board-more').click()
  await page.getByTestId('board-more-menu-stages').click()
  await expect(page.getByTestId('project-stages')).toBeVisible()

  // N — an empty row; ↵ creates the stage.
  await page.keyboard.press('n')
  const title = page.getByTestId('stage-new-title')
  await expect(title).toBeFocused()
  await title.fill('Спринт E2E')
  await title.press('Enter')
  const row = page.getByTestId('stage-row').filter({ hasText: 'Спринт E2E' })
  await expect(row).toBeVisible()

  // A click opens the stage page; E edits the goals, ⌘↵ saves them.
  await row.click()
  await expect(page.getByTestId('stage-page')).toBeVisible()
  await expect(page.getByTestId('stage-progress')).toBeVisible()
  await page.keyboard.press('e')
  const editor = page.getByTestId('stage-goals').locator('textarea')
  await expect(editor).toBeFocused()
  await editor.fill('1. Довести окно «Проект»')
  await page.keyboard.press(`${MOD}+Enter`)
  await expect(page.getByTestId('stage-goals')).toContainText('Довести окно «Проект»')
  await expect
    .poll(async () => {
      const list = await backend.get(`/projects/${project.id}/milestones`)
      return list.find((m) => m.title === 'Спринт E2E')?.description
    })
    .toBe('1. Довести окно «Проект»')

  // B — the stage's board, filtered, with the strip; ⇧H unfolds it.
  await page.keyboard.press('b')
  await expect(page).toHaveURL(/\?milestone=/)
  const strip = page.getByTestId('stage-strip')
  await expect(strip).toBeVisible()
  await expect(strip).toContainText('Довести окно «Проект»')
  await page.mouse.click(10, 880)
  const open = await strip.getAttribute('data-open')
  await page.keyboard.press('Shift+H')
  await expect(strip).toHaveAttribute('data-open', open === '1' ? '0' : '1')

  // The selector: «Все задачи» drops the filter and the strip with it.
  await page.getByTestId('stage-select').click()
  await page.getByTestId('stage-option-all').click()
  await expect(page).not.toHaveURL(/\?milestone=/)
  await expect(strip).toHaveCount(0)
})
