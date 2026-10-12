import { test, expect, openBoard } from '../fixtures.js'

// Palette commands and task templates (#3069): a template and a command made in
// the «Команды» window; «>bug» in ⌘K then makes a task from the template on the
// open board — description, priority, subtask — and opens it.

const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

async function openWindow(page, name) {
  await page.evaluate((n) => {
    const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
    pinia._s.get('wsDialogs').open(n)
  }, name)
}

test('команды: шаблон «Баг-репорт» и >bug в палитре создают задачу', async ({ page, backend }) => {
  const { ws, board } = await backend.freshBoard('macros')
  const stamp = Date.now().toString(36)
  await openBoard(page, board.id, ws.id)
  await openWindow(page, 'commands')
  await expect(page.getByTestId('commands-window')).toBeVisible()

  // A template.
  await page.getByTestId('commands-tab-templates').click()
  await page.getByTestId('template-new').click()
  await page.getByTestId('template-name').fill(`Баг-репорт ${stamp}`)
  await page.getByTestId('template-title').fill(`Баг ${stamp}`)
  await page.getByTestId('template-description').fill('## Шаги\n- [ ] воспроизвести')
  await page.getByTestId('template-subtasks').fill('Написать тест')
  await page.getByTestId('template-save').click()
  await expect(page.getByTestId('template-row')).toHaveCount(1)

  // A command that uses it.
  await page.getByTestId('commands-tab-macros').click()
  await page.getByTestId('macro-new').click()
  await page.getByTestId('macro-name').fill('Новый баг')
  await page.getByTestId('macro-trigger').fill(`bug${stamp}`)
  await page.locator('[data-testid="macro-form"] .n-select').nth(1).click()
  await page.locator('.n-base-select-option', { hasText: `Баг-репорт ${stamp}` }).click()
  await page.getByTestId('macro-save').click()
  await expect(page.getByTestId('macro-row')).toHaveCount(1)
  if (process.env.F3069_SHOT) await page.screenshot({ path: process.env.F3069_SHOT })
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('commands-window')).toHaveCount(0)

  // ⌘K «>bug…» runs it on the open board.
  await page.keyboard.press(`${MOD}+k`)
  await page.getByTestId('palette-input').fill(`>bug${stamp}`)
  const row = page.getByTestId('command-palette').locator('[data-row^="macro:"]')
  await expect(row).toHaveCount(1)
  await row.click()
  await expect(page.getByTestId('task-modal')).toBeVisible({ timeout: 15000 })
  await expect(page.getByTestId('task-modal')).toContainText('Написать тест')

  const tasks = await backend.get(`/boards/${board.id}/tasks`)
  const list = Array.isArray(tasks) ? tasks : tasks.tasks || []
  const made = list.find((x) => x.title === `Баг ${stamp}`)
  expect(made).toBeTruthy()
  const detail = await backend.get(`/tasks/${made.id}`)
  expect(detail.description).toContain('- [ ] воспроизвести')
  expect(detail.subtasks.map((s) => s.title)).toEqual(['Написать тест'])
})
