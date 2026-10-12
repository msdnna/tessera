// Окно «Разрешение конфликтов» (v2, #2975): выбор версии — по полю, а сервер
// по-прежнему принимает одно из трёх решений на конфликт (theirs / ours /
// manual). Проверяем отображение выбора в решение: всё GitLab → theirs, всё
// Tessera → ours, смесь → manual со значением каждого поля; поле, изменённое с
// одной стороны, берётся само; «Применить» закрыто, пока не решены все поля;
// черновик переживает закрытие окна.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

const api = {
  gitlab: {
    conflicts: vi.fn(),
    resolveConflict: vi.fn(() => Promise.resolve({ data: {} })),
    resolveConflicts: vi.fn(() => Promise.resolve({ data: {} })),
  },
  tasks: { get: vi.fn() },
}
vi.mock('@/api', () => api)
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const stubs = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /></div>' },
  Modal: { template: '<div class="n-modal-stub"><slot /></div>' },
}

// Заголовок и описание поменяли обе стороны; срок — только GitLab (авто).
const TITLE_DESC = {
  id: 'c1',
  task_id: 't1',
  task_number: 7,
  task_title: 'Экспорт отчёта',
  change_kind: 'title_desc',
  gl_iid: 318,
  fields: [
    { field: 'title', base: 'Экспорт', ours: 'Экспорт в PDF', theirs: 'Экспорт (PDF и CSV)' },
    { field: 'description', base: 'A', ours: 'A + B', theirs: 'A + C' },
  ],
}
const STATE = {
  id: 'c2',
  task_id: 't2',
  task_number: 8,
  task_title: 'Права гостей',
  change_kind: 'state',
  fields: [{ field: 'state', base: 'opened', ours: 'closed', theirs: 'opened' }],
}

async function open(list = [TITLE_DESC]) {
  api.gitlab.conflicts.mockResolvedValue({ data: list })
  const C = (await import('@/components/ConflictResolverModal.vue')).default
  const w = mount(C, { props: { show: false, wsId: 'ws1' }, global: { stubs } })
  await w.setProps({ show: true })
  await flushPromises()
  return w
}
const fieldRow = (w, i) => w.findAll('[data-testid="conflict-field"]')[i]
const applyBtn = (w) => w.find('[data-testid="conflict-apply"]')

describe('ConflictResolverModal · выбор по полю', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    api.gitlab.resolveConflict.mockResolvedValue({ data: {} })
  })

  it('не даёт применить, пока не решены все поля', async () => {
    const w = await open()
    expect(applyBtn(w).attributes('disabled')).toBeDefined()
    await fieldRow(w, 0).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    expect(applyBtn(w).attributes('disabled')).toBeDefined()
    await fieldRow(w, 1).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    expect(applyBtn(w).attributes('disabled')).toBeUndefined()
  })

  it('всё GitLab — решение theirs, без значений', async () => {
    const w = await open()
    await fieldRow(w, 0).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await fieldRow(w, 1).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t1', 'c1', { resolution: 'theirs' })
  })

  it('всё Tessera — решение ours', async () => {
    const w = await open()
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await fieldRow(w, 1).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t1', 'c1', { resolution: 'ours' })
  })

  it('смесь сторон — manual со значением каждого поля', async () => {
    const w = await open()
    await fieldRow(w, 0).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await fieldRow(w, 1).find('[data-testid="conflict-pick-base"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t1', 'c1', {
      resolution: 'manual',
      value: { title: 'Экспорт (PDF и CSV)', description: 'A' },
    })
  })

  it('ручное объединение уходит своим текстом', async () => {
    const w = await open()
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    // M на поле описания — через ту же развилку, что и клавиша.
    await fieldRow(w, 1).trigger('click')
    w.vm.keyPick('manual')
    await flushPromises()
    await w.find('[data-testid="conflict-manual"]').setValue('A + B + C')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t1', 'c1', {
      resolution: 'manual',
      value: { title: 'Экспорт в PDF', description: 'A + B + C' },
    })
  })

  it('поле, изменённое с одной стороны, выбрано само', async () => {
    const one = {
      ...TITLE_DESC,
      fields: [
        { field: 'due', base: '2026-10-01', ours: '2026-10-01', theirs: '2026-10-09' },
        { field: 'title', base: 'X', ours: 'Y', theirs: 'Z' },
      ],
    }
    const w = await open([one])
    expect(fieldRow(w, 0).classes()).toContain('auto')
    await fieldRow(w, 1).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t1', 'c1', { resolution: 'theirs' })
  })

  it('дискретное поле (статус) решается выбором стороны', async () => {
    const w = await open([STATE])
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t2', 'c2', { resolution: 'ours' })
  })

  it('черновик выбора переживает закрытие окна', async () => {
    const w = await open()
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await w.setProps({ show: false })
    await w.setProps({ show: true })
    await flushPromises()
    expect(fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').classes()).toContain('on')
  })

  it('последний разрешённый конфликт закрывает окно', async () => {
    const w = await open([STATE])
    await fieldRow(w, 0).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(w.emitted('update:show')?.at(-1)).toEqual([false])
    expect(w.emitted('resolved')).toHaveLength(1)
  })

  // #3089: «Применить» берёт все готовые задачи очереди одним запросом.
  it('готовые задачи очереди уходят одним пакетом', async () => {
    const w = await open([TITLE_DESC, STATE])
    // Вторая задача: решаем статус и возвращаемся к первой.
    await w.findAll('[data-testid="conflict-item"]')[1].trigger('click')
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await w.findAll('[data-testid="conflict-item"]')[0].trigger('click')
    await fieldRow(w, 0).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await fieldRow(w, 1).find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).not.toHaveBeenCalled()
    expect(api.gitlab.resolveConflicts).toHaveBeenCalledWith('ws1', [
      { task_id: 't1', conflict_id: 'c1', resolution: 'theirs' },
      { task_id: 't2', conflict_id: 'c2', resolution: 'ours' },
    ])
    expect(w.emitted('resolved')).toHaveLength(2)
  })

  it('«Отложить» убирает задачу в конец очереди и из пакета', async () => {
    localStorage.clear()
    const w = await open([TITLE_DESC, STATE])
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await w.find('[data-testid="conflict-postpone"]').trigger('click')
    const items = w.findAll('[data-testid="conflict-item"]')
    expect(items[1].text()).toContain('Экспорт отчёта')
    expect(items[0].classes()).toContain('on')
    // Черновик отложенной задачи сохранён и переживёт перезагрузку.
    expect(JSON.parse(localStorage.getItem('tessera_conflict_drafts'))).toEqual({
      c1: { title: { pick: 'ours' } },
    })
    await fieldRow(w, 0).find('[data-testid="conflict-pick-ours"]').trigger('click')
    await applyBtn(w).trigger('click')
    await flushPromises()
    expect(api.gitlab.resolveConflict).toHaveBeenCalledWith('t2', 'c2', { resolution: 'ours' })
    expect(api.gitlab.resolveConflicts).not.toHaveBeenCalled()
  })
})
