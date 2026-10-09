import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from '@/composables/useMessage'
import {
  AddOutline,
  ColorPaletteOutline,
  ExtensionPuzzleOutline,
  LogoGitlab,
  PeopleOutline,
  TerminalOutline,
  TimerOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useConflictsStore } from '@/stores/conflicts'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { workspaceName } from '@/utils/defaultNames'

// The workspace menu (v2): what used to be the tool row under the workspace
// name — members, integrations — plus the workspace's own actions (appearance,
// create, delete). One source for the sidebar header and the rail tile's menu,
// both drawn by kit/ActionMenu (#3110): `header` is its title, `items` its rows.
//
// `workspace` is a getter; for a workspace other than the current one (a rail
// tile's right-click) only the actions that don't need it to be open remain.
// `anchor` (a getter) is the element the small dialogs open at (#2969).
export function useWorkspaceMenu(workspace = null, anchor = null) {
  const { t } = useI18n()
  const message = useMessage()
  const ws = useWorkspacesStore()
  const auth = useAuthStore()
  const conflicts = useConflictsStore()
  const dialogs = useWsDialogsStore()

  const target = computed(() => (workspace ? workspace() : ws.current))
  const isCurrent = computed(() => !!target.value && target.value.id === ws.currentId)
  const header = computed(() => (target.value ? workspaceName(target.value) : ''))

  // Unresolved GitLab conflicts: an orange count after «Интеграции» and GitLab.
  const conflictNote = computed(() =>
    conflicts.count
      ? { note: conflicts.count > 9 ? '9+' : String(conflicts.count), noteWarn: true }
      : {},
  )

  const items = computed(() => {
    const w = target.value
    if (!w) return []
    const manage = ['owner', 'admin'].includes(w.my_role)
    const out = []
    if (isCurrent.value) {
      out.push({ key: 'members', label: t('shell.tools.members'), icon: PeopleOutline })
      out.push({
        key: 'integrations',
        label: t('shell.tools.integrations'),
        icon: ExtensionPuzzleOutline,
        ...conflictNote.value,
        subWidth: 230,
        children: [
          { key: 'gitlab', label: 'GitLab', icon: LogoGitlab, ...conflictNote.value },
          // The unit is a workspace setting whose PUT is gated by requireManager
          // server-side (#2932): offer it only to those the endpoint will accept.
          ...(manage
            ? [{ key: 'estimation', label: t('shell.tools.estimation'), icon: TimerOutline }]
            : []),
          // The dictionary is workspace settings: owner/admin only (the PUT is
          // gated by requireManager server-side, this only hides a door that
          // would 403).
          ...(ws.commandsCanManage
            ? [{ key: 'commands', label: t('shell.tools.commands'), icon: TerminalOutline }]
            : []),
        ],
      })
    }
    if (manage)
      out.push({
        key: 'appearance',
        label: t('shell.rail.appearance'),
        icon: ColorPaletteOutline,
        keys: 'row.icon',
      })
    if (isCurrent.value) {
      out.push({ gap: true })
      out.push({
        key: 'create',
        label: t('shell.workspace.create'),
        icon: AddOutline,
        keys: 'row.new',
      })
    }
    // Not a two-step row: it opens the form that asks for the name
    // (rail/WorkspaceDeletePopover). The last workspace can't go.
    if (w.owner_id === auth.user?.id) {
      if (!isCurrent.value && out.length) out.push({ gap: true })
      out.push({
        key: 'delete',
        label: t('shell.workspace.delete'),
        icon: TrashOutline,
        danger: true,
        disabled: ws.list.length <= 1,
        disabledTitle: t('shell.workspace.deleteLast'),
      })
    }
    return out
  })

  function select(key) {
    if (key === 'delete' && ws.list.length <= 1) {
      message.warning(t('shell.workspace.deleteLast'))
      return
    }
    if (key === 'integrations') return
    dialogs.open(key, isCurrent.value ? null : target.value, anchor ? anchor() : null)
  }

  return { header, items, select }
}
