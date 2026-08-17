import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// D5 (#2730). The unit tests drive the threading rules and the panel in
// isolation, and the Go tests drive the endpoints; none of them can show that an
// annotation made in one browser reaches another one without a reload, or that a
// remark survives the paragraph it was made on being deleted.
//
// Two different users, as in the presence spec: the nudge that refetches the
// panel travels over the document socket, and a second tab of the same person
// would not prove it crossed sessions.

async function mateContext(browser, seed, baseURL) {
  const creds = newCredentials(seed.runId, '-cmt')
  await register(creds)
  const context = await browser.newContext({ baseURL })
  const page = await context.newPage()
  await signIn(page, creds)
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  return { context, page, creds }
}

test('документ: замечание к блоку доезжает до соседа и переживает удаление блока', async ({
  page,
  browser,
  backend,
  seed,
}) => {
  const baseURL = new URL(page.url() || 'http://localhost:4174').origin
  const { context, page: matePage, creds } = await mateContext(browser, seed, baseURL)
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: creds.email })

  try {
    await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
    await page.goto('/documents')
    await page.getByRole('button', { name: /Новый документ/ }).click()

    const editor = page.locator('.ProseMirror')
    await expect(editor).toBeVisible()

    const saved = page.waitForResponse(
      (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
    )
    await editor.click()
    await editor.pressSequentially('Исполнитель обязан согласовать')
    expect((await saved).status()).toBe(200)
    await expect(page).toHaveURL(/\/documents\/[^/]+$/)
    const docURL = new URL(page.url()).pathname

    // Give the paragraph back before the mate arrives, so the annotation is not
    // competing with a lock.
    await page.getByRole('button', { name: /К списку/ }).click()
    await page.goto(docURL)
    await expect(page.locator('.ProseMirror')).toContainText('Исполнитель обязан')

    await matePage.goto(docURL)
    await expect(matePage.locator('.ProseMirror')).toContainText('Исполнитель обязан')

    // 1. The mate annotates the paragraph: hovering reveals the gutter, and the
    // "обсудить блок" button arms the draft with the block's text as the quote.
    await matePage.locator('.ProseMirror p').first().hover()
    await matePage.locator('.gutter-btn[title="Обсудить блок"]').click()
    await expect(matePage.locator('.anchor-text')).toContainText('Исполнитель обязан')
    await matePage.getByPlaceholder('Комментарий к блоку…').fill('Нужен срок согласования')
    await matePage.getByRole('button', { name: 'Отправить' }).click()

    // 2. It reaches the author without a reload — this is the socket nudge.
    await expect(page.getByText('Нужен срок согласования')).toBeVisible()
    // …and the block is marked up in the author's copy, with the open count.
    const marked = page.locator('.ProseMirror .doc-block-commented')
    await expect(marked).toHaveCount(1)
    await expect(marked).toHaveAttribute('data-comment-count', '1')

    // 3. The author answers; the reply lands in the same thread for the mate.
    await page.getByPlaceholder('Ответить…').fill('До пятницы')
    await page.getByPlaceholder('Ответить…').press('Enter')
    await expect(matePage.getByText('До пятницы')).toBeVisible()

    // 4. The annotation writes nothing into the document: the text is untouched
    // and no content save was needed to carry the discussion.
    await expect(page.locator('.ProseMirror')).toContainText('Исполнитель обязан согласовать')

    // 5. The author deletes the annotated paragraph. The thread is not deleted
    // with it — a rewritten paragraph is the normal course of a review, and the
    // discussion that asked for the rewrite has to survive it.
    const resaved = page.waitForResponse(
      (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
    )
    await page.locator('.ProseMirror p').first().click()
    await page.keyboard.press('Control+a')
    await page.keyboard.press('Backspace')
    await resaved

    await expect(page.getByText('Блок удалён')).toBeVisible()
    await expect(page.getByText('Нужен срок согласования')).toBeVisible()
    await expect(page.locator('.ProseMirror .doc-block-commented')).toHaveCount(0)

    // 6. Resolving is any member's job, not just the author's, and it takes the
    // thread out of the open count.
    await page.getByRole('button', { name: /Решено/ }).click()
    await expect(page.locator('.thread.done')).toHaveCount(1)
  } finally {
    await context.close()
  }
})

