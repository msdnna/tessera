import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useMessage } from '@/composables/useMessage'
import { columnName } from '@/utils/defaultNames'

// «Задача #N создана» (#3117, PackageI «Успех: тост или правка на месте»): a
// success toast is shown only where the result is not on screen — the task
// went to a board the reader is not looking at. It names where it went and
// opens it.
export function useTaskCreatedToast() {
  const { t } = useI18n()
  const router = useRouter()
  const message = useMessage()
  return function taskCreated(task, { project, board, column } = {}) {
    if (!task) return
    const where = [project?.name, board?.name, column ? columnName(column) : '']
      .filter(Boolean)
      .join(' · ')
    message.success(t('toasts.created', { n: task.number }), {
      line: where,
      action: {
        label: () => t('toasts.act.open'),
        run: () => router.push(`/board/${task.board_id || board?.id}?task=${task.number}`),
      },
    })
  }
}
