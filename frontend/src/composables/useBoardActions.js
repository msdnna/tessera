import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  ArchiveOutline,
  LogoGitlab,
  PricetagsOutline,
  SettingsOutline,
  Star,
  StarOutline,
  TrailSignOutline,
} from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import { useFavoritesStore } from '@/stores/favorites'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useMessage } from '@/composables/useMessage'
import { humanizeError } from '@/utils/errors'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useConflictsStore } from '@/stores/conflicts'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'

// The board's own actions (#3112) — the project's tags, stages and settings,
// the board's GitLab and its archive. One list for the «⋯ Действия доски»
// button of the desktop sub-toolbar and the phone's board menu (BoardMobileMenu);
// rows in kit/ActionMenu's shape.
export function useBoardActions() {
  const { t } = useI18n()
  const store = useBoardViewStore()
  const projectWindow = useProjectWindowStore()
  const dialogs = useWsDialogsStore()
  const conflicts = useConflictsStore()
  const favorites = useFavoritesStore()
  const ws = useWorkspacesStore()
  const message = useMessage()
  const route = useRoute()
  const router = useRouter()

  const isArchive = computed(() => isArchiveQuery(route.query))

  // The open board in / out of the reader's favourites (#3115) — the crumbs'
  // star on a desktop; on a phone the header has no room for it, so this row
  // is where the star lives there.
  const isFavorite = computed(() => favorites.isFavorite(store.boardId))
  async function toggleFavorite() {
    if (!store.board?.id) return
    const project = ws.projects.find((p) => p.id === store.projectId) || null
    try {
      await favorites.toggle(store.board, project)
    } catch (e) {
      message.error(humanizeError(e))
    }
  }

  const items = computed(() => [
    {
      key: 'favorite',
      label: t(isFavorite.value ? 'board.crumbs.unfavorite' : 'board.crumbs.favorite'),
      icon: isFavorite.value ? Star : StarOutline,
      iconGrad: isFavorite.value,
      keys: 'board.favorite',
      testid: 'board-action-favorite',
    },
    { gap: true },
    // «Теги», «Этапы» and the settings open the «Проект» window (#2971) on
    // their tab — full screen on a phone.
    { key: 'tags', label: t('board.actions.tags'), icon: PricetagsOutline },
    { key: 'stages', label: t('board.actions.milestones'), icon: TrailSignOutline },
    {
      key: 'project',
      label: t('project.menu.settings'),
      icon: SettingsOutline,
      keys: 'project.window',
    },
    { gap: true },
    {
      key: 'gitlab',
      label: 'GitLab',
      icon: LogoGitlab,
      ...(conflicts.count
        ? { note: conflicts.count > 9 ? '9+' : String(conflicts.count), noteWarn: true }
        : {}),
    },
    // The archive is a read-only scope of the board itself (?archived=1): the
    // row is a toggle, its check mark the scope being on.
    {
      key: 'archive',
      label: t('board.actions.archive'),
      icon: ArchiveOutline,
      checked: isArchive.value,
    },
  ])

  // Leaving the archive replaces rather than pushes, same as the board's own
  // scope chip: the archive should not become a step in the back history of the
  // board it overlays (utils/boardScope).
  function toggleArchive() {
    if (isArchive.value) router.replace({ query: withoutArchiveQuery(route.query) })
    else router.push({ query: archiveQuery(route.query) })
  }

  // true when the key was one of these rows.
  function select(key) {
    if (key === 'favorite') toggleFavorite()
    else if (key === 'tags') projectWindow.open(store.projectId, 'tags')
    else if (key === 'stages') projectWindow.open(store.projectId, 'stages')
    else if (key === 'project') projectWindow.open(store.projectId, 'general')
    else if (key === 'gitlab') dialogs.open('gitlab')
    else if (key === 'archive') toggleArchive()
    else return false
    return true
  }

  return { items, select, isArchive }
}
