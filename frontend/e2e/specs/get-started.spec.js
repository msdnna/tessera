import { test, expect, signIn, openBoard, column, cardsIn, dragCard, dragRow } from '../fixtures.js'
import { api, newCredentials, register, seedBoard, createTask } from '../api.js'

// The Get Started guide (#2753) as a first-time user actually meets it: nobody
// opens it, it opens itself on the first load of a freshly registered account.
//
// This spec is the one place in the suite where that happens — api.js register()
// opts every other user out, because the guide's mask and popover would land on
// top of whatever that spec is about to click.
//
// Scope is the entry/exit machinery of #2760: autostart, «Понятно» advancing,
// «Пропустить» ending it for good (ack written → not back after a reload) and
// «Обучение» in the footer bringing it back on demand. Walking the whole
// twelve-point scenario is deliberately out: those steps wait on real projects,
// boards and tasks being created, which the board specs already cover.

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
  await expect(pop).toContainText('Пространства')

  // The first step is informational: both buttons, and «Понятно» moves on.
  await expect(page.getByTestId('tour-next')).toBeVisible()
  await page.getByTestId('tour-next').click()
  await expect(pop).toContainText('Дерево проектов')

  // Step 2 asks the user to act, so «Понятно» is gone and only «Пропустить» is
  // left — the author's call: no way to skip past an action step.
  await expect(page.getByTestId('tour-next')).toHaveCount(0)
  await expect(page.getByTestId('tour-skip')).toBeVisible()
})

test('«Пропустить» ends the guide for good', async ({ page }) => {
  await signIn(page, (await freshUser()).creds)
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-skip').click()
  await expect(pop).toHaveCount(0)

  // The ack is server-side, so it survives a reload — this is the check that the
  // e2e suite's own opt-out works, not just an in-memory flag.
  await page.reload()
  await expect(page.locator('.sb-footer')).toBeVisible() // app is up again
  await expect(pop).toHaveCount(0)
})

test('«Обучение» in the sidebar footer restarts the guide', async ({ page }) => {
  await signIn(page, (await freshUser()).creds)
  await page.goto('/')

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await page.getByTestId('tour-skip').click()
  await expect(pop).toHaveCount(0)

  await page.locator('[data-tour="footer-tour"]').click()
  await expect(pop).toBeVisible()
  await expect(pop).toContainText('Пространства')
})

// ── DnD-шаги (#2778) ───────────────────────────────────────────────────────
//
// Живой браузер — единственное место, где эти шаги проверяются по-настоящему:
// SortableJS слушает pointer-события, а jsdom их не двигает, так что юнит-тесты
// покрывают правило продвижения, а здесь — что оно срабатывает от настоящего
// перетаскивания.
//
// Тур поднимается сразу на нужном шаге: `tessera_tour_step` — тот же ключ, по
// которому autoStart() возобновляет гайд после F5 (stores/tour.js).
const TOUR_STEP_KEY = 'tessera_tour_step'

test('шаг «Перетащите карточку» закрывается настоящим переносом между колонками', async ({
  page,
}) => {
  const { creds, token, id } = await freshUser('-tour-dnd')
  const { board, columns } = await seedBoard(token, `${id}-dnd`)
  await createTask(token, board.id, columns[0].id, 'Карточка обучения')

  await signIn(page, creds)
  await page.evaluate((k) => localStorage.setItem(k, 'dnd-card'), TOUR_STEP_KEY)
  await openBoard(page, board.id)

  const pop = page.getByTestId('tour-pop')
  await expect(pop).toBeVisible({ timeout: 15000 })
  await expect(pop).toContainText('Перетащите карточку')

  const card = cardsIn(page, 'К работе').first()
  await expect(card).toBeVisible()
  await dragCard(page, card, column(page, 'В процессе'))

  // Карточка переехала — и вместе с ней уехал шаг.
  await expect(cardsIn(page, 'В процессе')).toHaveCount(1)
  await expect(pop).toContainText('Проекты тоже группируются')
})

test('перенос проекта в группу меняет адрес строки, который читает шаг dnd-project', async ({
  page,
}) => {
  // Сам шаг привязан к проекту, созданному внутри тура (`{project}` → ctx), а
  // ctx живёт только в памяти и после reload пуст — поднять тур сразу на нём
  // нельзя. Проверяем то, ради чего сюда и идём: после настоящего переноса
  // строка проекта оказывается внутри [data-tour-group] нужной группы — ровно
  // тот адрес, по смене которого шаг и закрывается.
  const { creds, token, id } = await freshUser('-tour-tree')
  const { ws, group, project, board } = await seedBoard(token, `${id}-tree`)
  const loose = await api.post(`/workspaces/${ws.id}/projects`, { name: 'Вне группы' }, token)

  await signIn(page, creds)
  // Через доску, а не через '/': так приложение переключается на то
  // пространство, где лежит засеянное дерево (BoardView делает это сам).
  await openBoard(page, board.id)
  await page.getByTestId('tour-skip').click() // гайд тут только мешает

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
