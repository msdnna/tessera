import { useI18n } from 'vue-i18n'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useUndoStore } from '@/stores/undo'
import { useToast } from '@/composables/useToast'
import { humanizeError } from '@/utils/errors'

// The project archive (#3005, migration 0086). «Архивировать» (E in the
// project's menu, «Опасная зона» of the «Проект» window) happens at once and
// offers its undo in a toast — «Проект „Сайт“ в архиве», mockup CatalogA. The
// project leaves the tree, search and «Мои задачи» but stays readable: it opens
// on view only from «Архив проектов» at the foot of the tree, where
// «Восстановить» brings it back. Owner/admin only (the server checks too).
export function useProjectArchive() {
  const { t } = useI18n()
  const store = useWorkspacesStore()
  const undo = useUndoStore()
  const message = useToast()

  async function archiveProject(project) {
    const name = project.name
    try {
      await projApi.archive(project.id)
    } catch (e) {
      message?.error(humanizeError(e))
      return false
    }
    await store.refresh()
    undo.push({
      icon: 'archive',
      text: () => t('project.archive.done', { name }),
      doneText: () => t('project.archive.undone', { name }),
      undo: async () => {
        await projApi.unarchive(project.id)
        await store.refresh()
      },
    })
    return true
  }

  async function restoreProject(project) {
    try {
      await projApi.unarchive(project.id)
    } catch (e) {
      message?.error(humanizeError(e))
      return false
    }
    await store.refresh()
    message?.success(t('project.archive.restored', { name: project.name }))
    return true
  }

  return { archiveProject, restoreProject }
}
