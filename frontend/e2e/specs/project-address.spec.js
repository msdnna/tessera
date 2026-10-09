import { test, expect, openBoard } from '../fixtures.js'
import { t, tRe } from '../i18n.js'

// Project addresses and the delete confirmations (#3007): a former address
// leads to the project for 30 days (#3012), a taken one names its holder and
// offers a free variant on ⌥↵ (#3013), and the confirmations say what goes with
// the project or the space (#3014).

// Addresses are unique across the instance and the backend outlives a run.
const uniq = (base) => `${base}-${Date.now().toString(36)}`

async function openSlugForm(page) {
  await page.getByTestId('project-menu').first().click()
  await page.getByTestId('project-actions-slug').click()
  await expect(page.getByTestId('project-slug')).toBeVisible()
  return page.getByTestId('project-slug-input')
}

test('смена адреса: старая ссылка на доску и на этап ведёт на новый адрес', async ({
  page,
  backend,
}) => {
  const { board, project } = await backend.freshBoard('addr-alias')
  const stage = await backend.post(`/projects/${project.id}/milestones`, { title: 'Этап E2E' })
  const oldSlug = project.slug
  const newSlug = uniq('novyy-adres')

  await openBoard(page, board.id)
  const input = await openSlugForm(page)
  // The form promises the old links keep working.
  await expect(page.getByTestId('project-slug')).toContainText(
    t('project.slug.oldLinks', { old: oldSlug }),
  )
  await input.fill(newSlug)
  await input.press('Enter')
  await expect(page.getByTestId('project-slug')).toHaveCount(0)
  await expect(page).toHaveURL(new RegExp(`/project/${newSlug}/board/${board.slug}`))

  // A link handed out before the change: same board, address swapped in place.
  await page.goto(`/project/${oldSlug}/board/${board.slug}`)
  await page.waitForURL(new RegExp(`/project/${newSlug}/board/${board.slug}`))
  await expect(page.getByTestId('column').first()).toBeVisible()

  // The stage page under the old address too.
  await page.goto(`/project/${oldSlug}/stage/${stage.slug}`)
  await page.waitForURL(new RegExp(`/project/${newSlug}/stage/${stage.slug}`))
  await expect(page.getByTestId('stage-page')).toBeVisible()
})

test('занятый адрес: «Адрес занят: его использует проект X», ⌥↵ подставляет свободный', async ({
  page,
  backend,
}) => {
  const { board, ws } = await backend.freshBoard('addr-taken')
  const taken = uniq('sayt')
  await backend.post(`/workspaces/${ws.id}/projects`, { name: 'Сайт', slug: taken })

  await openBoard(page, board.id)
  const input = await openSlugForm(page)
  await input.fill(taken)
  await input.press('Enter')

  const form = page.getByTestId('project-slug')
  await expect(form).toContainText(t('project.slug.takenBy', { name: 'Сайт' }))
  await expect(page.getByTestId('slug-suggest-chip')).toHaveText(`${taken}-2`)

  // ⌥↵ puts the free variant in; ↵ saves it.
  await input.press('Alt+Enter')
  await expect(input).toHaveValue(`${taken}-2`)
  await expect(page.getByTestId('slug-suggest')).toHaveCount(0)
  await input.press('Enter')
  await expect(form).toHaveCount(0)
  await expect(page).toHaveURL(new RegExp(`/project/${taken}-2/board/`))
})

test('новый проект на занятом адресе: «так уже называется проект в группе», ⌥↵ и создать', async ({
  page,
  backend,
}) => {
  const { board, ws, group } = await backend.freshBoard('addr-create')
  const taken = uniq('tessera-web')
  await backend.post(`/workspaces/${ws.id}/projects`, {
    name: 'Tessera Web',
    slug: taken,
    group_id: group.id,
  })

  await openBoard(page, board.id)
  await page.getByTestId('group-add-project').first().click()
  const form = page.getByTestId('project-create')
  await expect(form).toBeVisible()
  await page.getByTestId('project-create-name').fill('Ещё один')
  const slug = page.getByTestId('project-create-slug')
  await slug.fill(taken)
  await slug.press('Enter')
  await expect(form).toContainText(t('project.slug.takenInGroup', { group: group.name }))
  await slug.press('Alt+Enter')
  await expect(slug).toHaveValue(`${taken}-2`)
  await slug.press('Enter')
  await expect(form).toHaveCount(0)
  await expect
    .poll(async () => (await backend.get(`/workspaces/${ws.id}/projects`)).map((p) => p.slug))
    .toContain(`${taken}-2`)
})

test('окно «Проект» → «Общее»: занятый адрес с подсказкой, счётчики у удаления', async ({
  page,
  backend,
}) => {
  const { board, ws, columns } = await backend.freshBoard('addr-window')
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: 'Одна' })
  const taken = uniq('zanyato')
  await backend.post(`/workspaces/${ws.id}/projects`, { name: 'Чужой', slug: taken })

  await openBoard(page, board.id)
  await page.getByTestId('project-menu').first().click()
  await page.getByTestId('project-actions-settings').click()
  const tab = page.getByTestId('project-general')
  await expect(tab).toBeVisible()
  await expect(tab).toContainText(t('project.slug.aliasesHint'))
  await expect(page.getByTestId('project-delete-hint')).toHaveText(
    tRe('project.window.general.deleteHintCounts', { what: '1 \\S+ \\S+ 1 \\S+' }),
  )

  const field = page.getByTestId('project-slug-field')
  await field.fill(taken)
  await field.press('Enter')
  await expect(tab).toContainText(t('project.slug.takenBy', { name: 'Чужой' }))
  // The chip puts the free variant in without sending the taken one again.
  await page.getByTestId('slug-suggest-chip').click()
  await expect(field).toHaveValue(`${taken}-2`)
  await expect(field).toBeFocused()
  await field.press('Enter')
  await expect(page).toHaveURL(new RegExp(`/project/${taken}-2/board/`))
})

test('подтверждение удаления проекта и пространства показывает счётчики', async ({
  page,
  backend,
}) => {
  const { board, project, ws, columns } = await backend.freshBoard('addr-counts')
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: 'Раз' })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: 'Два' })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[1].id, title: 'Три' })
  await backend.post(`/projects/${project.id}/boards`, { name: 'Вторая' })

  await openBoard(page, board.id)

  // The project's delete arms first and says what goes with it.
  await page.getByTestId('project-menu').first().click()
  await page.getByTestId('project-actions-delete').click()
  const armed = page.getByTestId('project-actions-delete-armed')
  await expect(armed).toBeVisible()
  await expect(armed).toContainText(
    tRe('project.remove.armedSubCounts', { what: '2 \\S+ \\S+ 3 \\S+' }),
  )
  await page.keyboard.press('Escape')

  // The space's delete — the form that asks for the name.
  await page.getByTestId('ws-head').click()
  await page.getByTestId('ws-menu-delete').click()
  const form = page.getByTestId('ws-delete')
  await expect(form).toBeVisible()
  await expect(form).toContainText(
    tRe('shell.workspaceForm.deleteSubCounts', { what: '1 \\S+, 2 \\S+ \\S+ 3 \\S+' }),
  )
  await page.keyboard.press('Escape')
  await expect(form).toHaveCount(0)
  // Nothing was deleted along the way.
  expect((await backend.get(`/workspaces/${ws.id}/counts`)).tasks).toBe(3)
})