// The /rework of #2730: the counter in the margin says a block is discussed, but
// not *which* card discusses it. The lines are the answer, and they are geometry
// — a unit test can check the arithmetic, only a browser can show that the curve
// starts where the block is and ends where the card is.
test('документ: блоки связаны с аннотациями линиями, без пересечений', async ({ page, seed }) => {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.goto('/documents')
  await page.getByRole('button', { name: /Новый документ/ }).click()

  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()

  const saved = page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
  await editor.click()
  await editor.pressSequentially('Первый пункт договора')
  await page.keyboard.press('Enter')
  await editor.pressSequentially('Второй пункт договора')
  expect((await saved).status()).toBe(200)

  const paragraphs = editor.locator('p')
  await expect(paragraphs).toHaveCount(2)

  // Annotate the SECOND paragraph first. The panel used to show threads in the
  // order the API returned them, so this is the case that produced the crossed
  // arrows in the review screenshot.
  for (const [index, body] of [
    [1, 'Замечание ко второму'],
    [0, 'Замечание к первому'],
  ]) {
    await paragraphs.nth(index).hover()
    await page.locator('.gutter-btn[title="Обсудить блок"]').click()
    await page.getByPlaceholder('Комментарий к блоку…').fill(body)
    await page.getByRole('button', { name: 'Отправить' }).click()
    await expect(page.getByText(body)).toBeVisible()
  }

  // Two blocks discussed, two lines drawn.
  const lines = page.locator('.annotation-lines path')
  await expect(lines).toHaveCount(2)

  // The second /rework of #2730: each line must leave the END of the block's
  // dashed underline, not the middle of the block. Measured rather than eyeballed
  // — the start point is read off the path and compared with the underline's own
  // position, both in page coordinates.
  const start = await page.evaluate(() => {
    const work = document.querySelector('.work').getBoundingClientRect()
    const first = document.querySelector('.ProseMirror .doc-block-commented')
    const block = first.getBoundingClientRect()
    const border = Number.parseFloat(getComputedStyle(first).borderBottomWidth)
    // The topmost line is the one belonging to the first block on the page.
    const paths = [...document.querySelectorAll('.annotation-lines path')].map((n) => {
      const v = (n.getAttribute('d').match(/-?\d+(\.\d+)?/g) || []).map(Number)
      return { x: v[0], y: v[1] }
    })
    const p = paths.sort((a, b) => a.y - b.y)[0]
    return {
      x: work.left + p.x,
      y: work.top + p.y,
      underlineY: block.bottom - border / 2,
      blockMidY: (block.top + block.bottom) / 2,
      blockRight: block.right,
    }
  })
  // On the underline, not on the block's middle — the two are far enough apart
  // that this cannot pass by accident.
  expect(Math.abs(start.y - start.underlineY)).toBeLessThanOrEqual(1)
  expect(Math.abs(start.y - start.blockMidY)).toBeGreaterThan(2)
  // …and at the end of it, where the underline stops.
  expect(Math.abs(start.x - start.blockRight)).toBeLessThanOrEqual(1)

  // Cards are laid out in document order, not in the order the remarks were
  // made — that ordering is the whole anti-crossing mechanism.
  const cards = page.locator('.panel-body .thread')
  await expect(cards.nth(0)).toContainText('Замечание к первому')
  await expect(cards.nth(1)).toContainText('Замечание ко второму')

  // …and the lines themselves do not cross: both ends run the same way down the
  // screen. Read off the path data, which is what is actually drawn.
  const ends = await lines.evaluateAll((nodes) =>
    nodes.map((n) => {
      const v = (n.getAttribute('d').match(/-?\d+(\.\d+)?/g) || []).map(Number)
      return { y1: v[1], y2: v[7] }
    }),
  )
  expect(ends).toHaveLength(2)
  const [top, bottom] = ends[0].y1 <= ends[1].y1 ? ends : [ends[1], ends[0]]
  expect(top.y2).toBeLessThanOrEqual(bottom.y2)

  // Clicking a discussed block highlights its line and only its line.
  await paragraphs.nth(0).click()
  await expect(page.locator('.annotation-lines path.active')).toHaveCount(1)

  // Resolving takes the markup off the block; the line goes with it.
  await cards.nth(0).click()
  await page
    .getByRole('button', { name: /Решено/ })
    .first()
    .click()
  await expect(page.locator('.annotation-lines path')).toHaveCount(1)
})
