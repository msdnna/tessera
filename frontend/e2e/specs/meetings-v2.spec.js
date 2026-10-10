import { test, expect } from '../fixtures.js'
import { api, login, newCredentials, register } from '../api.js'

// Meetings v2 (#3034), end to end: the plan of a meeting — its length and repeat
// (#3052) and its stage (#3053) — through the «Запланировать» popover form, the
// list, the lobby and «Изменить»; and the call chat's «/» task and @-mention
// (#3055). Reactions travel over the SFU's data channel, which this stand does
// not run (the token answers 503) — they are covered by the unit tests.

async function openConferences(page, workspaceId) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), workspaceId)
  await page.goto('/conferences')
  await expect(page.getByTestId('conference-schedule')).toBeEnabled()
}

test('встреча: длительность, повтор и этап — в форме, списке, лобби и правке', async ({
  page,
  backend,
  seed,
}) => {
  const stage = await backend.post(`/projects/${seed.projectId}/milestones`, {
    title: `Спринт ${seed.runId}`,
  })
  const title = `E2E план ${seed.runId}`
  await openConferences(page, seed.workspaceId)

  await page.getByTestId('conference-schedule').click()
  await page.getByTestId('conference-title').fill(title)
  // Repeat needs a start: the button is off until a time is picked.
  await expect(page.getByTestId('conference-repeat')).toBeDisabled()
  // The picker's own panel: «Сейчас», then confirm — typing depends on the
  // profile's date format.
  await page.locator('.when-at input').click()
  const panel = page.locator('.n-date-panel')
  await panel
    .locator('.n-date-panel-actions .n-button')
    .filter({ hasText: /Сейчас|Now/ })
    .click()
  // «Сейчас» may close the panel by itself; a confirm button, if still there.
  await panel
    .locator('.n-date-panel-actions .n-button--primary-type')
    .click({ timeout: 1500 })
    .catch(() => {})
  await expect(page.getByTestId('conference-repeat')).toBeEnabled()

  // Length: 45 minutes.
  await page.getByTestId('conference-duration').click()
  await page.locator('.n-base-select-option', { hasText: '45 мин' }).click()

  // Stage.
  await page.getByTestId('conference-stage').click()
  await page.getByTestId('conference-stage-menu').getByText(stage.title).click()
  await expect(page.getByTestId('conference-stage')).toContainText(stage.title)

  // Repeat: every two weeks.
  await page.getByTestId('conference-repeat').click()
  await page.getByTestId('conference-repeat-menu').getByText('Каждые 2 недели').click()
  await expect(page.getByTestId('conference-repeat')).toContainText('Каждые 2 недели')

  await page.getByTestId('conference-submit').click()
  await page.waitForURL(/\/conferences\/[0-9a-f-]{36}$/)
  const id = new URL(page.url()).pathname.split('/').pop()

  // The server has the plan.
  const { conference } = await backend.get(`/conferences/${id}`)
  expect(conference).toMatchObject({
    duration_minutes: 45,
    recurrence: 'biweekly',
    milestone_id: stage.id,
    milestone_title: stage.title,
  })
  expect(conference.timezone).toBeTruthy()

  // The lobby names the stage and links to its page.
  const lobbyStage = page.getByTestId('conference-lobby-stage')
  await expect(lobbyStage).toContainText(stage.title)
  await expect(lobbyStage).toHaveAttribute('href', /\/project\/.+\/stage\/.+/)

  // The list row: length under the time, the repeat and the stage pill.
  await page.goto('/conferences')
  const row = page.getByTestId('conference-row').filter({ hasText: title })
  await expect(row.getByTestId('conference-row-duration')).toHaveText('45 мин')
  await expect(row.getByTestId('conference-row-repeat')).toContainText('Каждые 2 недели')
  await expect(row.getByTestId('conference-row-stage')).toContainText(stage.title)

  // «Изменить» in the row's menu: the same form, filled in; drop the repeat.
  await row.getByTestId('conference-row-menu').click()
  await page.getByTestId('conference-edit').click()
  await expect(page.getByTestId('conference-title')).toHaveValue(title)
  await page.getByTestId('conference-repeat').click()
  await page.getByTestId('conference-repeat-menu').getByText('Не повторять').click()
  await page.getByTestId('conference-submit').click()
  await expect(row.getByTestId('conference-row-repeat')).toHaveCount(0)
  await expect(row.getByTestId('conference-row-stage')).toContainText(stage.title)
  const after = await backend.get(`/conferences/${id}`)
  expect(after.conference.recurrence).toBe('')
  expect(after.conference.duration_minutes).toBe(45)
})

test('чат звонка: «/задача» создаёт задачу, «@» зовёт человека', async ({
  page,
  backend,
  seed,
}) => {
  // A colleague to mention.
  const mate = newCredentials(seed.runId, '-chat-mate')
  const mateUser = await register(mate)
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: mate.email })

  const conf = await backend.post(`/workspaces/${seed.workspaceId}/conferences`, {
    title: `E2E чат-команды ${seed.runId}`,
  })
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto(`/conferences/${conf.id}`)
  await page.getByTestId('conference-join').click()
  await expect(page.getByTestId('conference-leave')).toBeVisible()
  await page.getByTestId('conference-rail-chat').click()
  const input = page.getByTestId('conference-chat-input').locator('textarea')

  // «/» offers the command; Enter puts it in the composer.
  await input.fill('/')
  await expect(page.getByTestId('conference-chat-suggest')).toBeVisible()
  await input.press('Enter')
  await expect(input).toHaveValue('/задача ')
  const title = `Прогноз ${seed.runId}`
  await input.pressSequentially(title)
  await expect(page.getByTestId('conference-chat-task-strip')).toBeVisible()
  // The board defaults to one of the workspace's (the seeded one is the only).
  await expect(page.getByTestId('conference-chat-task-board')).toContainText('Доска')
  await input.press('Enter')

  const line = page.getByTestId('conference-chat-task').filter({ hasText: title })
  await expect(line).toHaveCount(1)
  await expect(line.getByTestId('conference-chat-task-link')).toHaveAttribute(
    'href',
    /\/project\/.+\/board\/.+\?task=\d+/,
  )
  const tasks = await backend.get(`/boards/${seed.boardId}/tasks`)
  expect(tasks.some((t) => t.title === title)).toBe(true)

  // «@» offers the roster; the chip and the notification follow.
  await input.fill('@')
  await input.pressSequentially(mate.name.split(' ')[0].slice(0, 3))
  // Pick the mate's row by name: in a full run other e2e users share the seeded
  // workspace, so the first row is not necessarily theirs.
  const mateRow = page.getByTestId('conference-chat-suggest-row').filter({ hasText: mate.name })
  await expect(mateRow).toBeVisible()
  await mateRow.click()
  await input.pressSequentially('глянь прогноз')
  await input.press('Enter')
  const msg = page.getByTestId('conference-chat-message').filter({ hasText: 'глянь прогноз' })
  await expect(msg.getByTestId('conference-chat-mention')).toHaveText(`@${mate.name}`)

  const { token } = await login(mate.email, mate.password)
  await expect
    .poll(async () => {
      const rows = await api.get('/notifications', token)
      return rows.filter((n) => n.payload?.event === 'conference_mention').length
    })
    .toBe(1)
  expect(mateUser.user.id).toBeTruthy()

  await page.getByTestId('conference-leave').click()
})
