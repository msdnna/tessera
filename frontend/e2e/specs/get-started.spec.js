import { test, expect, signIn, openBoard, column, cardsIn, dragCard, dragRow } from '../fixtures.js'
import { api, newCredentials, register, seedBoard, createTask } from '../api.js'
import { t } from '../i18n.js'

// The Get Started guide (#2753) as a first-time user actually meets it: nobody
// opens it, it opens itself on the first load of a freshly registered account.
//
// This spec is the one place in the suite where that happens — api.js register()
// opts every other user out, because the guide's mask and popover would land on
// top of whatever that spec is about to click.
//
// Scope is the entry/exit machinery (#2760, v2 #3000): autostart, «Понятно»
// advancing, the step number in its chapter, «Прервать?» with «Завершить
// обучение» (for good: ack written → not back after a reload) and «Продолжить
// позже» (back from the checklist on «Мои задачи»), «Обучение» in the footer,
// and the checklist counting what the user did without the tour. Walking every
// chapter is deliberately out: the steps wait on real projects, boards and
// tasks being created, which the board specs already cover.

test.use({ signedIn: false })

// A fresh account per test: the guide's autostart is once-per-account by design,
// so a shared user would only ever see it in whichever test ran first.
async function freshUser(suffix = '-tour') {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const creds = newCredentials(id, suffix)
  const { token } = await register(creds, { guide: true }) // the users that keep it
  return { creds, token, id }
}

test('guide autostarts for a new account and advances on «Понятно»', async ({ page }) => {
  await signIn(page, (await freshUser()).creds)
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await expect(pop).toContainText(t('tour.steps.workspaces.title'))
  // v2: the chapter and the step's number in it.
  await expect(pop).toContainText(t('tour.chapters.basics.title'))
  await expect(pop).toContainText('1')

  // The first step is informational: «Понятно» moves on (↵ does too).
  await expect(page.getByTestId('tour-next')).toBeVisible()
  await page.getByTestId('tour-next').click()
  await expect(pop).toContainText(t('tour.steps.tree-add.title'))

  // Step 2 asks the user to act: no «Понятно», a plate saying what counts it,
  // and only «Прервать» — no way to skip past an action step.
  await expect(page.getByTestId('tour-next')).toHaveCount(0)
  await expect(page.getByTestId('tour-wait')).toBeVisible()
  await expect(page.getByTestId('tour-skip')).toBeVisible()

  // Opening the project form is what the step waits for.
  await page.locator('[data-tour="proj-add"]').click()
  await page.locator('[data-tour="menu-project"]').click()
  await expect(pop).toContainText(t('tour.steps.project-create.title'), { timeout: 10000 })
})

test('«Прервать» → «Завершить обучение» ends the guide for good', async ({ page }) => {
  await signIn(page, (await freshUser()).creds)
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  // Esc asks in place of the step…
  await page.keyboard.press('Escape')
  await expect(pop).toHaveAttribute('data-kind', 'confirm')
  // …a second Esc goes back to the step…
  await page.keyboard.press('Escape')
  await expect(pop).toHaveAttribute('data-kind', 'info')
  // …and «Завершить обучение» ends it.
  await page.getByTestId('tour-skip').click()
  await page.getByTestId('tour-finish').click()
  await expect(pop).toHaveCount(0)

  // The ack is server-side, so it survives a reload.
  await page.reload()
  await expect(page.locator('.sb-footer')).toBeVisible() // app is up again
  await expect(pop).toHaveCount(0)
})

test('«Продолжить позже» keeps the step; the checklist brings it back', async ({ page }) => {
  await signIn(page, (await freshUser()).creds)
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-next').click()
  await expect(pop).toContainText(t('tour.steps.tree-add.title'))
  await page.getByTestId('tour-skip').click()
  await page.getByTestId('tour-later').click()
  await expect(pop).toHaveCount(0)

  // The checklist on «Мои задачи»: «Основы», nothing done yet, «Продолжить».
  const list = page.getByTestId('tour-checklist')
  await expect(list).toBeVisible()
  await expect(list.locator('[data-item="basics.project"]')).toBeVisible()
  await list.locator('.tc-row.cur').click()
  await expect(pop).toBeVisible()
  await expect(pop).toContainText(t('tour.steps.tree-add.title'))
})

test('«Обучение» in the sidebar footer restarts the guide', async ({ page }) => {
  await signIn(page, (await freshUser()).creds)
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-skip').click()
  await page.getByTestId('tour-finish').click()
  await expect(pop).toHaveCount(0)

  // «Обучение» lives in the footer's «Помощь» menu next to the help centre
  // (#2792), so restarting the guide is two clicks: open the menu, pick the item.
  await page.locator('[data-tour="footer-help"]').click()
  await page.locator('[data-help-menu="tour"]').click()
  await expect(pop).toBeVisible()
  await expect(pop).toContainText(t('tour.steps.workspaces.title'))
})

