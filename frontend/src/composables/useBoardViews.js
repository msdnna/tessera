import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from '@/composables/useMessage'
import { boards as boardsApi } from '@/api'
import { useUndoStore } from '@/stores/undo'
import { humanizeError } from '@/utils/errors'
import {
  defaultView,
  moveNeighbours,
  neighboursOf,
  normalizeViewConfig,
  orderViews,
  sameViewConfig,
  viewChanges,
  viewNameTaken,
} from '@/utils/boardViews'

// Saved board views (v2, #2970 — #2987, #3015, #3016): the server list, the
// loaded view, its «изменено» state and every action on a view. The board owns
// the state a view captures; it hands over `currentConfig()` (the state now)
// and `applyConfig(cfg)` (load a config in bulk). Everything else lives here.
//
// Autosave is gone (#2970 decision 3): a loaded view is never rewritten behind
// the reader's back. The board's own state is still remembered locally per
// device (useBoardViewConfig), and so is which view it was loaded from — the
// chip and «изменено» survive a reload.
//
// The default view (the star) applies once per browser session per board: open
// the board in a new tab and it starts from the default, wander off within the
// session and the board is where you left it.
const LOADED_KEY = (id) => `tessera_view_loaded_${id}`
const SESSION_KEY = (id) => `tessera_view_session_${id}`

function lsGet(k) {
  try {
    return localStorage.getItem(k) || ''
  } catch {
    return ''
  }
}
function lsSet(k, v) {
  try {
    if (v) localStorage.setItem(k, v)
    else localStorage.removeItem(k)
  } catch {
    /* storage disabled — the chip just won't survive a reload */
  }
}

