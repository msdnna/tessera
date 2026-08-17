import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// Сквозная приёмка раздела «Документы» (#2718): один документ проходит путь,
// которым его ведёт человек — импорт (D8) → связь с задачей и протокол
// согласования (D7) → журнал версий и откат (D6).
//
// Каждая подзадача уже покрыта своей спекой, и покрыта глубже: правила маршрута
// живут в doc-links, коалесцирование и диф — в doc-history, разбор файла — в
// юнитах docImport. Здесь проверяется то, чего не видно ни в одной из них по
// отдельности — что подзадачи не ломают друг друга на одном документе:
//
//   - импортированное тело переживает откат (D6 возвращает именно тот текст,
//     который положил D8, а не пустой документ, с которого начинался бы
//     созданный руками);
//   - связь с задачей и подписанный протокол откат содержимого **переживают**:
//     они лежат отдельно от тела, и возврат текста к прежней редакции не должен
//     стирать согласование, которое к этой редакции и относится;
//   - экспорт отдаёт то же самое тело — то есть D8 читает документ, собранный
//     всеми остальными, а не только свой собственный.
//
// Второй пользователь нужен по той же причине, что в doc-links: маршрут из
// одного согласующего, который сам его и завёл, ничего не доказывает.

const SOURCE_MD = `# Регламент совещаний

Первая редакция, импортированная из файла.

## Порядок
- собрать повестку
- разослать участникам
`

async function mateContext(browser, seed, baseURL, suffix) {
  const creds = newCredentials(seed.runId, suffix)
  await register(creds)
  const context = await browser.newContext({ baseURL })
  const page = await context.newPage()
  await signIn(page, creds)
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
  return { context, page, creds }
}

// Одно сохранение содержимого, дождавшись ответа. Автосохранение отложенное,
// поэтому проверка сразу после ввода гонится с тем самым запросом, от которого
// она зависит.
async function typeAndSave(page, text) {
  const saved = page.waitForResponse(
    (r) => /\/documents\/[^/]+\/content$/.test(r.url()) && r.request().method() === 'PATCH',
  )
  const editor = page.locator('.ProseMirror')
  await editor.click()
  await page.keyboard.press('Control+End')
  await editor.pressSequentially(text)
  expect((await saved).status()).toBe(200)
}