// The checklist counts a result done without the tour (#3102): the server
// tags the user's own socket events with their id, so a project created behind
// the page's back (here: straight through the API, as ⌘K or another tab would)
// ticks «Создать проект» with «сделано без тура».
test('the checklist ticks a project created without the tour', async ({ page }) => {
  const { creds, token, id } = await freshUser('-tour-self')
  const ws = await api.post('/workspaces', { name: `E2E ${id}-self` }, token)
  await signIn(page, creds)
  await page.evaluate((w) => localStorage.setItem('tessera_ws', w), ws.id)
  await page.goto('/')
  // The guide is in the way of nothing here, but leave it for the checklist.
  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-skip').click()
  await page.getByTestId('tour-later').click()

  const row = page.locator('[data-testid="tour-checklist"] [data-item="basics.project"]')
  await expect(row).toBeVisible()
  await expect(row).not.toHaveAttribute('data-done', /.+/)
  await page.waitForLoadState('networkidle')
  await api.post(`/workspaces/${ws.id}/projects`, { name: 'Сам по себе' }, token)
  await expect(row).toHaveAttribute('data-done', 'self', { timeout: 10000 })
  await expect(row).toContainText(t('tour.checklist.self'))
})

// ── DnD-шаги (#2778) ───────────────────────────────────────────────────────
//
// Живой браузер — единственное место, где эти шаги проверяются по-настоящему:
// SortableJS слушает pointer-события, а jsdom их не двигает, так что юнит-тесты
// покрывают правило продвижения, а здесь — что оно срабатывает от настоящего
// перетаскивания.
//
// Тур поднимается сразу на нужном шаге: `tessera_tour_step` — тот же ключ, по
// которому autoStart() возобновляет главу после F5 (stores/tour.js), в форме
// `<глава>:<шаг>` (#3000).
const TOUR_STEP_KEY = 'tessera_tour_step'

test('шаг «Перетащите карточку» закрывается настоящим переносом между колонками', async ({
  page,
}) => {
  const { creds, token, id } = await freshUser('-tour-dnd')
  const { board, columns } = await seedBoard(token, `${id}-dnd`)
  await createTask(token, board.id, columns[0].id, 'Карточка обучения')

  await signIn(page, creds)
  await page.evaluate((k) => localStorage.setItem(k, 'basics:dnd-card'), TOUR_STEP_KEY)
  await openBoard(page, board.id)

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await expect(pop).toContainText(t('tour.steps.dnd-card.title'))

  // Имена колонок остаются русскими в любой локали: их сеет бэкенд
  // (`handlers/boards.go defaultColumns`), это данные доски, а не строки
  // интерфейса. Отсюда литералы здесь и ключи каталога выше.
  const card = cardsIn(page, 'К работе').first()
  await expect(card).toBeVisible()
  await dragCard(page, card, column(page, 'В процессе'))

  // Карточка переехала — шаг сказал «Засчитано» и уступил следующему.
  await expect(cardsIn(page, 'В процессе')).toHaveCount(1)
  await expect(pop).toContainText(t('tour.steps.card-open.title'))
})

test('глава «Проекты и группы»: созданная группа закрывает шаг', async ({ page }) => {
  const { creds, token, id } = await freshUser('-tour-group')
  const { board } = await seedBoard(token, `${id}-group`)

  await signIn(page, creds)
  await page.evaluate((k) => localStorage.setItem(k, 'projects:group-add'), TOUR_STEP_KEY)
  await openBoard(page, board.id)

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await expect(pop).toContainText(t('tour.steps.group-add.title'))

  await page.locator('[data-tour="proj-add"]').click()
  await page.locator('[data-tour="menu-group"]').click()
  // С #2969 группа называется прямо в дереве: пустая строка, имя, Enter.
  const name = `E2E группа ${id}`
  await page.getByTestId('new-group-row').locator('input').fill(name)
  await page.getByTestId('new-group-row').locator('input').press('Enter')
  await expect(pop).toContainText(t('tour.steps.dnd-project.title'))
})

test('перенос проекта в группу меняет адрес строки, который читает шаг dnd-project', async ({
  page,
}) => {
  // Шаг закрывается по смене адреса строки проекта — id группы, в которой она
  // лежит. Проверяем то, ради чего сюда и идём: после настоящего переноса
  // строка проекта оказывается внутри [data-tour-group] нужной группы.
  const { creds, token, id } = await freshUser('-tour-tree')
  const { ws, group, project, board } = await seedBoard(token, `${id}-tree`)
  const loose = await api.post(`/workspaces/${ws.id}/projects`, { name: 'Вне группы' }, token)

  await signIn(page, creds)
  // Через доску, а не через '/': так приложение переключается на то
  // пространство, где лежит засеянное дерево (BoardView делает это сам).
  await openBoard(page, board.id)
  // Гайд тут только мешает.
  await page.getByTestId('tour-skip').click()
  await page.getByTestId('tour-finish').click()

  const looseRow = page.locator(`[data-tour-project="${loose.id}"] [data-tour="project-row"]`)
  const insideRow = page.locator(`[data-tour-project="${project.id}"] [data-tour="project-row"]`)
  await expect(looseRow).toBeVisible()

  const groupOf = (locator) =>
    locator.evaluate(
      (el) => el.closest('[data-tour-group]')?.getAttribute('data-tour-group') ?? null,
    )

  // До переноса проект лежит в корне дерева: контейнера с адресом над ним нет.
  expect(await groupOf(looseRow)).toBe(null)

  await dragRow(page, looseRow, insideRow)

  await expect.poll(() => groupOf(looseRow), { timeout: 10000 }).toBe(group.id)
})
