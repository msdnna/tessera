// The workspace's palette commands and task templates (#3069): loaded per
// workspace by the layout, refreshed by `macros.changed` / `task_templates.changed`,
// run from the palette (>trigger) or by their keys (shortcuts/dynamic.js).
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  workspaces as wsApi,
  boards as boardsApi,
  projects as projectsApi,
  tasks as tasksApi,
  documents as docsApi,
} from '@/api'
import { i18n } from '@/i18n'
import { useAuthStore } from './auth'
import { useBoardViewStore } from './boardView'
import { macroBlocked, runMacro } from '@/utils/macroRun'
import { setMacroKeys } from '@/shortcuts/dynamic'
import { messageApi } from '@/composables/useMessage'
import { humanizeError } from '@/utils/errors'

export const useMacrosStore = defineStore('macros', () => {
  const wsId = ref(null)
  const macros = ref([])
  const templates = ref([])
  const canManage = ref(false)
  const loaded = ref(false)
  let seq = 0

  const enabled = computed(() => macros.value.filter((m) => m.enabled))

  async function load(id = wsId.value) {
    if (!id) return
    if (id !== wsId.value) {
      macros.value = []
      templates.value = []
      loaded.value = false
    }
    wsId.value = id
    const mine = ++seq
    const [m, tp] = await Promise.allSettled([wsApi.macros(id), wsApi.taskTemplates(id)])
    if (mine !== seq) return
    if (m.status === 'fulfilled') {
      macros.value = m.value.data?.macros || []
      canManage.value = !!m.value.data?.can_manage
    }
    if (tp.status === 'fulfilled') templates.value = tp.value.data || []
    loaded.value = true
    bindKeys()
  }

  function bindKeys() {
    setMacroKeys(
      enabled.value
        .filter((m) => m.keys)
        .map((m) => ({ id: m.id, keys: m.keys, run: () => run(m) })),
    )
  }

  function onEvent(ev) {
    if (!wsId.value || ev?.scope !== wsId.value) return
    if (ev.type === 'macros.changed' || ev.type === 'task_templates.changed') load()
  }

  function blockedReason(m) {
    return macroBlocked(m, { boardId: useBoardViewStore().boardId })
  }

  // Run a command with the screen it is run from; reports its own outcome.
  // `go` — the palette's own navigation (it unwinds its overlay first, or its
  // history.back() would cancel the push).
  async function run(m, { go = null } = {}) {
    const t = i18n.global.t
    const message = messageApi()
    const why = blockedReason(m)
    if (why) {
      message.info(
        why === 'soon' ? t('shell.commands.macro.soon') : t('shell.commands.macro.needBoard'),
      )
      return null
    }
    const bv = useBoardViewStore()
    try {
      const docTemplates =
        m.action === 'create_document' ? (await docsApi.templates(wsId.value)).data || [] : []
      const res = await runMacro(m, {
        wsId: wsId.value,
        me: useAuthStore().user?.id,
        board: bv.boardId
          ? {
              id: bv.boardId,
              projectId: bv.projectId,
              columns: bv.columns,
              doneColumnId: bv.board?.done_column_id || null,
              milestones: bv.milestonesList,
            }
          : null,
        templates: templates.value,
        docTemplates,
        api: { boards: boardsApi, projects: projectsApi, tasks: tasksApi, documents: docsApi },
        open: (url) => window.open(url, '_blank', 'noopener'),
        // A key, not the palette: the router, loaded lazily — the store is
        // imported by the palette, which tests mount without a router.
        go: go || (async (path) => (await import('@/router')).default.push(path)),
      })
      if (res?.pipeline) {
        const pl = res.pipeline
        message.success(
          t('shell.commands.macro.pipelineStarted', { id: pl.id, project: pl.project }),
          {
            action: pl.url
              ? {
                  label: t('shell.commands.macro.open'),
                  run: () => window.open(pl.url, '_blank', 'noopener'),
                }
              : undefined,
          },
        )
      }
      if (res?.missedTags?.length)
        message.warning(t('shell.commands.macro.missedTags', { tags: res.missedTags.join(', ') }))
      return res
    } catch (e) {
      message.error(humanizeError(e))
      return null
    }
  }

  // ── editing (the «Команды» window) ──
  async function saveMacro(m, data) {
    const res = m?.id
      ? await wsApi.updateMacro(m.id, data)
      : await wsApi.createMacro(wsId.value, data)
    await load()
    return res.data
  }
  async function removeMacro(m) {
    await wsApi.deleteMacro(m.id)
    await load()
  }
  async function saveTemplate(tpl, data) {
    const res = tpl?.id
      ? await wsApi.updateTaskTemplate(tpl.id, data)
      : await wsApi.createTaskTemplate(wsId.value, data)
    await load()
    return res.data
  }
  async function removeTemplate(tpl) {
    await wsApi.deleteTaskTemplate(tpl.id)
    await load()
  }

  return {
    wsId,
    macros,
    templates,
    canManage,
    loaded,
    enabled,
    load,
    onEvent,
    run,
    blockedReason,
    saveMacro,
    removeMacro,
    saveTemplate,
    removeTemplate,
  }
})
