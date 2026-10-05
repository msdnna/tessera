import { computed, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, useMessage } from 'naive-ui'
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

const icon = (c, color) => () => h(NIcon, color ? { color } : null, { default: () => h(c) })

// An orange count pill after a menu label — unresolved GitLab conflicts.
function withCount(label, count) {
  if (!count) return label
  return () =>
    h('span', { style: 'display:inline-flex;align-items:center;gap:8px' }, [
      label,
      h(
        'span',
        {
          style:
            'min-width:18px;height:18px;padding:0 5px;border-radius:9px;display:inline-flex;' +
            'align-items:center;justify-content:center;font-size:11px;font-weight:600;' +
            'background:#e0922f;color:#fff;',
        },
        count > 9 ? '9+' : String(count),
      ),
    ])
}

// The workspace menu (v2): what used to be the tool row under the workspace
// name — members, integrations — plus the workspace's own actions (appearance,
// create, delete). One source for the sidebar header and the rail tile's menu.
//
// `workspace` is a getter; for a workspace other than the current one (a rail
// tile's right-click) only the actions that don't need it to be open remain.
export function useWorkspaceMenu(workspace = null) {
  const { t } = useI18n()
  const message = useMessage()
  const ws = useWorkspacesStore()
  const auth = useAuthStore()
  const conflicts = useConflictsStore()
  const dialogs = useWsDialogsStore()

  const target = computed(() => (workspace ? workspace() : ws.current))
  const isCurrent = computed(() => !!target.value && target.value.id === ws.currentId)

  const options = computed(() => {
    const w = target.value
    if (!w) return []
    const manage = ['owner', 'admin'].includes(w.my_role)
    const out = []
    if (isCurrent.value) {
      out.push({ label: t('shell.tools.members'), key: 'members', icon: icon(PeopleOutline) })
      out.push({
        label: withCount(t('shell.tools.integrations'), conflicts.count),
        key: 'integrations',
        icon: icon(ExtensionPuzzleOutline),
        children: [
          { label: withCount('GitLab', conflicts.count), key: 'gitlab', icon: icon(LogoGitlab) },
          { label: t('shell.tools.estimation'), key: 'estimation', icon: icon(TimerOutline) },
          // The dictionary is workspace settings: owner/admin only (the PUT is
          // gated by requireManager server-side, this only hides a door that
          // would 403).
          ...(ws.commandsCanManage
            ? [{ label: t('shell.tools.commands'), key: 'commands', icon: icon(TerminalOutline) }]
            : []),
        ],
      })
    }
    if (manage)
      out.push({
        label: t('shell.rail.appearance'),
        key: 'appearance',
        icon: icon(ColorPaletteOutline),
      })
    if (isCurrent.value) {
      out.push({ type: 'divider', key: 'd1' })
      out.push({ label: t('shell.workspace.create'), key: 'create', icon: icon(AddOutline) })
    }
    if (w.owner_id === auth.user?.id) {
      out.push({
        label: t('shell.workspace.delete'),
        key: 'delete',
        icon: icon(TrashOutline, '#e0533d'),
        props: { style: 'color:#e0533d' },
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
    dialogs.open(key, isCurrent.value ? null : target.value)
  }

  return { options, select }
}
