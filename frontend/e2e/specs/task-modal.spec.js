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
  const { board, columns } = await backend.freshBoard('modal')
  const title = `Задача с описанием ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
    description: '## Заголовок описания\n\nОбычный абзац.',
  })

  await openBoard(page, board.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()
  // The title is an editable field, so it holds the text as a value — `getByText`
  // would never see it.
  await expect(modal.getByTestId('task-title')).toHaveValue(title)
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

test('ответ в треде остаётся вложенным после перезагрузки', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('threads')
  const title = `Задача с тредом ${Date.now().toString(36)}`
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title,
  })
  const root = await backend.post(`/tasks/${task.id}/comments`, { body: 'Корневой комментарий' })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: title }).click()

  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()

  const thread = modal.locator('.c-thread')
  await thread.getByTestId('comment-reply').click()
  const replyBody = `Ответ из e2e ${Date.now().toString(36)}`
  const composer = modal.locator('.c-reply-add textarea')
  await composer.fill(replyBody)
  await composer.press('Control+Enter')

  // Nested, not appended to the flat list: the reply has to land inside the
  // root's .c-replies branch, which is the whole point of the feature.
  await expect(thread.locator('.c-replies')).toContainText(replyBody)

  // And the nesting is stored, not just rendered — the server has to give the
  // reply back with its parent, or the tree dies on the next page load.
  await expect
    .poll(async () => {
      const comments = await backend.get(`/tasks/${task.id}/comments`)
      return comments.find((c) => c.body === replyBody)?.parent_id
    })
    .toBe(root.id)

  // No second click after the reload: the open task lives in `?task=`, so the
  // modal reopens by itself and would swallow the click on the card behind it.
  await page.reload()
  const reloaded = page.getByTestId('task-modal')
  await expect(reloaded).toBeVisible()
  await expect(reloaded.locator('.c-thread .c-replies')).toContainText(replyBody)
})

test('модалка закрывается по Escape', async ({ page, backend }) => {
  const { board, columns } = await backend.freshBoard('modal-esc')
  const title = `Закрой меня ${Date.now().toString(36)}`
  await backend.post(`/boards/${board.id}/tasks`, { column_id: columns[0].id, title })

  await openBoard(page, board.id)
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
  await expect(modal.getByTestId('task-title')).toHaveValue(second)
  await expect(modal).toHaveClass(/tm-sidebar/)

  // The choice is device-level and survives a reload.
  await page.reload()
  await expect(page.getByTestId('column').first()).toBeVisible()
  await cardsIn(page, columns[0].name).filter({ hasText: first }).click()
  await expect(page.getByTestId('task-modal')).toHaveClass(/tm-sidebar/)
})

// #2716 /rework — the panel is a fixed-height box, so its body has to scroll on its
// own (v2, #2958: the content column `.tm-scroll`). A rule that targets the wrong
// element fails silently, hence an assertion on the observable outcome — scrollTop
// moving.
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

  const body = modal.locator('.tm-scroll')
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

// v2 (#2958): the comments composer is pinned under the scrolling column, outside
// it — so nothing can scroll beneath it (the old sticky composer let the last
// comment bleed through a padding strip, #2716). The guard is geometric: the
// scroller ends where the composer begins.
test('панель: композер комментариев под колонкой, комментарии не просвечивают', async ({
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

  await expect
    .poll(() =>
      modal.evaluate((root) => {
        const sc = root.querySelector('.tm-scroll')
        const add = root.querySelector('.comment-add')
        if (!sc || !add || sc.contains(add)) return 999
        return Math.round(sc.getBoundingClientRect().bottom - add.getBoundingClientRect().top)
      }),
    )
    .toBeLessThanOrEqual(1)
})

// v2 (#2958): there is no Save button any more — the title commits on Enter (or
// blur) and that has to reach the server, not just the field.
test('заголовок сохраняется без кнопки «Сохранить»', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('modal-autosave')
  const stamp = Date.now().toString(36)
  const task = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Старое имя ${stamp}`,
  })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name).filter({ hasText: stamp }).click()
  const modal = page.getByTestId('task-modal')
  await expect(modal).toBeVisible()

  const field = modal.getByTestId('task-title')
  await field.fill(`Новое имя ${stamp}`)
  await field.press('Enter')
  await expect
    .poll(async () => (await backend.get(`/tasks/${task.id}`)).title)
    .toBe(`Новое имя ${stamp}`)
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
  await expect(modal.getByTestId('task-title')).toHaveValue(second)

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

// v2 (#2958): J/K walk the board's order without closing the modal, the header
// counts the position, and the address follows (replaced, so Back still closes).
test('J/K листают задачи доски, P меняет приоритет с клавиатуры', async ({ page, backend }) => {
  const { ws, board, columns } = await backend.freshBoard('modal-jk')
  const stamp = Date.now().toString(36)
  const first = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Первая ${stamp}`,
  })
  const second = await backend.post(`/boards/${board.id}/tasks`, {
    column_id: columns[0].id,
    title: `Вторая ${stamp}`,
  })

  await openBoard(page, board.id, ws.id)
  await cardsIn(page, columns[0].name)
    .filter({ hasText: `Первая ${stamp}` })
    .click()
  const modal = page.getByTestId('task-modal')
  const title = modal.getByTestId('task-title')
  await expect(title).toHaveValue(`Первая ${stamp}`)

  await page.keyboard.press('j')
  await expect(title).toHaveValue(`Вторая ${stamp}`)
  await expect(page).toHaveURL(new RegExp(`task=${second.number}`))
  await page.keyboard.press('k')
  await expect(title).toHaveValue(`Первая ${stamp}`)

  // «P, then 3» — the high priority, saved without touching the mouse.
  await page.keyboard.press('p')
  await page.keyboard.press('3')
  await expect.poll(async () => (await backend.get(`/tasks/${first.id}`)).priority).toBe(3)
})
