import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  ArchiveOutline,
  LogoGitlab,
  PricetagsOutline,
  SettingsOutline,
  TrailSignOutline,
} from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
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
  const route = useRoute()
  const router = useRouter()

  const isArchive = computed(() => isArchiveQuery(route.query))

  const items = computed(() => [
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
    if (key === 'tags') projectWindow.open(store.projectId, 'tags')
    else if (key === 'stages') projectWindow.open(store.projectId, 'stages')
    else if (key === 'project') projectWindow.open(store.projectId, 'general')
    else if (key === 'gitlab') dialogs.open('gitlab')
    else if (key === 'archive') toggleArchive()
    else return false
    return true
  }

  return { items, select, isArchive }
}
