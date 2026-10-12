import { test, expect } from '../fixtures.js'
import { t } from '../i18n.js'

// v2 documents and templates (#3032): a document's stage (#3044), template
// categories (#3045), the agenda of a meeting (#3046) and board templates for an
// empty project (#3047). The Go tests own the endpoints; these walk the loops a
// person does — pick a stage in the head and find the document on the stage
// page, start an agenda from the gallery and find it on the meeting's row, open
// an empty project and start a board from a template.

async function inWorkspace(page, wsId) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), wsId)
}

test('документ: этап в шапке и документ на странице этапа', async ({ page, backend, seed }) => {
  const { ws, project } = await backend.freshBoard('doc-stage')
  const stage = await backend.post(`/projects/${project.id}/milestones`, { title: 'Спринт 42' })
  const doc = await backend.post(`/workspaces/${ws.id}/documents`, {
    title: 'Спецификация доски v2',
  })
  await inWorkspace(page, ws.id)
  await page.goto(`/documents/${doc.slug}`)
  await expect(page.locator('.ProseMirror')).toBeVisible()

  const chip = page.getByTestId('doc-milestone')
  await expect(chip).toContainText(t('documents.view.milestone'))
  await chip.click()
  const pinned = page.waitForResponse(
    (r) => /\/documents\/[^/]+\/milestone$/.test(r.url()) && r.request().method() === 'PUT',
  )
  await page.getByTestId(`doc-stage-menu-ms:${stage.id}`).click()
  expect((await pinned).status()).toBe(200)
  await expect(chip).toContainText('Спринт 42')

  // The pin survives a reload — it is the document's, not the screen's.
  await page.reload()
  await expect(page.getByTestId('doc-milestone')).toContainText('Спринт 42')

  // «Открыть страницу этапа» lands on the stage page, where the document is listed.
  await page.getByTestId('doc-milestone').click()
  await page.getByTestId('doc-stage-menu-open').click()
  await expect(page).toHaveURL(new RegExp(`/project/${project.slug}/stage/`))
  const card = page.getByTestId('stage-docs')
  await expect(card).toContainText('Спецификация доски v2')
  await card.getByTestId('stage-doc').first().click()
  await expect(page).toHaveURL(new RegExp(`/documents/${doc.slug}$`))
  expect(seed.runId).toBeTruthy()
})

test('шаблоны: категории и повестка, привязанная к встрече', async ({ page, backend }) => {
  const { ws } = await backend.freshBoard('doc-agenda')
  const when = new Date(Date.now() + 26 * 3600 * 1000).toISOString()
  const meeting = await backend.post(`/workspaces/${ws.id}/conferences`, {
    title: 'Планирование Спринта 43',
    scheduled_at: when,
  })
  await inWorkspace(page, ws.id)
  await page.goto('/documents')
  await page.getByTestId('doc-templates').click()

  // The column of the mockup: «Встречи» narrows the grid to the agenda.
  await page.getByTestId('tpl-source-meetings').click()
  const tiles = page.getByTestId('tpl-tile')
  await expect(tiles).toHaveCount(1)
  await expect(tiles.first()).toContainText(t('documents.templates.meeting.title'))
  await tiles.first().click()

  // The footer offers the meeting, the nearest one already picked.
  const pick = page.getByTestId('tpl-meeting')
  await expect(pick).toBeVisible()
  await expect(pick).toContainText('Планирование Спринта 43')

  const created = page.waitForResponse(
    (r) => /\/workspaces\/[^/]+\/documents$/.test(r.url()) && r.request().method() === 'POST',
  )
  await page.getByTestId('tpl-create').click()
  const body = (await created).request().postDataJSON()
  expect(body.conference_id).toBe(meeting.id)

  await expect(page.locator('.ProseMirror')).toBeVisible()
  await expect(page.getByTestId('doc-meeting')).toContainText('Планирование Спринта 43')

  // The meeting's row carries the agenda, and it opens the document.
  await page.goto('/conferences')
  const agenda = page.getByTestId('conference-agenda').first()
  await expect(agenda).toContainText('Планирование Спринта 43')
  await agenda.click()
  await expect(page).toHaveURL(/\/documents\/[^/]+$/)
  await expect(page.getByTestId('doc-meeting')).toBeVisible()
})

test('пустой проект: доска из шаблона «Баг-трекер» и пустая по ⇧N', async ({ page, backend }) => {
  const { ws, group } = await backend.freshBoard('empty-project')
  const project = await backend.post(`/workspaces/${ws.id}/projects`, {
    name: 'Tessera Mobile',
    group_id: group.id,
  })
  await inWorkspace(page, ws.id)
  await page.goto(`/project/${project.slug}`)

  const empty = page.getByTestId('project-empty')
  await expect(empty).toContainText(t('project.emptyProject.title', { name: 'Tessera Mobile' }))
  for (const key of ['sprint', 'bugs', 'roadmap'])
    await expect(page.getByTestId(`board-template-${key}`)).toBeVisible()

  await page.getByTestId('board-template-bugs').click()
  await expect(page).toHaveURL(new RegExp(`/project/${project.slug}/board/`))
  const boards = await backend.get(`/projects/${project.id}/boards`)
  expect(boards).toHaveLength(1)
  expect(boards[0].default_layout).toBe('list')
  const cols = await backend.get(`/boards/${boards[0].id}/columns`)
  expect(cols.map((c) => c.name_key)).toEqual([
    'new',
    'confirmed',
    'in_progress',
    'verify',
    'closed',
  ])

  // A project with a board goes straight to it.
  await page.goto(`/project/${project.slug}`)
  await expect(page).toHaveURL(new RegExp(`/project/${project.slug}/board/${boards[0].slug}`))

  // ⇧N on another empty project: a name field in place, ↵ creates a plain board.
  const other = await backend.post(`/workspaces/${ws.id}/projects`, {
    name: 'Пустой второй',
    group_id: group.id,
  })
  await page.goto(`/project/${other.slug}`)
  await expect(page.getByTestId('project-empty')).toBeVisible()
  await page.locator('body').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('Shift+N')
  const name = page.getByTestId('board-blank-name')
  await expect(name).toBeFocused()
  await name.fill('Своя доска')
  await name.press('Enter')
  await expect(page).toHaveURL(new RegExp(`/project/${other.slug}/board/`))
  const plain = await backend.get(`/projects/${other.id}/boards`)
  const plainCols = await backend.get(`/boards/${plain[0].id}/columns`)
  expect(plainCols.map((c) => c.name_key)).toEqual(['todo', 'in_progress', 'review', 'done'])
})
