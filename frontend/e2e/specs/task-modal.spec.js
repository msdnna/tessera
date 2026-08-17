import { test, expect, cardsIn, openBoard } from '../fixtures.js'

// The side panel slides in, so anything geometric read right after the switch lands
// mid-flight: `boundingBox()` reports a card that is still travelling, and a mouse
// aimed at "its centre" can miss the panel entirely (or fall outside the viewport)
// and the event goes to the board instead. Wait for the transform to be gone first.
async function panelSettled(modal) {
  await expect.poll(() => modal.evaluate((el) => getComputedStyle(el).transform)).toBe('none')
}

test('модалка задачи открывается, показывает описание и принимает комментарий', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('modal')
  const title = `Задача с описанием ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    description: '## Заголовок описания\n\nОбычный абзац.',
  })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  // The title is an editable field, so it holds the text as a value — `getByText`
  // would never see it.
  await expect(modal.locator('.title-input input')).toHaveValue(title)
  // The description is stored as Markdown and rendered — the heading must come
  // back as a real <h2>, not as literal "##".
  await expect(modal.locator('h2', { hasText: 'Заголовок описания' })).toBeVisible()

  const body = `Комментарий из e2e ${Date.now().toString(36)}`
  const composer = modal.locator('.comment-add textarea')
  await composer.fill(body)
  await composer.press('Control+Enter')

  await expect(modal.locator('.c-list')).toContainText(body)

  // ...and it really reached the server, not just the local list.
  await expect
    .poll(async () => {
      const comments = await backend.get(`/tasks/${task.id}/comments`)
      return comments.map((c) => c.body)
    })
    .toContain(body)
})

test('модалка закрывается по Escape', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('modal-esc')
  const title = `Закрой меня ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  await expect(page.getByTestId('task-modal')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(page.getByTestId('task-modal')).toHaveCount(0)
})

// #2716 — the side-panel layout. The point of the panel (as opposed to the modal)
// is that the board keeps working next to it, so the load-bearing assertion is not
// "the card moved right" but "clicking a board card behind it still works and the
// panel doesn't slam shut". That's what a mask would break.
test('задача открывается панелью справа, доска под ней остаётся живой', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('modal-layout')
  const stamp = Date.now().toString(36)
  const first = `Первая ${stamp}`
  const second = `Вторая ${stamp}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: first })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: second })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: first }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('task-layout-trigger').click()
  await page.getByTestId('task-layout-sidebar').click()

  // Pinned to the right edge of the viewport. Measured with a retrying poll, not a
  // bare boundingBox(): switching the mask off re-runs the modal's enter transition,
  // and a single read lands mid-scale on a card that hasn't settled yet.
  await expect(modal).toHaveClass(/tm-sidebar/)
  const width = page.viewportSize().width
  await expect
    .poll(async () => {
      const box = await modal.boundingBox()
      return Math.abs(box.x + box.width - width)
    })
    .toBeLessThan(2)

  // The board underneath is still interactive: this click opens the other task
  // rather than being eaten by a mask or dismissing the panel.
  await cardsIn(page, columns[0].name).filter({ hasText: second }).click()
  await expect(modal).toBeVisible()
  await expect(modal.locator('.title-input input')).toHaveValue(second)
  await expect(modal).toHaveClass(/tm-sidebar/)

  // The choice is device-level and survives a reload.
  await page.reload()
  await expect(page.getByTestId('column').first()).toBeVisible()
  await cardsIn(page, columns[0].name).filter({ hasText: first }).click()
  await expect(page.getByTestId('task-modal')).toHaveClass(/tm-sidebar/)
})

// #2716 /rework — the panel is a fixed-height box, so its body has to scroll on its
// own. It didn't: the rule was written against `n-card__content` while Naive names
// that element `n-card-content` (only its footer is BEM), so the whole task hung off
// the bottom of the viewport with nothing to grab. A wrong class name is a rule that
// fails silently, hence an assertion on the observable outcome — scrollTop moving.
test('панель задачи прокручивается внутри себя', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('modal-scroll')
  const title = `Длинная ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    description: Array.from({ length: 60 }, (_, i) => `Строка описания номер ${i}.`).join('\n\n'),
  })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('task-layout-trigger').click()
  await page.getByTestId('task-layout-sidebar').click()
  await expect(modal).toHaveClass(/tm-sidebar/)

  await panelSettled(modal)

  const body = modal.locator('.n-card-content')
  // The body is a real scroller: taller content than box, and it stays inside the
  // viewport instead of the card growing past it.
  await expect
    .poll(async () => {
      const m = await body.evaluate((el) => ({ sh: el.scrollHeight, ch: el.clientHeight }))
      return m.sh > m.ch && m.ch > 0
    })
    .toBe(true)
  const box = await modal.boundingBox()
  expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize().height + 1)

  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.wheel(0, 900)
  await expect.poll(() => body.evaluate((el) => el.scrollTop)).toBeGreaterThan(100)
})

