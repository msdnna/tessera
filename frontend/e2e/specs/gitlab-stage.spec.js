import { test, expect, openBoard } from '../fixtures.js'
import { t } from '../i18n.js'

// GitLab stages both ways (#3010). The stand has no GitLab, so the stage is a
// real native one whose GitLab side is played by page.route: the project list
// marks it linked (gl_* fields), and the answers of the write (gl_sync), the
// retry, the conflicts list and the resolve are what the backend would send
// after talking to GitLab. The writes themselves still reach the real backend —
// the three-way logic behind them is covered by backend/gitlab_milestone_writeback_test.go.
const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

// A list read may still be in flight when a test ends; its route must not fail it.
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' })
})

async function gitlabStage(page, backend, label, { writable = true } = {}) {
  const { ws, board, project } = await backend.freshBoard(label)
  const stage = await backend.post(`/projects/${project.id}/milestones`, {
    title: 'Спринт GL',
    description: 'Цели из GitLab',
  })
  const gl = {
    gl_global_id: 'gid://gitlab/Milestone/41',
    gl_url: 'https://gitlab.example/grp/app/-/milestones/4',
    gl_iid: 4,
    gl_writable: writable,
    gl_push_error: '',
    gl_push_fields: [],
    gl_conflict: false,
  }
  const ctl = { gl, nextSync: 'ok', patches: [], retries: 0, resolves: [] }

  // The project's stages: ours, marked as GitLab's.
  await page.route(
    (url) => url.pathname === `/api/projects/${project.id}/milestones`,
    async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      const res = await route.fetch()
      const list = (await res.json()).map((m) => (m.id === stage.id ? { ...m, ...ctl.gl } : m))
      await route.fulfill({ response: res, json: list })
    },
  )
  // The write: the real PATCH, answered with what GitLab made of it.
  await page.route(
    (url) => url.pathname === `/api/milestones/${stage.id}`,
    async (route) => {
      if (route.request().method() !== 'PATCH') return route.continue()
      ctl.patches.push(route.request().postDataJSON())
      const res = await route.fetch()
      const body = await res.json()
      const status = ctl.nextSync
      if (status === 'failed') {
        ctl.gl.gl_push_error = 'gitlab: http 500: milestone write refused'
        ctl.gl.gl_push_fields = ['description']
      } else if (status === 'conflict') {
        ctl.gl.gl_conflict = true
      }
      const sync =
        status === 'failed'
          ? { status, error: ctl.gl.gl_push_error, fields: ['description'] }
          : status === 'conflict'
            ? { status, fields: ['description'] }
            : { status }
      await route.fulfill({ response: res, json: { ...body, gl_sync: sync } })
    },
  )
  // «повторить».
  await page.route(
    (url) => url.pathname === `/api/milestones/${stage.id}/gitlab/sync`,
    async (route) => {
      ctl.retries++
      ctl.gl.gl_push_error = ''
      ctl.gl.gl_push_fields = []
      await route.fulfill({ json: { ...stage, ...ctl.gl, gl_sync: { status: 'ok' } } })
    },
  )
  // The conflicts window: the stage's conflict while it is open.
  await page.route(
    (url) => url.pathname === `/api/workspaces/${ws.id}/gitlab/conflicts`,
    async (route) => {
      const list = ctl.gl.gl_conflict
        ? [
            {
              id: stage.id,
              entity: 'milestone',
              milestone_id: stage.id,
              milestone_title: 'Спринт GL',
              milestone_slug: stage.slug,
              project_slug: project.slug,
              task_id: null,
              change_kind: 'milestone',
              gl_iid: 4,
              detected_at: new Date().toISOString(),
              fields: [
                {
                  field: 'description',
                  base: 'Цели из GitLab',
                  ours: 'Цели Tessera',
                  theirs: 'Цели, поправленные в GitLab',
                },
              ],
            },
          ]
        : []
      await route.fulfill({ json: list })
    },
  )
  await page.route(
    (url) => url.pathname === `/api/milestones/${stage.id}/gitlab/conflict/resolve`,
    async (route) => {
      ctl.resolves.push(route.request().postDataJSON())
      ctl.gl.gl_conflict = false
      await route.fulfill({ json: { status: 'ok', resolution: 'ours', gl_sync: { status: 'ok' } } })
    },
  )

  // The board, filtered to the stage — its strip leads to the stage page.
  await openBoard(page, board.id)
  await page.goto(`/project/${project.slug}/board/${board.slug}?milestone=${stage.slug}`)
  await expect(page.getByTestId('stage-strip')).toBeVisible()
  return { ws, board, project, stage, ctl }
}

