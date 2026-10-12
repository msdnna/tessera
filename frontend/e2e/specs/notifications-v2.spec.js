import { test, expect, signIn } from '../fixtures.js'
import { api, newCredentials, register, seedBoard } from '../api.js'

// The v2 notifications (#3033), on their own pair of users so the shared seed
// user's bell stays as the other specs expect it:
//   - a meeting invitation answered right in the bell's row (#3051), and on the
//     meetings page;
//   - a mention in a document's comment and a closed stage (#3048);
//   - the page of all notifications with server filters and paging (#3049);
//   - «Только срочное и упоминания» in the quiet hours (#3050).

// Each test signs its own reader in; the fixture's seed session would race it
// (two logins, two refresh cookies — the bell then reads the seed user's feed).
test.use({ signedIn: false })

async function pair(label) {
  const runId = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const meCreds = newCredentials(runId, `-${label}-me`)
  const mateCreds = newCredentials(runId, `-${label}-mate`)
  const me = await register(meCreds)
  const mate = await register(mateCreds)
  const s = await seedBoard(me.token, `${runId}-${label}`)
  await api.post(`/workspaces/${s.ws.id}/members`, { email: mateCreds.email }, me.token)
  return { me, mate, meCreds, s }
}

test('уведомления v2: RSVP в ленте, упоминание в документе, этап, страница всех уведомлений', async ({
  page,
}) => {
  const { me, mate, meCreds, s } = await pair('feed')

  // A closed stage with the reader's task in it.
  const ms = await api.post(
    `/projects/${s.project.id}/milestones`,
    { title: 'Спринт 41' },
    me.token,
  )
  const task = await api.post(
    `/boards/${s.board.id}/tasks`,
    { title: 'Задача этапа', column_id: s.columns[0].id },
    me.token,
  )
  await api.post(`/tasks/${task.id}/milestone`, { milestone_id: ms.id }, me.token)
  await api.post(`/tasks/${task.id}/assignees`, { user_id: me.user.id }, me.token)
  await api.patch(`/milestones/${ms.id}`, { title: 'Спринт 41', state: 'closed' }, mate.token)

  // Thirty-two assignments: more than one page of the page of all notifications.
  for (let i = 0; i < 32; i++) {
    const t = await api.post(
      `/boards/${s.board.id}/tasks`,
      { title: `Назначение ${i}`, column_id: s.columns[0].id },
      mate.token,
    )
    await api.post(`/tasks/${t.id}/assignees`, { user_id: me.user.id }, mate.token)
  }

  // A mention in a document's comment.
  const doc = await api.post(
    `/workspaces/${s.ws.id}/documents`,
    { title: 'План релиза' },
    mate.token,
  )
  await api.post(
    `/documents/${doc.id}/comments`,
    { body: `@${meCreds.name} глянь раздел`, mentions: [me.user.id] },
    mate.token,
  )

  // The invitation, newest of all.
  const conf = await api.post(
    `/workspaces/${s.ws.id}/conferences`,
    {
      title: 'Синк по синхронизации',
      scheduled_at: new Date(Date.now() + 2 * 3600e3).toISOString(),
    },
    mate.token,
  )
  await api.post(`/conferences/${conf.id}/invite`, { user_ids: [me.user.id] }, mate.token)

  await signIn(page, meCreds)
  await page.goto('/')
  await expect(page.getByTestId('sidebar')).toBeVisible()
  // The bell's count says the feed has loaded.
  await expect(page.getByTestId('notifications-unread')).toBeVisible()
  await page.getByTestId('notifications-bell').click()
  const feed = page.getByTestId('notifications-feed')
  await expect(feed).toBeVisible()

  // «Принять» in the row: the answer stays drawn.
  const rsvp = feed.getByTestId('notification-rsvp').first()
  await expect(rsvp).toHaveAttribute('data-rsvp', 'pending')
  await rsvp.getByTestId('notification-accept').click()
  await expect(rsvp).toHaveAttribute('data-rsvp', 'accepted')
  // The row answers before the server does; the server catches up.
  await expect
    .poll(async () => {
      const list = await api.get(`/workspaces/${s.ws.id}/conferences`, me.token)
      return list.find((c) => c.id === conf.id).my_rsvp
    })
    .toBe('accepted')

  // «Все уведомления →» leads to the page.
  await page.getByTestId('notifications-all').click()
  await expect(page).toHaveURL(/\/notifications$/)
  const all = page.getByTestId('notifications-page').getByTestId('notifications-feed')
  await expect(all).toHaveAttribute('data-all', '')
  // The popover stepped aside: only the page's feed is left.
  await expect(page.getByTestId('notifications-feed')).toHaveCount(1)
  const rows = all.getByTestId('notification-row')
  await expect(rows).toHaveCount(30)

  // The end of the list loads the next page.
  await all.getByTestId('notifications-more').scrollIntoViewIfNeeded()
  await expect(rows).toHaveCount(35)
  await expect(all.getByTestId('notifications-load-more')).toHaveCount(0)

  // The server's filter: only the document mention is a mention.
  await all.getByTestId('notifications-filter-mentions').click()
  await expect(rows).toHaveCount(1)
  await expect(rows.first()).toContainText('План релиза')

  // The closed stage opens its page.
  await all.getByTestId('notifications-filter-all').click()
  await expect(rows).toHaveCount(30)
  await all.getByTestId('notifications-more').scrollIntoViewIfNeeded()
  await expect(rows).toHaveCount(35)
  const stageRow = rows.filter({ hasText: 'Спринт 41' })
  await expect(stageRow).toHaveCount(1)
  await stageRow.click()
  await expect(page).toHaveURL(new RegExp(`/stage/${ms.id}`))
})