// #2716 /rework — the comments composer is `position: sticky; bottom: 0`. Naive's
// card content carries a 20px padding-bottom, so the composer pinned to the
// content-box bottom left a padding strip below it through which the last comment
// bled ("текст комментариев просвечивает через форму"). The guard is geometric: the
// composer's bottom sits flush against the scroller's bottom, so no comment can show
// beneath it.
test('панель: композер комментариев прижат к низу, комментарии не просвечивают', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('modal-bleed')
  const stamp = Date.now().toString(36)
  const title = `С комментариями ${stamp}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
  })
  for (let i = 0; i < 20; i++) {
    await backend.post(`/tasks/${task.id}/comments`, {
      body: `Комментарий ${i} — длинная строка для переполнения панели ${stamp}`,
    })
  }

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('task-layout-trigger').click()
  await page.getByTestId('task-layout-sidebar').click()
  await expect(modal).toHaveClass(/tm-sidebar/)
  await panelSettled(modal)

  // Scroll to a mid position where a comment would land in any strip below the
  // pinned composer, then assert the composer is flush with the scroller bottom.
  await expect
    .poll(() =>
      modal.evaluate((root) => {
        const sc = root.querySelector('.n-card-content')
        sc.scrollTop = sc.scrollHeight - sc.clientHeight - 40
        const add = root.querySelector('.comment-add')
        if (!add) return -1
        const strip = sc.getBoundingClientRect().bottom - add.getBoundingClientRect().bottom
        return Math.round(strip)
      }),
    )
    .toBeLessThanOrEqual(1)
})

// #2716 /rework — without a mask nothing dismisses the panel for free, and Naive's
// own click-outside can't help: it only fires when the event target is the modal
// container, which is `pointer-events: none` here. So the two halves of the rule are
// asserted together — an empty-space click closes, a click on another card doesn't.
test('панель закрывается кликом в пустое место, но не кликом по другой задаче', async ({
  page,
  backend,
}) => {
  const { ws, board, columns } = await backend.freshBoard('modal-dismiss')
  const stamp = Date.now().toString(36)
  const first = `Первая ${stamp}`
  const second = `Вторая ${stamp}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: first })
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title: second })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: first }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('task-layout-trigger').click()
  await page.getByTestId('task-layout-sidebar').click()
  await expect(modal).toHaveClass(/tm-sidebar/)
  await panelSettled(modal)

  // A popover of the panel's own is teleported out of the card — dismissing on it
  // would make every control in the header close the thing it belongs to.
  await page.getByTestId('task-layout-trigger').click()
  await expect(page.getByTestId('task-layout-fullscreen')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(modal).toBeVisible()

  // Another card re-points the panel instead of closing it.
  await cardsIn(page, columns[0].name).filter({ hasText: second }).click()
  await expect(modal.locator('.title-input input')).toHaveValue(second)

  // Empty space below the cards in a column: this one closes.
  const col = await page.getByTestId('column').first().boundingBox()
  await page.mouse.click(col.x + col.width / 2, col.y + col.height - 8)
  await expect(page.getByTestId('task-modal')).toHaveCount(0)
})

// #2716 /rework — the panel slides in from the right edge and back out to it. The
// fragile part is not the keyframes but whether our override outranks the centred
// scale-up Naive injects at runtime, so the enter-from state is forced on and the
// resolved transform read back: a lost specificity fight shows up as scale(.9).
test('панель выезжает справа, а не появляется в центре', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('modal-anim')
  const title = `Анимация ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('task-layout-trigger').click()
  await page.getByTestId('task-layout-sidebar').click()
  await expect(modal).toHaveClass(/tm-sidebar/)
  await panelSettled(modal)

  const probe = await modal.evaluate((el) => {
    const read = (cls) => {
      el.classList.add(cls)
      const cs = getComputedStyle(el)
      const out = { transform: cs.transform, opacity: cs.opacity, duration: cs.transitionDuration }
      el.classList.remove(cls)
      return out
    }
    return {
      width: el.getBoundingClientRect().width,
      enterFrom: read('fade-in-scale-up-transition-enter-from'),
      leaveTo: read('fade-in-scale-up-transition-leave-to'),
      enterActive: read('fade-in-scale-up-transition-enter-active'),
      leaveActive: read('fade-in-scale-up-transition-leave-active'),
    }
  })

  // Off-screen to the right by exactly its own width — a pure translate, so no
  // leftover scale from the default transition, and no fade (the panel is opaque).
  const offscreen = `matrix(1, 0, 0, 1, ${probe.width}, 0)`
  expect(probe.enterFrom.transform).toBe(offscreen)
  expect(probe.leaveTo.transform).toBe(offscreen)
  expect(probe.enterFrom.opacity).toBe('1')
  // ...and both directions are actually animated.
  for (const phase of [probe.enterActive, probe.leaveActive]) {
    expect(parseFloat(phase.duration)).toBeGreaterThan(0)
  }
})
