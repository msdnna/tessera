import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from '@/composables/useMessage'
import {
  ArchiveOutline,
  ArrowUndoOutline,
  EyeOffOutline,
  EyeOutline,
  LogoGitlab,
  TrashOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { useUndoStore } from '@/stores/undo'
import { deleteTask } from '@/utils/taskTrash'

// The «⋯» of the task modal's head (#3112): GitLab, the side panel, then the
// task's archive and delete — rows for kit/ActionMenu (TaskModalHeader draws
// them). «В архив» happens at once and closes the modal, its undo in a toast;
// a task with subtasks asks what to do with them in a submenu. «Удалить» sends
// it to the board's trash (#3004) the same way, with its own undo toast.
//
//   task             ref of the open task
//   readonly         getter — the archive view: «Вернуть из архива» instead
//   wide, paneHidden getters — the side panel's toggle is there only when wide
//   togglePane, restore, changed, close — the modal's own actions
export function useTaskModalMenu({
  task,
  readonly,
  restorable,
  wide,
  paneHidden,
  togglePane,
  restore,
  changed,
  close,
}) {
  const { t } = useI18n()
  const message = useMessage()
  const undo = useUndoStore()

  const items = computed(() => {
    const tk = task.value
    if (!tk) return []
    const out = []
    if (tk.gitlab)
      out.push({
        key: 'gitlab',
        label: t('task.head.openGitlab'),
        icon: LogoGitlab,
        note: `!${tk.gitlab.iid}`,
      })
    if (wide())
      out.push({
        key: 'pane',
        label: paneHidden() ? t('task.head.showPane') : t('task.head.hidePane'),
        icon: paneHidden() ? EyeOutline : EyeOffOutline,
      })
    if (readonly()) {
      // Only the task's own archive has a «Вернуть»; an archived project (#3005)
      // is restored from its own window.
      if (restorable?.() ?? true)
        out.push({ key: 'restore', label: t('task.action.restore'), icon: ArrowUndoOutline })
      return out
    }
    if (out.length) out.push({ gap: true })
    const subs = tk.subtasks?.length || 0
    out.push(
      subs
        ? {
            key: 'archive',
            label: t('task.action.archive'),
            icon: ArchiveOutline,
            subWidth: 240,
            children: [
              { key: 'archive:all', label: t('task.footer.archiveBoth'), icon: ArchiveOutline },
              {
                key: 'archive:detach',
                label: t('task.footer.detachSubtasks'),
                icon: ArrowUndoOutline,
              },
            ],
          }
        : { key: 'archive', label: t('task.action.archive'), icon: ArchiveOutline },
      { key: 'delete', label: t('task.action.delete'), icon: TrashOutline, danger: true },
    )
    return out
  })

  async function archive(detach) {
    const tk = task.value
    try {
      await tasksApi.archive(tk.id, detach ? { subtasks: 'detach' } : undefined)
    } catch (e) {
      message.error(e.message)
      return
    }
    changed()
    close()
    undo.push({
      icon: 'archive',
      text: { key: 'task.menu.archived', params: { title: tk.title || '' } },
      doneText: { key: 'task.menu.archivedUndone', params: { title: tk.title || '' } },
      undo: async () => {
        await tasksApi.restore(tk.id)
        changed()
      },
    })
  }

  // «Удалить» sends the task to the board's trash (#3004) at once; the undo is
  // in the toast, and the board's «Корзина» keeps it for 30 days.
  async function remove() {
    const tk = task.value
    try {
      await deleteTask(tk, { onChanged: changed })
      close()
    } catch (e) {
      message.error(e.message)
    }
  }

  function select(key) {
    const tk = task.value
    if (!tk) return
    if (key === 'gitlab') window.open(tk.gitlab.web_url, '_blank', 'noopener,noreferrer')
    else if (key === 'pane') togglePane()
    else if (key === 'restore') restore()
    else if (readonly()) return
    else if (key === 'archive' || key === 'archive:all') archive(false)
    else if (key === 'archive:detach') archive(true)
    else if (key === 'delete') remove()
  }

  return { items, select }
}
