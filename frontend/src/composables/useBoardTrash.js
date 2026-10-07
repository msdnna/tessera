import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { boards as boardsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useUndoStore } from '@/stores/undo'
import { useToast } from '@/composables/useToast'
import { humanizeError } from '@/utils/errors'

// useBoardTrash deletes a board the v2 way (#2969): at once, with an undo toast.
// The server keeps the board in its trash (migration 0078), so the undo is a
// real restore. A board deleted while open sends the reader home; undoing
// brings them back to it.
export function useBoardTrash() {
  const { t } = useI18n()
  const route = useRoute()
  const router = useRouter()
  const store = useWorkspacesStore()
  const undo = useUndoStore()
  const message = useToast()

  function isOpen(project, board) {
    return route.params.projectSlug === project.slug && route.params.boardSlug === board.slug
  }

  async function removeBoard(project, board) {
    const wasOpen = isOpen(project, board)
    const path = `/project/${project.slug}/board/${board.slug}`
    const name = board.name
    try {
      await boardsApi.remove(board.id)
    } catch (e) {
      message?.error(humanizeError(e))
      return false
    }
    if (wasOpen) router.push('/')
    await store.loadBoards(project.id)
    undo.push({
      icon: 'trash',
      text: () => t('project.board.deleted', { name }),
      doneText: () => t('project.board.restored', { name }),
      errorText: () => t('project.board.restoreFailed'),
      undo: async () => {
        await boardsApi.restore(board.id)
        await store.loadBoards(project.id)
        if (wasOpen) router.push(path)
      },
    })
    return true
  }

  return { removeBoard }
}
