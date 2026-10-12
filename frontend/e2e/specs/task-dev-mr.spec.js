import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// «Разработка» (#2964): the merge requests working on a task, their state pill,
// branches and pipeline, and the live repaint from the `task.code_links` socket
// event. There is no GitLab on the stand, so the task's GitLab half is stitched
// into the real API answer (page.route) and the socket event is injected into the
// real socket (page.routeWebSocket) — the rest is the real app against the real
// backend.

const GL = 'https://gl.example'

function mr(number, extra = {}) {
  return {
    provider: 'gitlab',
    kind: 'merge_request',
    number,
    repo: 'grp/app',
    url: `${GL}/grp/app/-/merge_requests/${number}`,
    title: `MR ${number}`,
    state: 'open',
    draft: false,
    source_branch: 'feat/board-v2',
    target_branch: 'main',
    ci_status: '',
    ci_url: '',
    via: 'closes',
    updated_at: '2026-10-02T10:00:00Z',
    ...extra,
  }
}

test('MR в «Разработке»: карточка по макету, живое обновление по сокету, строка в истории', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('dev-mr')
  const title = `Задача с MR ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
  })

  // The task's GitLab half: the issue it mirrors and two merge requests.
  await page.route(`**/api/tasks/${task.id}`, async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    const res = await route.fetch()
    const body = await res.json()
    body.gitlab = {
      iid: 12,
      web_url: `${GL}/grp/app/-/issues/12`,
      project_path: 'grp/app',
      is_group: false,
    }
    body.code_links = [
      mr(419, { title: 'Старый фикс', state: 'merged', updated_at: '2026-10-05T10:00:00Z' }),
      mr(418, {
        title: 'Редизайн доски v2',
        ci_status: 'success',
        ci_url: `${GL}/grp/app/-/pipelines/90`,
      }),
    ]
    await route.fulfill({ response: res, json: body })
  })
  await page.route(`**/api/tasks/${task.id}/events`, async (route) => {
    const res = await route.fetch()
    const list = await res.json()
    list.push({
      id: 'e2e-code-change',
      task_id: task.id,
      kind: 'code_change',
      actor_id: null,
      actor_name: '',
      data: { source: 'gitlab', number: 418, state: 'open', branch: 'feat/board-v2' },
      created_at: new Date().toISOString(),
    })
    await route.fulfill({ response: res, json: list })
  })
  // Pass-through sockets we can speak into. The layout and the board each open
  // their own; the event is the layout's to hear, so it goes to every one.
  const sockets = []
  await page.routeWebSocket(/\/api\/ws/, (ws) => {
    const server = ws.connectToServer()
    ws.onMessage((m) => server.send(m))
    server.onMessage((m) => ws.send(m))
    sockets.push(ws)
  })
  await page.context().route(`${GL}/**`, (r) => r.fulfill({ body: 'gitlab' }))

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()

  const cards = modal.getByTestId('task-mr')
  await expect(cards).toHaveCount(2)
  // Open first, then the merged one.
  const first = cards.nth(0)
  await expect(first).toHaveAttribute('data-mr', '418')
  await expect(first).toContainText('!418')
  await expect(first).toContainText('Редизайн доски v2')
  await expect(first.getByTestId('task-mr-state')).toHaveText('открыт')
  await expect(first).toContainText('feat/board-v2 → main')
  await expect(first.getByTestId('task-mr-ci')).toHaveText('пайплайн пройден')
  await expect(first.getByTestId('task-mr-ci')).toHaveAttribute(
    'href',
    `${GL}/grp/app/-/pipelines/90`,
  )
  await expect(cards.nth(1).getByTestId('task-mr-state')).toHaveText('смержен')
  await expect(cards.nth(1).getByTestId('task-mr-ci')).toHaveCount(0)
  // The issue card stays below the MRs.
  await expect(modal.locator('.dev-card')).toContainText('grp/app')

  // The card opens the MR in GitLab.
  const [popup] = await Promise.all([page.waitForEvent('popup'), first.click()])
  await expect.poll(() => popup.url()).toBe(`${GL}/grp/app/-/merge_requests/418`)
  await popup.close()

  // History: the provider's line, not a person's.
  const line = modal.getByTestId('task-event-code').locator('..')
  await expect(line).toContainText('GitLab')
  await expect(line).toContainText('открыт !418 в feat/board-v2')

  // GitLab moved on: !418 became a draft with a running pipeline. The socket
  // event repaints the open window — no request for the task.
  await expect.poll(() => sockets.length).toBeGreaterThan(0)
  const event = JSON.stringify({
    scope: ws.id,
    type: 'task.code_links',
    data: {
      task_id: task.id,
      code_links: [mr(418, { title: 'Редизайн доски v2', draft: true, ci_status: 'running' })],
    },
  })
  for (const s of sockets) s.send(event)
  await expect(cards).toHaveCount(1)
  await expect(cards.first().getByTestId('task-mr-state')).toHaveText('черновик')
  await expect(cards.first().getByTestId('task-mr-ci')).toHaveText('пайплайн идёт')
})
