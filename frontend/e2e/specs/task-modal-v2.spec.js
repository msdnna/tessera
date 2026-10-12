import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// The v2 task modal (#3113): the description opens its editor in place, the tag
// picker, the two-step comment delete and the link form — end to end, against a
// real backend, so what the screen shows is what the server ended up with.

async function openTask(page, backend, label, extra = {}) {
  const { ws, project, board, columns } = await backend.freshBoard(label)
  const title = `Задача v2 ${label} ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    ...extra,
  })
  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  return { modal, task, project, board, columns }
}

test('описание: клик по тексту — правка, ⌘↵ сохраняет, Esc с правками спрашивает', async ({
  page,
  backend,
}) => {
  const { modal, task } = await openTask(page, backend, 'desc', {
    description: 'Первый абзац и [ссылка](https://example.com).',
  })
  const prose = modal.getByTestId('desc-prose')
  await expect(prose).toContainText('Первый абзац')

  await prose.locator('p').click({ position: { x: 10, y: 8 } })
  const field = modal.locator('.tm-desc textarea')
  await expect(field).toBeFocused()
  await expect(modal.getByTestId('md2-bar')).toBeVisible()
  await field.press('Control+End')
  await field.pressSequentially(' Дописано.')
  await field.press('Control+Enter')
  await expect(field).toHaveCount(0)
  await expect(prose).toContainText('Дописано.')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).description)
    .toContain('Дописано.')

  // Again, now with Esc: the typing is asked about, «Да» puts the text back.
  await page.keyboard.press('Shift+E')
  await expect(field).toBeFocused()
  await field.press('Control+End')
  await field.pressSequentially(' Лишнее.')
  await field.press('Escape')
  await expect(modal.getByTestId('md2-discard')).toBeVisible()
  // The modal itself is still open — Esc stayed with the editor.
  await expect(modal).toBeVisible()
  await field.press('Enter')
  await expect(field).toHaveCount(0)
  await expect(prose).not.toContainText('Лишнее.')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).description)
    .not.toContain('Лишнее.')
})

test('пустое описание — строка «Добавить описание…», не открытый редактор', async ({
  page,
  backend,
}) => {
  const { modal } = await openTask(page, backend, 'empty')
  await expect(modal.locator('.tm-desc textarea')).toHaveCount(0)
  await modal.getByTestId('desc-add').click()
  await expect(modal.locator('.tm-desc textarea')).toBeFocused()
})

test('теги: ↵ по найденному, создание по ⌘↵, Esc закрывает только пикер', async ({
  page,
  backend,
}) => {
  const { modal, task, project } = await openTask(page, backend, 'tags')
  await backend.post(`/projects/${project.id}/tags`, { name: 'дизайн', color: '#eb2f96' })
  // The board store reads the project's tags on load.
  await page.reload()
  await expect(modal).toBeVisible()

  await modal.getByTestId('task-tags-open').click()
  const picker = page.getByTestId('task-tag-picker')
  const search = picker.getByTestId('tag-picker-search')
  await expect(search).toBeFocused()

  // An existing tag by name: ↵ puts it on.
  await search.fill('диз')
  await search.press('Enter')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).tags?.map((t) => t.name))
    .toEqual(['дизайн'])

  // A new one: ⌘↵ creates it and puts it on.
  const name = `новый-${Date.now().toString(36)}`
  await search.fill(name)
  await expect(picker.getByTestId('tag-picker-create')).toContainText(name)
  await search.press('Control+Enter')
  await expect
    .poll(async () =>
      ((await backend.get(`/tasks/${task.id}`)).tags || []).map((t) => t.name).sort(),
    )
    .toEqual(['дизайн', name].sort())

  // Esc closes the picker only.
  await search.press('Escape')
  await expect(picker).toHaveCount(0)
  await expect(modal).toBeVisible()
})

test('комментарий удаляется со второго нажатия', async ({ page, backend }) => {
  const { modal, task } = await openTask(page, backend, 'cdel')
  await backend.post(`/tasks/${task.id}/comments`, { body: 'Удалить меня' })
  await page.reload()
  await expect(modal).toBeVisible()
  const row = modal.locator('.comment').filter({ hasText: 'Удалить меня' })
  await row.hover()
  await row.getByTestId('comment-delete').click()
  // Armed, not gone.
  await expect(row.getByTestId('comment-delete-armed')).toBeVisible()
  expect((await backend.get(`/tasks/${task.id}/comments`)).length).toBe(1)
  await row.getByTestId('comment-delete-armed').click()
  await expect(modal.locator('.comment').filter({ hasText: 'Удалить меня' })).toHaveCount(0)
  await expect.poll(async () => (await backend.get(`/tasks/${task.id}/comments`)).length).toBe(0)
})

test('связь: форма у «Связать» — вид сегментом, задача по названию', async ({ page, backend }) => {
  const { modal, task, board, columns } = await openTask(page, backend, 'rel')
  const other = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: 'Экспорт в PDF для связи',
  })
  await modal.getByTestId('rel-open').click()
  const form = page.getByTestId('rel-form-card')
  await expect(form).toBeVisible()
  await form.getByTestId('rel-kind-blocks').click()
  const input = form.getByTestId('rel-number')
  await expect(input).toBeFocused()
  await input.fill('Экспорт в PDF для')
  await input.press('Enter')
  // Package J (#3118): the form stays open for the next link; Esc closes it.
  await expect(input).toHaveValue('')
  await page.keyboard.press('Escape')
  await expect(form).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}/relations`)).map((r) => r.kind))
    .toEqual(['blocks'])
  await expect(modal.locator('.relrow')).toContainText(`#${other.number}`)
})
