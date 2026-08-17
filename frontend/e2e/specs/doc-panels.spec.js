import { test, expect } from '../fixtures.js'

// #2738 — панели раздела «Документы» отнимали ширину у текста.
//
// Дефект нашла сквозная приёмка #2718, и он жил ровно в стыке подзадач: каждая
// спека открывала свою одну панель, а обсуждение, историю и связи не открывал
// втроём никто. На 1280 px три колонки по 300 px оставляли тексту 14 px —
// редактор становился нередактируемым в буквальном смысле, Playwright не мог
// кликнуть в `.ProseMirror`.
//
// Поэтому спека утверждает не «панель открылась», а **ширину полосы текста**:
// открытая панель — это то, что видно, а схлопнувшийся текст — то, из-за чего
// приходят с багом. Проверяется на самом узком десктопе, который мы держим за
// рабочий, потому что дефект — про арифметику ширин, и шире он просто не
// проявится.

const VIEWPORT = { width: 1280, height: 800 }
// Полоса, ниже которой текст перестаёт быть текстом. Взято с запасом от
// реальной ширины при одном сайдбаре (~618 px): спека сторожит схлопывание, а
// не пиксель, и не должна краснеть от смены отступа на 4 px.
const MIN_EDITOR_PX = 480

async function newDocument(page, seed) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  await page.setViewportSize(VIEWPORT)
  await page.goto('/documents')
  await page.getByRole('button', { name: /Новый документ/ }).click()
  const editor = page.locator('.ProseMirror')
  await expect(editor).toBeVisible()
  return editor
}

async function editorWidth(editor) {
  const box = await editor.boundingBox()
  return box?.width ?? 0
}

test('документ: три панели подряд не отнимают ширину у текста', async ({ page, seed }) => {
  const editor = await newDocument(page, seed)

  // Исходное состояние: обсуждение открыто, оглавление — полоса штрихов. Замер
  // берётся до всякого клика, чтобы дальше сравнивать с ним, а не с константой:
  // отступы вокруг редактора спеку не касаются.
  const base = await editorWidth(editor)
  expect(base).toBeGreaterThan(MIN_EDITOR_PX)

  // Тот самый сценарий, который не поймала ни одна спека подзадач: обе панели
  // подряд, поверх открытого по умолчанию обсуждения.
  await page.getByTestId('doc-links-toggle').click()
  await expect(page.getByTestId('doc-links')).toBeVisible()
  expect(await editorWidth(editor)).toBe(base)

  await page.locator('button[title="История версий"]').click()
  await expect(page.locator('.doc-history')).toBeVisible()
  expect(await editorWidth(editor)).toBe(base)

  // Сайдбар показывает одну панель за раз — это и есть причина, по которой
  // ширина выше не изменилась ни разу.
  await expect(page.getByTestId('doc-side')).toHaveCount(1)
  await expect(page.getByTestId('doc-links')).toHaveCount(0)

  // И главное — в редактор по-прежнему можно писать. Дефект проявлялся именно
  // так: клик перехватывала то панель, то свернувшийся в колонку тулбар.
  await editor.click()
  await editor.pressSequentially('Текст поверх открытой панели')
  await expect(editor).toContainText('Текст поверх открытой панели')
})

test('документ: сайдбар закрывается своей же кнопкой и не открыт при входе', async ({
  page,
  seed,
}) => {
  await newDocument(page, seed)

  // Сайдбар не открыт на входе — отдельное утверждение, а не побочный эффект
  // проверки выше. Обсуждение остаётся открытым по умолчанию (оно неотрывная
  // часть документа и держит линии к аннотациям), а история и связи — нет:
  // сайдбар это то, что запрашивают.
  await expect(page.getByTestId('doc-side')).toHaveCount(0)
  await expect(page.locator('.doc-comments')).toBeVisible()

  await page.getByTestId('doc-links-toggle').click()
  await expect(page.getByTestId('doc-links')).toBeVisible()

  // Повторный клик по кнопке уже открытой панели закрывает сайдбар: кнопки в
  // тулбаре должны читаться как тумблеры, хотя слот у них общий.
  await page.getByTestId('doc-links-toggle').click()
  await expect(page.getByTestId('doc-side')).toHaveCount(0)
})
