import { test, expect, openBoard } from '../fixtures.js'

// The editor of package J (#3118): the comment composer with «Aa» and the
// formatting keys, the @ # / lists at the caret, the bubble over read text and
// «Цитировать в ответ», the description's I / Esc / ⌘↵, and the fullscreen window
// with its ⌘1–3 — end to end, against a real backend.
//
// The modifier is Ctrl here: the suite runs on Linux, where `mod` is Ctrl.

async function openTask(page, backend, label, extra = {}) {
  const { ws, project, board, columns } = await backend.freshBoard(label)
  const title = `Редактор ${label} ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    ...extra,
  })
  await openBoard(page, board.id, ws.id)
  await page.goto(`/board/${board.id}?task=${task.number}`)
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible({ timeout: 15000 })
  return { modal, task, project, board, columns }
}

test('композер: «Aa» открывает строку и запоминается, ⌘B и ⌘⇧8 форматируют, ⌘↵ отправляет', async ({
  page,
  backend,
}) => {
  const { modal, task } = await openTask(page, backend, 'composer')
  const field = modal.locator('.comment-add textarea')
  await expect(modal.getByTestId('md2-strip')).toHaveCount(0)
  await modal.getByTestId('composer-format').click()
  await expect(modal.getByTestId('md2-strip')).toBeVisible()
  // The bubble's and the strip's tooltips name the key.
  await expect(modal.getByTestId('md2-tool-bold')).toHaveAttribute('aria-label', /.+/)

  await field.click()
  await field.pressSequentially('важно')
  await field.press('Shift+Home')
  await field.press('Control+b')
  await expect(field).toHaveValue('**важно**')
  await field.press('End')
  await field.press('Control+Shift+8')
  await expect(field).toHaveValue('- **важно**')
  await field.press('Control+Enter')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}/comments`)).map((c) => c.body))
    .toContain('- **важно**')

  // Remembered: the strip is there after a reload.
  await page.reload()
  await expect(page.getByTestId('task-modal').getByTestId('md2-strip')).toBeVisible({
    timeout: 15000,
  })
  await page.getByTestId('task-modal').getByTestId('composer-format').click()
})

test('@ # /: список у каретки, ↵ и Tab вставляют, Esc оставляет текст', async ({
  page,
  backend,
}) => {
  const { modal, board, columns } = await openTask(page, backend, 'suggest')
  const other = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[1].id,
    title: `Экспорт отчёта ${Date.now().toString(36)}`,
  })
  const field = modal.locator('.comment-add textarea')
  await field.click()

  // @ — the people, the task's own first.
  await field.pressSequentially('Посмотри @')
  const list = page.getByTestId('ed-suggest')
  await expect(list).toBeVisible()
  await expect(list.getByTestId('ed-suggest-row').first()).toBeVisible()
  await field.press('Enter')
  await expect(field).toHaveValue(/^Посмотри @\S.* $/)

  // # — tasks of the workspace by title, Tab inserts the number.
  await field.fill('')
  await field.pressSequentially('Похоже на #Экспорт')
  await expect(list.getByTestId('ed-suggest-row').first()).toContainText(`#${other.number}`)
  await field.press('Tab')
  await expect(field).toHaveValue(`Похоже на #${other.number} `)

  // Nothing found: the list says the text stays, Esc closes it and keeps the text.
  await field.pressSequentially('@зебра')
  await expect(page.getByTestId('ed-suggest-empty')).toBeVisible()
  await field.press('Escape')
  await expect(list).toHaveCount(0)
  await expect(field).toHaveValue(`Похоже на #${other.number} @зебра`)
  // That Esc was the list's: the modal is still open.
  await expect(modal).toBeVisible()

  // / — commands at the start of a line, in groups.
  await field.fill('')
  await field.pressSequentially('/')
  await expect(list).toContainText('/due')
  await expect(list).toContainText('/assign')
  await field.pressSequentially('clos')
  await field.press('Enter')
  await expect(field).toHaveValue('/close\n')
})