export function useBoardViews({ boardId, currentConfig, applyConfig }) {
  const { t } = useI18n()
  const message = useMessage()
  const undo = useUndoStore()

  const raw = ref([])
  const views = computed(() => orderViews(raw.value))
  const mine = computed(() => views.value.filter((v) => v.scope !== 'board'))
  const shared = computed(() => views.value.filter((v) => v.scope === 'board'))
  const loadedId = ref('')
  const loaded = computed(() => views.value.find((v) => v.id === loadedId.value) || null)
  // A request in flight on a view (its row shows a spinner) and the last refusal
  // ({ id, title, sub }) — drawn on the row it belongs to.
  const busyId = ref('')
  const failure = ref(null)
  // «Сохранить как…» asked from outside the panel (⌘⇧S, the chip menu, a locked
  // view's «Сохранить как моё»): the panel opens its inline field with this name.
  const saveAsDraft = ref(null)
  let draftSeq = 0

  // Re-read only when something the view stores changes; the board calls
  // `currentConfig()` reactively, so this recomputes on every toolbar edit.
  const nowConfig = computed(() => normalizeViewConfig(currentConfig()))
  const changed = computed(
    () => !!loaded.value && !sameViewConfig(loaded.value.config, nowConfig.value),
  )
  const changes = computed(() =>
    loaded.value && changed.value ? viewChanges(loaded.value.config, nowConfig.value) : [],
  )

  function setLoaded(id) {
    loadedId.value = id || ''
    lsSet(LOADED_KEY(boardId.value), loadedId.value)
  }

  async function load() {
    try {
      raw.value = (await boardsApi.views(boardId.value)).data || []
    } catch {
      raw.value = []
    }
    // A view deleted elsewhere unloads; the board keeps its state.
    if (loadedId.value && !raw.value.some((v) => v.id === loadedId.value)) setLoaded('')
  }

  function apply(v) {
    if (!v) return
    applyConfig(v.config || {})
    setLoaded(v.id)
    failure.value = null
  }

  // After the first load on a board: the default view (once a session), else the
  // view the board was last loaded from. Returns true when a view was applied.
  // `applyDefault: false` — the reader already changed the board while it was
  // loading; their click wins over the star.
  function onOpen({ applyDefault = true } = {}) {
    const id = boardId.value
    let fresh
    try {
      fresh = !sessionStorage.getItem(SESSION_KEY(id))
      sessionStorage.setItem(SESSION_KEY(id), '1')
    } catch {
      fresh = false
    }
    const remembered = lsGet(LOADED_KEY(id))
    const def = defaultView(views.value)
    if (applyDefault && def && fresh && remembered !== def.id) {
      apply(def)
      return true
    }
    loadedId.value = views.value.some((v) => v.id === remembered) ? remembered : ''
    if (!loadedId.value && remembered) lsSet(LOADED_KEY(id), '')
    return false
  }

  // 0 — no view; 1–9 — the view at that place in the list.
  function pick(n) {
    if (n === 0) return clear()
    const v = views.value[n - 1]
    if (v) apply(v)
  }

  // «Без представления»: the board keeps what is on screen, the chip goes.
  function clear() {
    setLoaded('')
  }

  // ⌘⌫ — back to what the loaded view stores.
  function reset() {
    if (loaded.value && changed.value) apply(loaded.value)
  }

  function refuse(v, e, title) {
    failure.value = { id: v?.id || '', title, sub: humanizeError(e) }
  }

  function requestSaveAs(name = '') {
    saveAsDraft.value = { name, n: ++draftSeq }
  }
  // The panel's field took the ask.
  function takeSaveAs() {
    const d = saveAsDraft.value
    saveAsDraft.value = null
    return d
  }

  // ⌘S — re-save the loaded view from the board. A view the reader may not edit
  // turns into «Сохранить как моё» (a personal copy under the same name).
  async function update() {
    const v = loaded.value
    if (!v) return requestSaveAs()
    if (!v.editable) return requestSaveAs(v.name)
    if (!changed.value) return
    busyId.value = v.id
    try {
      const { data } = await boardsApi.updateView(v.id, { config: currentConfig() })
      replace(data)
      failure.value = null
      message.success(t('board.views.updated', { name: v.name }))
    } catch (e) {
      refuse(v, e, t('board.views.updateFailed'))
    } finally {
      busyId.value = ''
    }
  }

  function replace(v) {
    if (!v) return
    raw.value = raw.value.map((x) => (x.id === v.id ? { ...x, ...v } : x))
  }

  // Save the board as a new view (or over the same-named one in that list — the
  // server upserts, the panel checks the name first). Returns the view or null.
  async function saveAs(name, { scope = 'user' } = {}) {
    const n = (name || '').trim()
    if (!n) return null
    try {
      const { data } = await boardsApi.saveView(boardId.value, {
        name: n,
        scope,
        config: currentConfig(),
      })
      await load()
      setLoaded(data.id)
      failure.value = null
      return data
    } catch (e) {
      message.error(humanizeError(e))
      return null
    }
  }

  async function patch(v, body, failTitle) {
    busyId.value = v.id
    try {
      const { data } = await boardsApi.updateView(v.id, body)
      failure.value = null
      return data
    } catch (e) {
      refuse(v, e, failTitle)
      return null
    } finally {
      busyId.value = ''
    }
  }

  // F2 / menu «Переименовать». Returns '' on success, else the reason (the
  // inline field keeps it open with the error under it).
  async function rename(v, name) {
    const n = (name || '').trim()
    if (!n || n === v.name) return ''
    if (viewNameTaken(raw.value, n, v.scope, v.id)) return t('board.views.nameTaken')
    try {
      const { data } = await boardsApi.updateView(v.id, { name: n })
      replace(data)
      return ''
    } catch (e) {
      return humanizeError(e)
    }
  }

  // Menu «Обновить из текущего» on any editable row (not only the loaded one).
  async function overwrite(v) {
    const data = await patch(v, { config: currentConfig() }, t('board.views.updateFailed'))
    if (data) {
      replace(data)
      setLoaded(data.id)
    }
  }

  // The star (#3015): own views — for me; shared — for everybody (managers).
  async function setDefault(v, on) {
    const data = await patch(v, { is_default: on }, t('board.views.updateFailed'))
    if (data) await load()
  }

  async function setScope(v, scope) {
    const data = await patch(v, { scope }, t('board.views.shareFailed'))
    if (data) await load()
  }

  // ⌥↑ / ⌥↓ (#3016): one step inside the view's own list.
  async function move(v, dir) {
    const nb = moveNeighbours(raw.value, v.id, dir)
    if (!nb) return
    const data = await patch(v, nb, t('board.views.updateFailed'))
    if (data) {
      replace(data)
      await load()
    }
  }

  // ⌫ — at once, with an undo toast that puts the view back in its place.
  async function remove(v) {
    const nb = neighboursOf(raw.value, v.id)
    const wasLoaded = loadedId.value === v.id
    busyId.value = v.id
    try {
      await boardsApi.deleteView(v.id)
    } catch (e) {
      refuse(v, e, t('board.views.deleteFailed'))
      return
    } finally {
      busyId.value = ''
    }
    raw.value = raw.value.filter((x) => x.id !== v.id)
    if (wasLoaded) setLoaded('')
    undo.push({
      text: { key: 'board.views.deleted', params: { name: v.name } },
      doneText: { key: 'board.views.restored', params: { name: v.name } },
      undo: async () => {
        const { data } = await boardsApi.saveView(boardId.value, {
          name: v.name,
          scope: v.scope || 'user',
          config: v.config || {},
          is_default: !!v.is_default,
        })
        if (nb.before_id || nb.after_id) {
          try {
            await boardsApi.updateView(data.id, nb)
          } catch {
            /* a neighbour gone meanwhile — the view stays at the end */
          }
        }
        await load()
        if (wasLoaded) setLoaded(data.id)
      },
    })
  }

  function reset_() {
    raw.value = []
    loadedId.value = ''
    busyId.value = ''
    failure.value = null
    saveAsDraft.value = null
  }

  return {
    views,
    mine,
    shared,
    loadedId,
    loaded,
    changed,
    changes,
    busyId,
    failure,
    saveAsDraft,
    load,
    onOpen,
    apply,
    pick,
    clear,
    reset,
    update,
    saveAs,
    requestSaveAs,
    takeSaveAs,
    rename,
    overwrite,
    setDefault,
    setScope,
    move,
    remove,
    forget: reset_,
  }
}
