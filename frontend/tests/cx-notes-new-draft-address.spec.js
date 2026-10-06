import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// Notes section (#2932): the open note is mirrored in the URL as `?note=<slug>`
// so a note can be linked to and survives a refresh. Starting a NEW note has to
// clear that mirror — otherwise `loadList()`, which re-reads the query on every
// refetch, re-opens the note that was open before and the just-saved one is
// nowhere on screen.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const api = {
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}
// The workspaces store destructures these at import time; nothing here calls them.
vi.mock('@/api', () => ({ notes: api, workspaces: {}, projects: {} }))

const { default: NotesView } = await import('@/views/NotesView.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')

const note = (over = {}) => ({
  id: 'n1',
  workspace_id: 'ws1',
  title: 'Регламент',
  body: 'Дежурим по очереди',
  slug: 'reglament',
  ...over,
})

const OTHER = note({ id: 'n2', title: 'Идеи', body: 'Сгруппировать по тегам', slug: 'idei' })

// useResponsive reads matchMedia on mount; jsdom has none.
function setViewport({ mobile }) {
  window.matchMedia = vi.fn().mockReturnValue({
    matches: mobile,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
}

const mounted = []
async function mountAt(path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/notes', component: NotesView },
      { path: '/:rest(.*)*', component: { template: '<div />' } },
    ],
  })
  router.push(path)
  await router.isReady()
  const w = mount(NotesView, { global: { plugins: [router] } })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

// What the editor currently holds — the two n-inputs, in order (title, body).
function editor(w) {
  const title = w.find('.title input')
  const body = w.find('textarea')
  return {
    title: title.exists() ? title.element.value : null,
    body: body.exists() ? body.element.value : null,
  }
}

async function type(w, { title, body }) {
  await w.find('.title input').setValue(title)
  await w.find('textarea').setValue(body)
}

function saveButton(w) {
  return w.findAll('button').find((b) => b.text() === 'Сохранить')
}

function newButton(w) {
  return w.findAll('button').find((b) => b.text().includes('Новая'))
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  setViewport({ mobile: false })
  for (const fn of Object.values(api)) fn.mockReset()
  for (const fn of Object.values(toast)) fn.mockReset()
  api.list.mockResolvedValue({ data: [note(), OTHER] })
  useWorkspacesStore().currentId = 'ws1'
})

afterEach(() => {
  // A wrapper left mounted keeps re-rendering on later mounts and throws inside
  // the scheduler, which silently drops the render under test.
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
})

describe('a new note is not hijacked by the previously opened one (#2932)', () => {
  it('opens the note named in the URL', async () => {
    const { w } = await mountAt('/notes?note=reglament')
    expect(editor(w)).toEqual({ title: 'Регламент', body: 'Дежурим по очереди' })
  })

  it('leaves the address empty while a new note is being written', async () => {
    const { w, router } = await mountAt('/notes?note=reglament')

    await newButton(w).trigger('click')
    await flushPromises()

    expect(editor(w)).toEqual({ title: '', body: '' })
    // Nothing is open, so nothing may be addressed: a stale ?note= here is what
    // loadList() later mistakes for "this is the note to show".
    expect(router.currentRoute.value.query.note).toBeUndefined()
  })

  it('keeps the just-saved note in the editor instead of reopening the old one', async () => {
    const fresh = note({ id: 'n3', title: 'Черновик', body: 'Созвон в четверг', slug: 'chernovik' })
    api.create.mockResolvedValue({ data: fresh })
    const { w, router } = await mountAt('/notes?note=reglament')

    await newButton(w).trigger('click')
    await flushPromises()
    await type(w, { title: 'Черновик', body: 'Созвон в четверг' })
    // The refetch that follows the save returns the new note too.
    api.list.mockResolvedValue({ data: [fresh, note(), OTHER] })
    await saveButton(w).trigger('click')
    await flushPromises()

    expect(api.create).toHaveBeenCalledWith('ws1', {
      title: 'Черновик',
      body: 'Созвон в четверг',
    })
    expect(editor(w)).toEqual({ title: 'Черновик', body: 'Созвон в четверг' })
    expect(router.currentRoute.value.query.note).toBe('chernovik')
    // And the second «Сохранить» must update the new note, not the old one.
    await saveButton(w).trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith('n3', {
      title: 'Черновик',
      body: 'Созвон в четверг',
    })
  })

  it('still switches notes when the address changes under an open one', async () => {
    const { w, router } = await mountAt('/notes?note=reglament')

    // A deep link from the search bar while the section is already mounted.
    await router.replace({ path: '/notes', query: { note: 'idei' } })
    await flushPromises()

    expect(editor(w)).toEqual({ title: 'Идеи', body: 'Сгруппировать по тегам' })
  })

  it('a new note started after a delete is still the one that stays', async () => {
    // Deleting leaves ?note= pointing at a row that is gone: harmless on its own
    // (nothing matches it), but the slug is freed, so a note titled the same way
    // lands right back on it. The draft must not inherit that pointer.
    api.remove.mockResolvedValue({})
    const fresh = note({ id: 'n3', title: 'Регламент', body: 'Новый порядок' })
    api.create.mockResolvedValue({ data: fresh })
    const { w, router } = await mountAt('/notes?note=reglament')

    api.list.mockResolvedValue({ data: [OTHER] })
    // The delete lives behind a popconfirm; its confirm button is the handler.
    await w
      .findAll('button')
      .find((b) => b.text() === 'Удалить')
      .trigger('click')
    await flushPromises()
    for (const btn of document.querySelectorAll('.n-popconfirm__action button')) {
      if (btn.textContent.trim() === 'Удалить') btn.click()
    }
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith('n1')

    await newButton(w).trigger('click')
    await flushPromises()
    await type(w, { title: 'Регламент', body: 'Новый порядок' })
    api.list.mockResolvedValue({ data: [fresh, OTHER] })
    await saveButton(w).trigger('click')
    await flushPromises()

    expect(editor(w)).toEqual({ title: 'Регламент', body: 'Новый порядок' })
    expect(router.currentRoute.value.query.note).toBe('reglament')
  })
})