test('документы: импорт → согласование → откат на одном документе', async ({
  page,
  browser,
  backend,
  seed,
}) => {
  const baseURL = new URL(page.url() || 'http://localhost:4174').origin
  const { context, page: matePage, creds } = await mateContext(browser, seed, baseURL, '-acc')
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: creds.email })

  const taskTitle = `Утвердить регламент ${seed.runId}`
  await backend.createTask(seed.columns[0].id, taskTitle)

  try {
    await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), seed.workspaceId)
    await page.goto('/documents')

    // ── D8: импорт ──
    // Подсказка на кнопке обязана называть форматы, которые работают **без**
    // сервиса конвертации: на стенде его нет, и если бы кнопка обещала docx, о
    // недоступности пользователь узнавал бы уже после выбора файла.
    await expect(page.getByTestId('doc-import')).toHaveAttribute('title', /md, json/)

    // Файл кладётся прямо в скрытый input: разбор .md идёт в браузере (D9),
    // поэтому этот путь импорта живёт и на установке без sidecar.
    await page.locator('input.hidden-file').setInputFiles({
      name: 'reglament.md',
      mimeType: 'text/markdown',
      buffer: Buffer.from(SOURCE_MD, 'utf-8'),
    })

    await expect(page).toHaveURL(/\/documents\/[^/]+$/)
    const editor = page.locator('.ProseMirror')
    await expect(editor).toBeVisible()
    // Заголовок из файла стал заголовком документа, а не остался строкой текста.
    await expect(editor.locator('h1')).toContainText('Регламент совещаний')
    await expect(editor).toContainText('Первая редакция, импортированная из файла')
    await expect(editor.locator('li')).toHaveCount(2)
    const docURL = new URL(page.url()).pathname
    // Хвост адреса — слаг, а не id (документ открывается и по слагу), поэтому
    // для прямого вызова API идентификатор берётся из списка, а не из URL.
    const list = await backend.get(`/workspaces/${seed.workspaceId}/documents`)
    const docId = list.find((d) => d.title === 'Регламент совещаний')?.id
    expect(docId, 'импортированный документ не найден в списке').toBeTruthy()

    // ── D7: связь с задачей и маршрут согласования на импортированном теле ──
    // Обход «сначала закрыть обсуждение» здесь стоял до #2738: панели не
    // исключали друг друга, и с тремя открытыми текст схлопывался до 14 px.
    // Теперь история и связи — один сайдбар поверх обсуждения, ширину у текста
    // они не отнимают, и обходить нечего.
    await page.getByTestId('doc-links-toggle').click()
    const panel = page.getByTestId('doc-links')
    await expect(panel).toBeVisible()

    await panel.getByTestId('doc-link-add').click()
    await page.getByPlaceholder('№ или название').fill(taskTitle)
    await page.getByText(taskTitle, { exact: false }).last().click()
    await expect(panel.getByTestId('doc-link')).toHaveCount(1)

    await panel.getByTestId('doc-approval-raise').click()
    await page.getByPlaceholder('Что согласуем').fill('Импортированная редакция')
    await page.getByTestId('doc-approval-approvers').click()
    // Точное имя: автор документа зовётся «E2E <runId>», согласующий — «E2E
    // <runId>-acc», и подстрочный поиск выбрал бы автора.
    await page.locator('.n-base-select-option', { hasText: creds.name }).first().click()
    await page.keyboard.press('Escape')
    await panel.getByRole('button', { name: 'Отправить', exact: true }).click()

    const protocol = panel.getByTestId('doc-approval')
    await expect(protocol).toContainText('На согласовании')
    // Маршрут пришпилен к ревизии, которую создал импорт: согласуется именно
    // импортированный текст, а не «документ вообще».
    await expect(protocol).toContainText(/Версия \d+/)

    await matePage.goto(docURL)
    await matePage.getByTestId('doc-links-toggle').click()
    const matePanel = matePage.getByTestId('doc-links')
    await matePanel.getByTestId('doc-approval-sign').click()
    await matePage.getByRole('button', { name: 'Согласовать', exact: true }).click()
    await expect(protocol).toContainText('Согласовано', { timeout: 10000 })

    // ── D6: журнал, правка поверх согласованного и откат ──
    const journal = page.waitForResponse(
      (r) => /\/documents\/[^/]+\/versions$/.test(r.url()) && r.request().method() === 'GET',
    )
    await page.locator('button[title="История версий"]').click()
    await journal
    const history = page.locator('.doc-history')
    await history.getByRole('button', { name: 'Сохранить версию' }).click()
    await history.getByPlaceholder('Например: согласованная редакция').fill('Согласованная')
    await history.getByRole('button', { name: 'Сохранить', exact: true }).click()
    // Именованная веха адресуется по имени, а не «первая с классом milestone»:
    // отправка на согласование уже поставила свою веху (маршрут пришпиливает
    // ревизию), и в журнале их две — что само по себе правильно.
    const milestone = history.locator('.entry.milestone', { hasText: 'Согласованная' })
    await expect(milestone).toHaveCount(1)

    await typeAndSave(page, ' Правка после согласования.')
    await expect(editor).toContainText('Правка после согласования')

    await milestone.click()
    await history.getByRole('button', { name: 'Восстановить' }).click()
    await page.getByRole('button', { name: 'Подтвердить' }).click()

    // Главное утверждение приёмки: откат вернул **импортированное** тело
    // целиком — заголовок, абзац и список, — а не пустой документ и не одну
    // строку, к которой свёлся бы текст, если бы импорт и версии расходились в
    // том, что считать телом.
    await expect(editor).not.toContainText('Правка после согласования')
    await expect(editor.locator('h1')).toContainText('Регламент совещаний')
    await expect(editor).toContainText('Первая редакция, импортированная из файла')
    await expect(editor.locator('li')).toHaveCount(2)

    // ── что откат не должен был тронуть ──
    // Связь с задачей и подписанный протокол лежат отдельно от содержимого.
    // Откат к согласованной редакции, стирающий её же согласование, был бы
    // ровно тем сортом поломки, которую поодиночке ни одна спека не увидит.
    //
    // Связи открываются заново: с #2738 история и связи делят один сайдбар, и
    // открытие журнала выше закрыло эту панель. Проверке это на пользу — она
    // теперь читает состояние с сервера, а не то, что осталось на экране.
    await page.getByTestId('doc-links-toggle').click()
    await expect(panel).toBeVisible()
    await expect(panel.getByTestId('doc-link')).toHaveCount(1)
    await expect(protocol).toContainText('Согласовано')

    // Сосед видит откат без перезагрузки — по кадру из сокета документа (D4).
    await expect(matePage.locator('.ProseMirror')).toContainText('Регламент совещаний')
    await expect(matePage.locator('.ProseMirror')).not.toContainText('Правка после согласования')

    // ── D8 в обратную сторону: экспорт читает то, что собрали остальные ──
    // HTML собирается на сервере и sidecar'а не требует, поэтому этот конец
    // импорта/экспорта проверяем и на стенде без конвертации.
    const exported = await page.request.get(`/api/documents/${docId}/export?format=html`, {
      headers: { authorization: `Bearer ${seed.token}` },
    })
    const html = await exported.text()
    // Тело в сообщении: ответ здесь либо HTML, либо json с причиной, и без него
    // «ожидали 200, получили 4xx» не говорит, дело в правах или в формате.
    expect(exported.status(), html.slice(0, 200)).toBe(200)
    expect(html).toContain('Регламент совещаний')
    expect(html).toContain('разослать участникам')
    expect(html).not.toContain('Правка после согласования')
  } finally {
    await context.close()
  }
})
