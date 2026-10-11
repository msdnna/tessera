import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useHelpStore } from '@/stores/help'
import { usePaletteStore } from '@/stores/palette'
import { useInboxStore } from '@/stores/inbox'
import { useFavoritesStore } from '@/stores/favorites'
import { useMacrosStore } from '@/stores/macros'
import { macroKeyId } from '@/shortcuts/dynamic'
import { favoriteFirst } from '@/utils/favorites'
import { unwindOverlay } from '@/utils/overlayNav'
import { workspaceName } from '@/utils/defaultNames'
import { CATALOG } from '@/shortcuts/catalog'
import { isLive, runShortcut } from '@/shortcuts/dispatcher'
import { rank } from '@/utils/fuzzy'
import { milestoneKey } from '@/utils/milestones'
import { roleLabel } from '@/utils/mentions'
import { useFormat } from '@/composables/useFormat'
import { ICONS } from './icons'
import { taskStatus, taskPath, taskSub } from './rows'

// Prefix at the start of the query → the one section it narrows to.
export const PREFIXES = { '#': 'tasks', '@': 'people', '>': 'commands' }

export function parseQuery(q) {
  const s = q || ''
  const section = PREFIXES[s[0]]
  return section ? { section, term: s.slice(1).trim() } : { section: 'all', term: s.trim() }
}

// How many rows each section gets in the mixed «Всё» list; a prefix lifts it.
const LIMITS = { tasks: 6, boards: 5, commands: 4, people: 4, notes: 4, help: 3 }
const FULL = 25
const DEBOUNCE = 150

