import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// The regression #2721 guards: a board opened by direct link while a *different*
// workspace is remembered used to render fine and then silently stop updating —
// KanbanBoard drops every realtime event whose scope isn't the current
// workspace, and nothing in the resolve path switched it.
//
// Both workspaces here are freshly seeded, so "A is active" is a real state and
// not an artifact of whatever the shared seed left in localStorage.
test('доска другого workspace, открытая по прямой ссылке, получает realtime-события', async ({
  page,
  backend,
}) => {
  const a = await backend.freshBoard('deep-link-a')
  const b = await backend.freshBoard('deep-link-b')

  // Remember workspace A the way the sidebar switcher would, then deep-link into
  // a board that lives in B.
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), a.ws.id)
  await openBoard(page, b.board.id)

  const title = `Deep-link realtime ${Date.now().toString(36)}`
  await backend.post(`/boards/${b.board.id}/tasks`, { column_id: b.columns[0].id, title })

  // The assertion is the arriving card, not `localStorage.tessera_ws === B`: the
  // latter stays green even if the scope filter still compares the wrong id.
  // No reload — if it shows up, the event survived the scope filter.
  await expect(cardsIn(page, b.columns[0].name).filter({ hasText: title })).toBeVisible()
})

test('сайдбар переключается на workspace доски, открытой по прямой ссылке', async ({
  page,
  backend,
}) => {
  const a = await backend.freshBoard('deep-link-sidebar-a')
  const b = await backend.freshBoard('deep-link-sidebar-b')

  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), a.ws.id)
  await openBoard(page, b.board.id)

  // The one visible behaviour change of the fix: the app follows the link's
  // workspace instead of leaving the sidebar describing another one. Asserting on
  // B's project also covers the startup race — AppLayout selects the remembered
  // workspace concurrently, and its slower response must not overwrite B's tree.
  await expect(page.getByText(b.project.name)).toBeVisible()
  await expect(page.getByText(a.project.name)).toHaveCount(0)
})