test('пузырь над текстом: «Цитировать в ответ» по Q кладёт цитату в композер', async ({
  page,
  backend,
}) => {
  const { modal, task } = await openTask(page, backend, 'quote')
  await backend.post(`/tasks/${task.id}/comments`, { body: 'Контраст чипа всё ещё низкий' })
  await page.reload()
  const bubble = modal.locator('.c-bubble').first()
  await expect(bubble).toContainText('Контраст чипа')
  // Select the comment's text.
  await bubble
    .locator('p')
    .first()
    .evaluate((p) => {
      const r = document.createRange()
      r.selectNodeContents(p)
      const sel = window.getSelection()
      sel.removeAllRanges()
      sel.addRange(r)
    })
  const rb = page.getByTestId('read-bubble')
  await expect(rb).toBeVisible()
  await expect(page.getByTestId('read-bubble-link')).toHaveAttribute('aria-disabled', 'true')
  await page.keyboard.press('q')
  const field = modal.locator('.comment-add textarea')
  await expect(field).toHaveValue('> Контраст чипа всё ещё низкий\n\n')
  await expect(field).toBeFocused()
})

test('описание: I открывает, Esc с правками спрашивает (Esc — продолжить, ↵ — отменить), ⌘↵ сохраняет', async ({
  page,
  backend,
}) => {
  const { modal, task } = await openTask(page, backend, 'desc', {
    description: '## Цель\n\nСтарый текст.',
  })
  await expect(modal.getByTestId('desc-prose')).toContainText('Старый текст')
  await page.keyboard.press('i')
  const field = modal.locator('.tm-desc textarea')
  await expect(field).toBeFocused()
  await expect(modal.locator('.tm-desc').getByTestId('md2-tint')).toContainText('## Цель')

  await field.press('Control+End')
  await field.pressSequentially(' Лишнее.')
  await field.press('Escape')
  const ask = modal.getByTestId('md2-discard-ask')
  await expect(ask).toBeVisible()
  await field.press('Escape') // «Продолжить»
  await expect(ask).toHaveCount(0)
  await expect(field).toHaveValue(/Лишнее\.$/)
  await field.press('Escape')
  await expect(ask).toBeVisible()
  await field.press('Enter') // «Отменить правки»
  await expect(field).toHaveCount(0)
  await expect(modal.getByTestId('desc-prose')).not.toContainText('Лишнее')

  // ⇧E is still a way in; ⌘↵ saves and leaves.
  await page.keyboard.press('Shift+E')
  await expect(field).toBeFocused()
  await field.press('Control+End')
  await field.pressSequentially(' Новое.')
  await field.press('Control+Enter')
  await expect(field).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).description)
    .toContain('Новое.')
})

test('полноэкранный: «Рядом» по умолчанию, ⌘1–3 режимы, Esc сворачивает с текстом, ⌘↵ — готово', async ({
  page,
  backend,
}) => {
  const { modal, task } = await openTask(page, backend, 'full', {
    description: 'Первый абзац.',
  })
  await page.keyboard.press('i')
  await modal.getByTestId('md2-tool-expand').click()
  const win = page.getByTestId('md-fullscreen')
  await expect(win).toBeVisible()
  await expect(win.getByTestId('fs-mode-split')).toHaveAttribute('aria-selected', 'true')
  await expect(win.locator('.md2-preview-side')).toBeVisible()
  const field = win.locator('textarea')
  await expect(field).toBeFocused()

  await field.press('Control+1')
  await expect(win.getByTestId('fs-mode-write')).toHaveAttribute('aria-selected', 'true')
  await expect(win.locator('.md2-preview-side')).toHaveCount(0)
  await field.press('Control+End')
  await field.pressSequentially(' Из окна.')

  await field.press('Control+3')
  await expect(win.getByTestId('md2-preview-pane')).toContainText('Из окна.')
  await page.keyboard.press('Control+2')
  await expect(win.locator('.md2-preview-side')).toBeVisible()

  // Esc folds the window; the text is in the inline editor, which is still open.
  await win.locator('textarea').press('Escape')
  await expect(win).toHaveCount(0)
  const inline = modal.locator('.tm-desc textarea')
  await expect(inline).toHaveValue('Первый абзац. Из окна.')

  // ⌘↵ in the window is «Готово»: the description saves and leaves its editor.
  await modal.getByTestId('md2-tool-expand').click()
  await expect(win).toBeVisible()
  await win.locator('textarea').press('Control+Enter')
  await expect(win).toHaveCount(0)
  await expect(inline).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).description)
    .toBe('Первый абзац. Из окна.')
})