test('уведомления v2: ответ на приглашение на странице встреч', async ({ page }) => {
  const { me, mate, meCreds, s } = await pair('meet')
  const conf = await api.post(
    `/workspaces/${s.ws.id}/conferences`,
    { title: 'Планирование', scheduled_at: new Date(Date.now() + 26 * 3600e3).toISOString() },
    mate.token,
  )
  await api.post(`/conferences/${conf.id}/invite`, { user_ids: [me.user.id] }, mate.token)

  await signIn(page, meCreds)
  // The link opens the call's lobby in its own workspace: «Ваш ответ» there.
  await page.goto(`/conferences/${conf.id}`)
  const answer = page.getByTestId('conference-rsvp')
  await expect(answer).toHaveAttribute('data-rsvp', 'pending')
  await page.getByTestId('conference-rsvp-accept').click()
  await expect(answer).toHaveAttribute('data-rsvp', 'accepted')

  // The list: the answer is a word, and the row's menu changes it.
  await page.getByTestId('conference-back').click()
  const row = page.getByTestId('conference-row').filter({ hasText: 'Планирование' })
  await expect(row.getByTestId('conference-row-answer')).toBeVisible()
  await expect(row.getByTestId('conference-row-rsvp')).toHaveCount(0)
  await row.getByTestId('conference-row-menu').click()
  await page.getByTestId('conference-menu-decline').click()
  await expect
    .poll(async () => {
      const detail = await api.get(`/conferences/${conf.id}`, mate.token)
      return detail.participants.find((p) => p.user_id === me.user.id).rsvp
    })
    .toBe('declined')
})

test('уведомления v2: «Только срочное и упоминания» в тихих часах', async ({ page }) => {
  const { me, meCreds } = await pair('quiet')
  await signIn(page, meCreds)
  await page.goto('/settings?s=notifications')
  const quiet = page.getByTestId('notif-quiet')
  await expect(quiet).toBeVisible()
  await quiet.locator('.n-switch').first().click()
  const exceptions = page.getByTestId('notif-quiet-exceptions').locator('.n-checkbox')
  await expect(exceptions).not.toHaveClass(/n-checkbox--disabled/)
  await exceptions.click()
  await expect
    .poll(async () => (await api.get('/notification-prefs', me.token)).quiet_allow_urgent)
    .toBe(true)
})
