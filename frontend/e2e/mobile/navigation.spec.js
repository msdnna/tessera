import { test, expect, openBoard } from '../fixtures.js'
import { expectFitsWidth } from './_layout.js'

// The phone's navigation model (#2976 — package G, model A): five tabs at the
// bottom (Мои задачи · Доска · Входящие · Поиск · Ещё), the project tree in a
// drawer opened from the board's name or a swipe in from the left edge, the
// notification feed and «Ещё» as pages the system Back leaves.

async function settle(page) {
  await expect(page.locator('.page-slot--mobile')).toBeVisible({ timeout: 15000 })
  await page.waitForLoadState('networkidle').catch(() => {})
  await page.waitForTimeout(300)
}
const tab = (page, key) => page.getByTestId(`mobile-tab-${key}`)

test('нижняя навигация: пять вкладок, активная — по экрану', async ({ page, seed }) => {
  await page.goto('/')
  await settle(page)
  const bar = page.getByTestId('mobile-tabbar')
  await expect(bar).toBeVisible()
  for (const k of ['home', 'board', 'inbox', 'search', 'more'])
    await expect(tab(page, k)).toBeVisible()
  await expect(tab(page, 'home')).toHaveAttribute('aria-current', 'page')

  // Targets: the whole tab is the target, 64px tall.
  const box = await tab(page, 'inbox').boundingBox()
  expect(box.height).toBeGreaterThanOrEqual(64)
  expect(box.width).toBeGreaterThanOrEqual(44)

  // The bar sits at the bottom of the screen, the page ends above it.
  const vh = page.viewportSize().height
  const barBox = await bar.boundingBox()
  expect(Math.round(barBox.y + barBox.height)).toBe(vh)
  const pane = await page.locator('.page-slot--mobile').boundingBox()
  expect(Math.round(pane.y + pane.height)).toBeLessThanOrEqual(Math.round(barBox.y) + 1)

  // The FAB stands over the bar, not on it.
  const fab = await page.getByTestId('quick-fab-button').boundingBox()
  expect(fab.y + fab.height).toBeLessThanOrEqual(barBox.y - 8)

  // «Доска» opens a board; the tab follows the screen.
  await openBoard(page, seed.boardId)
  await settle(page)
  await expect(tab(page, 'board')).toHaveAttribute('aria-current', 'page')
  await expectFitsWidth(page, 'доска с нижней навигацией')
})

test('нижняя навигация: «Входящие» и «Поиск» — поверх экрана, вкладка закрывает', async ({
  page,
}) => {
  await page.goto('/')
  await settle(page)

  await tab(page, 'inbox').tap()
  await expect(page.getByTestId('inbox-panel')).toBeVisible()
  await expect(tab(page, 'inbox')).toHaveAttribute('aria-current', 'page')
  // The page-like sheet leaves the bar live under it.
  await expect(page.getByTestId('mobile-tabbar')).toBeVisible()

  // Another tab takes over: the inbox closes, the palette fills the screen.
  await tab(page, 'search').tap()
  await expect(page.getByTestId('inbox-panel')).toHaveCount(0)
  const palette = page.getByTestId('command-palette')
  await expect(palette).toBeVisible()
  const pb = await palette.boundingBox()
  expect(pb.width).toBeGreaterThanOrEqual(page.viewportSize().width - 1)
  await page.getByTestId('palette-cancel').tap()
  await expect(palette).toHaveCount(0)
  // Nothing was navigated by the overlays: still on the start screen.
  await expect(page).toHaveURL(/\/$/)
})

test('«Ещё»: разделы, тема, уведомления страницей и «назад»', async ({ page }) => {
  await page.goto('/')
  await settle(page)
  await tab(page, 'more').tap()
  await expect(page).toHaveURL(/\/more$/)
  await expect(page.getByTestId('more-page')).toBeVisible()
  await expect(tab(page, 'more')).toHaveAttribute('aria-current', 'page')
  // Let the start screen finish leaving (its aurora overhangs the pane edges
  // while it fades out).
  await expect(page.getByTestId('home')).toHaveCount(0)
  for (const id of [
    'more-profile',
    'more-milestones',
    'more-documents',
    'more-conferences',
    'more-notifications',
    'logout',
  ])
    await expect(page.getByTestId(id)).toBeVisible()
  await expectFitsWidth(page, '«Ещё»')

  // The feed is a page of its own on a phone; its back returns here.
  await page.getByTestId('more-notifications').tap()
  await expect(page).toHaveURL(/\/notifications$/)
  await expect(page.getByTestId('notifications-feed')).toBeVisible()
  await expectFitsWidth(page, 'уведомления')
  await page.getByTestId('notifications-back').tap()
  await expect(page).toHaveURL(/\/more$/)

  // A screen opened from «Ещё» leads back to it from its header.
  await page.getByTestId('more-documents').tap()
  await expect(page).toHaveURL(/\/documents/)
  await page.getByTestId('mobile-back').tap()
  await expect(page).toHaveURL(/\/more$/)
})

test('дровер: по названию доски и свайпом от левого края', async ({ page, seed }) => {
  await openBoard(page, seed.boardId)
  await settle(page)

  await page.getByTestId('mobile-board-switch').tap()
  const drawer = page.locator('.n-drawer')
  await expect(drawer).toBeVisible()
  await expect(drawer.getByTestId('sidebar')).toBeVisible()
  // The tabs are not repeated in the tree: no «Входящие» / «Мои задачи» rows.
  await expect(drawer.getByTestId('nav-inbox')).toHaveCount(0)
  // System Back closes the drawer, not the board.
  await page.goBack()
  await expect(drawer).toHaveCount(0)
  await expect(page).toHaveURL(/\/board\//)

  // A swipe in from the very edge opens it as well.
  await page.evaluate(() => {
    const mk = (x) => new Touch({ identifier: 1, target: document.body, clientX: x, clientY: 400 })
    const fire = (type, x) =>
      window.dispatchEvent(
        new TouchEvent(type, {
          touches: type === 'touchend' ? [] : [mk(x)],
          changedTouches: [mk(x)],
          bubbles: true,
        }),
      )
    fire('touchstart', 4)
    fire('touchmove', 40)
    fire('touchmove', 90)
    fire('touchend', 90)
  })
  await expect(drawer).toBeVisible()
})

test('меню доски — лист снизу, вид меняется', async ({ page, seed }) => {
  await openBoard(page, seed.boardId)
  await settle(page)
  await page.getByTestId('board-mobile-menu').tap()
  const sheet = page.getByTestId('board-mobile-menu-list')
  await expect(sheet).toBeVisible()
  await page.waitForTimeout(350) // the slide-up
  const vh = await page.evaluate(() => window.innerHeight)
  const sb = await sheet.boundingBox()
  expect(sb.y).toBeGreaterThan(0)
  expect(Math.abs(Math.round(sb.y + sb.height) - vh)).toBeLessThanOrEqual(1)
  await page.getByTestId('board-mobile-menu-list-layout:list').tap()
  await expect(page.getByTestId('list-view')).toBeVisible()
  await expect(sheet).toHaveCount(0)
  // Back to the kanban for the specs after this one.
  await page.getByTestId('board-mobile-menu').tap()
  await page.getByTestId('board-mobile-menu-list-layout:board').tap()
  await expect(page.getByTestId('mobile-column-position')).toBeVisible()
})