async function editGoals(page, text) {
  // The page is live once the stage has loaded (its controls follow gl_writable).
  await expect(page.getByTestId('stage-goals-edit')).toBeEnabled()
  await page.keyboard.press('e')
  const editor = page.getByTestId('stage-goals').locator('textarea')
  await expect(editor).toBeFocused()
  await editor.fill(text)
  await page.keyboard.press(`${MOD}+Enter`)
}

test('GitLab-этап: цели правятся и уходят в GitLab, ⇧X закрывает этап', async ({
  page,
  backend,
}) => {
  const { project, stage, ctl } = await gitlabStage(page, backend, 'gl-stage-edit')
  await page.getByTestId('stage-strip').locator('.sstrip-name').click()
  await expect(page.getByTestId('stage-page')).toBeVisible()
  await expect(page.getByTestId('stage-goals-edit')).toBeEnabled()

  await editGoals(page, '1. Синк этапов в обе стороны')
  await expect(page.getByTestId('stage-goals')).toContainText('Синк этапов в обе стороны')
  expect(ctl.patches.at(-1).description).toBe('1. Синк этапов в обе стороны')
  await expect(page.getByTestId('stage-unsynced')).toHaveCount(0)

  // ⇧X arms, the second ⇧X closes — the state goes to GitLab with the same write.
  await page.mouse.click(5, 5)
  await page.keyboard.press('Shift+X')
  await expect(page.getByTestId('stage-close')).toHaveClass(/armed/)
  await page.keyboard.press('Shift+X')
  await expect(page.getByTestId('stage-state')).toHaveClass(/closed/)
  expect(ctl.patches.at(-1).state).toBe('closed')
  await expect
    .poll(async () => {
      const list = await backend.get(`/projects/${project.id}/milestones`)
      return list.find((m) => m.id === stage.id)?.state
    })
    .toBe('closed')
})

test('GitLab-этап: отказ GitLab — «не синхронизировано · повторить»', async ({ page, backend }) => {
  const { board, project, stage, ctl } = await gitlabStage(page, backend, 'gl-stage-retry')
  ctl.nextSync = 'failed'
  await page.getByTestId('stage-strip').locator('.sstrip-name').click()
  await editGoals(page, 'Цели, которые GitLab не принял')
  await expect(page.getByTestId('stage-unsynced')).toBeVisible()

  // The strip says it in place of the goal line, and retries on a click.
  await page.goto(`/project/${project.slug}/board/${board.slug}?milestone=${stage.slug}`)
  const strip = page.getByTestId('stage-strip')
  await expect(strip).toContainText(t('milestones.gitlab.unsyncedGoals'))
  await strip.getByTestId('stage-strip-retry').click()
  await expect.poll(() => ctl.retries).toBe(1)
  await expect(strip.getByTestId('stage-strip-retry')).toHaveCount(0)
  await expect(strip).toContainText('Цели, которые GitLab не принял')
})

test('GitLab-этап: описание поменяли в обоих местах — окно конфликтов', async ({
  page,
  backend,
}) => {
  const { ctl } = await gitlabStage(page, backend, 'gl-stage-conflict')
  ctl.nextSync = 'conflict'
  await page.getByTestId('stage-strip').locator('.sstrip-name').click()
  await editGoals(page, 'Цели Tessera')

  const win = page.getByTestId('conflicts-window')
  await expect(win).toBeVisible()
  await expect(win.getByTestId('conflict-item')).toContainText('Спринт GL')
  await win.getByTestId('conflict-pick-ours').click()
  await win.getByTestId('conflict-apply').click()
  await expect.poll(() => ctl.resolves.length).toBe(1)
  expect(ctl.resolves[0]).toEqual({ resolution: 'ours' })
  await expect(win).toHaveCount(0)
})

test('GitLab-этап без записи в GitLab остаётся только для чтения', async ({ page, backend }) => {
  const { ctl } = await gitlabStage(page, backend, 'gl-stage-ro', { writable: false })
  await page.getByTestId('stage-strip').locator('.sstrip-name').click()
  await expect(page.getByTestId('stage-page')).toBeVisible()
  await expect(page.getByTestId('stage-goals-edit')).toBeDisabled()
  await expect(page.getByTestId('stage-close')).toBeDisabled()
  await page.keyboard.press('e')
  await expect(page.getByTestId('stage-goals').locator('textarea')).toHaveCount(0)
  expect(ctl.patches).toHaveLength(0)
})