// The results list of the palette (v2, #2957): server search for tasks, boards,
// notes and documents; the workspace's milestones and members, filtered here;
// the shortcut catalog for commands; the bundled help index.
export function usePaletteResults() {
  const { t } = useI18n()
  const router = useRouter()
  const ws = useWorkspacesStore()
  const board = useBoardViewStore()
  const help = useHelpStore()
  const palette = usePaletteStore()
  const inbox = useInboxStore()
  const favorites = useFavoritesStore()
  const macros = useMacrosStore()
  const route = useRoute()

  // Where a palette form would create its thing — said under the command, as
  // the mockup does («В группе «Продукт» · пошагово, без мыши»).
  function formSub(form) {
    const project = ws.projects.find((p) => p.slug === route.params.projectSlug) || null
    if (form === 'project') {
      const group = ws.groups.find((g) => g.id === project?.group_id)
      return group
        ? t('shell.paletteForm.cmdSub.projectIn', { group: group.name })
        : t('shell.paletteForm.cmdSub.project')
    }
    if (form === 'board') {
      const target = project || ws.projects[0]
      return target ? t('shell.paletteForm.cmdSub.board', { project: target.name }) : ''
    }
    if (form === 'group')
      return t('shell.paletteForm.cmdSub.group', { space: workspaceName(ws.current) })
    return t('shell.paletteForm.cmdSub.workspace')
  }
  const { formatDate } = useFormat()

  // A typed prefix wins; otherwise the section pill the user clicked.
  const parsed = computed(() => {
    const p = parseQuery(palette.query)
    return p.section === 'all' && palette.section !== 'all' ? { ...p, section: palette.section } : p
  })
  const found = ref({ tasks: [], boards: [], notes: [], documents: [] })
  const loading = ref(false)
  const members = ref([])
  const milestones = ref([])
  // Bumped on every open: which commands are live depends on the screen.
  const openedAt = ref(0)

  let timer = 0
  let seq = 0

  // Reference lists, once per opening (both are shared, cached GETs).
  watch(
    () => palette.open,
    async (open) => {
      if (!open) return
      openedAt.value = Date.now()
      const wsId = ws.currentId
      if (!wsId) return
      const [m, ms] = await Promise.allSettled([wsApi.members(wsId), wsApi.milestones(wsId)])
      members.value = m.status === 'fulfilled' ? m.value.data || [] : []
      milestones.value = ms.status === 'fulfilled' ? ms.value.data || [] : []
    },
    { immediate: true },
  )

  const wantsServer = computed(() => {
    const { section, term } = parsed.value
    return !!term && ['all', 'tasks', 'boards'].includes(section)
  })

  watch(
    () => [parsed.value.term, wantsServer.value, palette.open],
    () => {
      clearTimeout(timer)
      if (!palette.open || !wantsServer.value) {
        found.value = { tasks: [], boards: [], notes: [], documents: [] }
        loading.value = false
        return
      }
      loading.value = true
      const term = parsed.value.term
      timer = setTimeout(() => search(term), DEBOUNCE)
    },
    { immediate: true },
  )

  async function search(term) {
    const wsId = ws.currentId
    if (!wsId) return
    const mine = ++seq
    try {
      const res = await wsApi.search(wsId, term.replace(/^#/, ''))
      if (mine !== seq) return
      found.value = {
        tasks: res.data.tasks || [],
        boards: res.data.boards || [],
        notes: res.data.notes || [],
        documents: res.data.documents || [],
      }
    } catch {
      if (mine === seq) found.value = { tasks: [], boards: [], notes: [], documents: [] }
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  // The palette owns a history entry (useOverlayBack): closing it after the
  // push let its `history.back()` cancel a push that was still resolving — a
  // board in another place stayed unopened (#3115). Close it first and wait
  // for the unwind (utils/overlayNav).
  async function go(path, ctx) {
    if (ctx?.newTab) {
      window.open(router.resolve(path).href, '_blank', 'noopener')
      return
    }
    await unwindOverlay(() => palette.hide())
    router.push(path)
  }

  function taskRow(task, hl) {
    return {
      key: `task:${task.id}`,
      status: taskStatus(task),
      title: task.title,
      hl,
      sub: taskSub(task),
      task,
      run: (ctx) => {
        palette.pushRecent(task)
        go(taskPath(task), ctx)
      },
    }
  }

  // The workspace's own commands (#3069, CmdsWin): found by their call (>bug)
  // and name, first in «Команды». A task command needs an open board.
  const MACRO_ICON = {
    create_task: 'AddOutline',
    open_link: 'LinkOutline',
    create_document: 'DocumentTextOutline',
    run_pipeline: 'GitBranchOutline',
  }
  const macroRows = computed(() => {
    void openedAt.value
    const term = parsed.value.term
    return rank(macros.enabled, (m) => [m.trigger, m.name], term).map((m) => {
      const why = macros.blockedReason(m)
      return {
        key: `macro:${m.id}`,
        icon: ICONS[MACRO_ICON[m.action]],
        title: m.name,
        hl: term,
        sub: `>${m.trigger} · ${t(`shell.commands.macro.action.${m.action}`)}`,
        keys: macroKeyId(m),
        disabled: !!why,
        hint: why
          ? t(why === 'soon' ? 'shell.commands.macro.soon' : 'shell.commands.macro.needBoard')
          : null,
        run: async (ctx) => {
          await macros.run(m, { go: (path) => go(path, ctx) })
        },
      }
    })
  })

  const commandRows = computed(() => [...macroRows.value, ...catalogRows.value])
  const catalogRows = computed(() => {
    void openedAt.value
    const term = parsed.value.term
    const live = CATALOG.filter((e) => e.command && (e.to || e.form || isLive(e.id)))
    const label = (e) => t(`shell.keys.${e.id}`)
    return rank(live, (e) => [label(e)], term).map((e) => ({
      key: `cmd:${e.id}`,
      icon: ICONS[e.command],
      title: label(e),
      hl: term,
      sub: e.form ? formSub(e.form) : t(`shell.keys.groups.${e.group || 'nav'}`),
      keys: e.keys || e.display ? e.id : null,
      form: e.form || null,
      run: (ctx) => {
        // A palette form takes over the palette itself (step mode).
        if (e.form) {
          palette.scope = { form: e.form, step: 0 }
          palette.query = ''
          return 'stay'
        }
        if (e.to) go(e.to, ctx)
        // After the palette has closed, so the command's own UI (a menu, a
        // dialog) doesn't open under a closing overlay.
        else setTimeout(() => runShortcut(e.id), 0)
      },
    }))
  })

  const peopleRows = computed(() => {
    const term = parsed.value.term
    return rank(members.value, (m) => [m.name, m.email], term).map((m) => ({
      key: `person:${m.user_id}`,
      avatar: m.user_id,
      title: m.name || m.email,
      hl: term,
      sub: [m.email, roleLabel(m.role)].filter(Boolean).join(' · '),
      disabled: !board.active,
      hint: board.active ? null : t('shell.palette.personNeedsBoard'),
      run: () => board.requestAssigneeFilter(m.user_id),
    }))
  })

  // Favourite boards (#3115): their own group while nothing is typed, and on
  // top of the found boards once something is — with the filled star.
  const favoriteRows = computed(() =>
    favorites.items.map((f) => ({
      key: `fav:${f.board_id}`,
      icon: ICONS.Star,
      grad: true,
      title: f.name,
      sub: `${t('shell.palette.board')} · ${f.project_name}`,
      run: (ctx) => go(`/project/${f.project_slug}/board/${f.slug}`, ctx),
    })),
  )

  const boardRows = computed(() => {
    const term = parsed.value.term
    const byFavorite = favoriteFirst(found.value.boards, favorites.isFavorite)
    const boards = byFavorite.map((b) => {
      const fav = favorites.isFavorite(b.id)
      return {
        key: `board:${b.id}`,
        icon: fav ? ICONS.Star : ICONS.GridOutline,
        grad: fav,
        title: b.name,
        hl: term,
        sub: `${t('shell.palette.board')} · ${b.project_name}`,
        run: (ctx) => go(`/project/${b.project_slug}/board/${b.slug}`, ctx),
      }
    })
    // Open milestones first; rank() keeps that order among equal scores.
    const ordered = [...milestones.value].sort(
      (a, b) => (a.state === 'closed') - (b.state === 'closed'),
    )
    const ms = rank(ordered, (m) => [m.title, m.project_name], term).map((m) => ({
      key: `ms:${m.id}`,
      icon: ICONS.TrailSignOutline,
      title: m.title,
      hl: term,
      sub: [
        t('shell.palette.milestone'),
        m.project_name,
        m.due_date
          ? t('shell.palette.until', {
              date: formatDate(m.due_date, { day: 'numeric', month: 'short' }),
            })
          : '',
      ]
        .filter(Boolean)
        .join(' · '),
      run: (ctx) =>
        m.project_slug && m.board_slug
          ? go(
              {
                path: `/project/${m.project_slug}/board/${m.board_slug}`,
                query: { milestone: milestoneKey(m) },
              },
              ctx,
            )
          : go('/milestones', ctx),
    }))
    // Without a query the milestones would flood the mixed list; the section
    // of its own lists them all.
    return term || parsed.value.section === 'boards' ? [...boards, ...ms] : []
  })

  const noteRows = computed(() => {
    const term = parsed.value.term
    return [
      ...found.value.notes.map((n) => ({
        key: `note:${n.id}`,
        icon: ICONS.DocumentTextOutline,
        title: n.title || t('shell.search.untitled'),
        hl: term,
        sub: t('shell.search.notes'),
        // Notes live in the «Входящие» panel (#2999): it opens on this one.
        run: () => inbox.reveal('note', n.id),
      })),
      ...found.value.documents.map((d) => ({
        key: `doc:${d.id}`,
        icon: ICONS.DocumentsOutline,
        title: d.title || t('shell.search.untitled'),
        hl: term,
        sub: t('shell.search.documents'),
        run: (ctx) => go(`/documents/${d.slug || d.id}`, ctx),
      })),
    ]
  })

  const helpRows = computed(() =>
    help.find(parsed.value.term, LIMITS.help).map((h) => ({
      key: `help:${h.slug}`,
      icon: ICONS.HelpCircleOutline,
      title: h.title,
      hl: parsed.value.term,
      sub: h.category,
      run: () => help.openDrawer(h.slug),
    })),
  )

  const groups = computed(() => {
    const { section, term } = parsed.value
    const all = section === 'all'
    const lim = (k) => (all ? LIMITS[k] : FULL)
    const out = []
    const add = (id, rows, limit = Infinity) => {
      if (rows.length)
        out.push({
          id,
          title: t(`shell.palette.groups.${id}`),
          count: rows.length,
          rows: rows.slice(0, limit),
        })
    }

    if (!term) {
      // Nothing typed: what the user touched last, then what they can do.
      if (all || section === 'tasks')
        add(
          'recent',
          palette.recent.map((r) => taskRow(r, null)),
        )
      if (all || section === 'boards')
        add('favorites', favoriteRows.value, all ? LIMITS.boards : FULL)
      if (all || section === 'commands') add('commands', commandRows.value, all ? 6 : FULL)
      if (section === 'people') add('people', peopleRows.value, FULL)
      if (section === 'boards') add('boards', boardRows.value, FULL)
      return out
    }
    if (all || section === 'tasks')
      add(
        'tasks',
        found.value.tasks.map((x) => taskRow(x, term)),
        lim('tasks'),
      )
    if (all || section === 'boards') add('boards', boardRows.value, lim('boards'))
    if (all || section === 'commands') add('commands', commandRows.value, lim('commands'))
    if (all || section === 'people') add('people', peopleRows.value, lim('people'))
    if (all) add('notes', noteRows.value, lim('notes'))
    if (all) add('help', helpRows.value, lim('help'))
    return out
  })

  return { parsed, groups, loading }
}
