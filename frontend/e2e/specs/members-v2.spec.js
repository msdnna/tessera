import { test, expect, openBoard, signIn } from '../fixtures.js'
import { api, newCredentials, register } from '../api.js'

// Members v2 (#3063): groups, the roles table, handing the workspace over, and
// the access request from «Нет доступа» to «Впустить». Every test works on a
// fresh workspace of its own — handing over the shared seed one would change
// the role other specs rely on.

async function openMembers(page) {
  await page.evaluate(() => {
    const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
    pinia._s.get('wsDialogs').open('members')
  })
  await expect(page.getByTestId('members-window')).toBeVisible()
}

async function mate(seed, wsId, suffix) {
  const creds = newCredentials(seed.runId, suffix)
  const m = await register(creds)
  await api.post(`/workspaces/${wsId}/members`, { email: creds.email, role: 'member' }, seed.token)
  return { ...m, creds }
}

test('группы, роли и передача владения', async ({ page, seed, backend }) => {
  const fresh = await backend.freshBoard('members-v2')
  const wsId = fresh.ws.id
  const m = await mate(seed, wsId, '-members-mate')
  await openBoard(page, fresh.board.id)
  await openMembers(page)

  // The row says when the member was last here.
  const row = page.getByTestId('member-row').filter({ hasText: m.creds.email })
  await expect(row.getByTestId('member-activity')).not.toBeEmpty()

  // «Группы»: a name and ↵ makes one and opens its people; a click adds a member.
  await page.getByTestId('members-tab-groups').click()
  await page.getByTestId('member-group-new').fill('Фронтенд')
  await page.getByTestId('member-group-new').press('Enter')
  await expect(page.getByTestId('member-group')).toHaveCount(1)
  const people = page.getByTestId('member-group-people')
  await people.getByTestId('member-group-toggle').filter({ hasText: m.creds.name }).click()
  await expect
    .poll(
      async () => (await api.get(`/workspaces/${wsId}/member-groups`, seed.token))[0].member_ids,
    )
    .toEqual([m.user.id])

  // «Роли и права»: the table of the three roles.
  await page.getByTestId('members-tab-roles').click()
  await expect(page.getByTestId('roles-pane')).toBeVisible()

  // Hand the workspace over: the name has to be typed before the button wakes up.
  await page.getByTestId('members-tab-members').click()
  await expect(row.getByTestId('member-groups-cell')).toHaveText('Фронтенд')
  if (process.env.F3063_SHOT) await page.screenshot({ path: process.env.F3063_SHOT })
  await row.getByTestId('member-role').click()
  await page.getByTestId('member-transfer').click()
  const submit = page.getByTestId('transfer-owner').locator('.pf-primary')
  const name = page.getByTestId('transfer-owner-name')
  await expect(name).toBeVisible()
  await name.fill('не то')
  await expect(submit).toBeDisabled()
  await name.fill(fresh.ws.name)
  await name.press('Enter')
  await expect
    .poll(async () => {
      const list = await api.get(`/workspaces/${wsId}/members`, seed.token)
      return Object.fromEntries(list.map((x) => [x.user_id, x.role]))
    })
    .toEqual({ [m.user.id]: 'owner', [seed.userId]: 'admin' })
})

test('запрос доступа: «Нет доступа» → «Впустить»', async ({ page, seed, backend, browser }) => {
  const fresh = await backend.freshBoard('access')
  const wsId = fresh.ws.id
  const creds = newCredentials(seed.runId, '-asker')
  const asker = await register(creds)

  // The stranger opens the workspace's link and asks.
  const ctx = await browser.newContext()
  const other = await ctx.newPage()
  await signIn(other, creds)
  await other.goto(`/space/${wsId}`)
  await expect(other.getByTestId('no-access')).toBeVisible()
  // Nothing about the space is on screen.
  await expect(other.getByTestId('no-access')).not.toContainText(fresh.ws.name)
  await other.getByTestId('no-access-message').fill('Пустите посмотреть')
  await other.getByTestId('no-access-request').click()
  await expect(other.getByTestId('no-access-request')).toHaveCount(0)

  // The owner lets them in from the members window.
  await openBoard(page, fresh.board.id)
  await openMembers(page)
  const req = page.getByTestId('access-request-row').filter({ hasText: creds.email })
  await expect(req).toContainText('Пустите посмотреть')
  await req.getByTestId('access-approve').click()
  await expect(req).toHaveCount(0)
  await expect(page.getByTestId('member-row').filter({ hasText: creds.email })).toBeVisible()
  const members = await api.get(`/workspaces/${wsId}/members`, asker.token)
  expect(members.find((x) => x.user_id === asker.user.id)?.role).toBe('member')
  await ctx.close()
})
