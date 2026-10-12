import { test, expect } from '../fixtures.js'
import { api } from '../api.js'

// «Отложить» on «Мои задачи» (#3108): a swipe left on a row offers «Отложить»,
// which asks how far and moves the task's due — a date-only due stays date-only.

async function swipeLeft(page, selector) {
  await page.evaluate((sel) => {
    const row = document.querySelector(sel).closest('.sw')
    const r = row.getBoundingClientRect()
    const y = r.y + r.height / 2
    const fire = (type, x) =>
      row.dispatchEvent(
        new PointerEvent(type, {
          pointerType: 'touch',
          pointerId: 1,
          clientX: x,
          clientY: y,
          bubbles: true,
        }),
      )
    fire('pointerdown', r.right - 10)
    for (const dx of [20, 60, 120, 180]) fire('pointermove', r.right - 10 - dx)
    fire('pointerup', r.right - 190)
  }, selector)
}

test('«Отложить» свайпом переносит срок задачи', async ({ page, seed, backend }) => {
  const title = `Отложить ${seed.runId}`
  // Overdue by five days: «Завтра» moves it to tomorrow — of the profile's
  // calendar, which need not be the browser's, so the check below is zone-free.
  const now = new Date()
  const due = new Date(
    Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - 5),
  ).toISOString()
  const task = await backend.createTask(seed.columns[0].id, title, { due_date: due })
  await api.post(`/tasks/${task.id}/assignees`, { user_id: seed.userId }, seed.token)

  // «Мои задачи» list the current workspace — the run's one.
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/')
  const row = `.hl-row[data-task-id="${task.id}"]`
  await expect(page.locator(row)).toBeVisible()
  await swipeLeft(page, row)
  await page.getByTestId('swipe-snooze').click()
  await expect(page.getByTestId('home-snooze')).toBeVisible()
  await page.getByTestId('home-snooze-tomorrow').click()

  await expect
    .poll(async () => (await api.get(`/tasks/${task.id}`, seed.token)).due_date)
    .not.toBe(due)
  const got = (await api.get(`/tasks/${task.id}`, seed.token)).due_date
  const d = new Date(got)
  // Tomorrow, still a date without a clock (UTC midnight).
  expect(d.getUTCHours() + d.getUTCMinutes()).toBe(0)
  // Tomorrow in some zone: later than the old due, within the next two days.
  expect(d.getTime()).toBeGreaterThan(Date.now() - 864e5)
  expect(d.getTime()).toBeLessThan(Date.now() + 2 * 864e5)
})
