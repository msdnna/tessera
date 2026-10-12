// What happened to a GitLab stage's write (#3010), told the same way on every
// surface that edits a stage — the stage page, the stage strip, the «Проект»
// window. The server writes to GitLab in the same request and answers gl_sync:
//   ok       — written; nothing to say
//   failed   — kept here, marked «not synced»; a toast says why
//   conflict — changed in GitLab too; the conflicts window opens on the stage
import { useI18n } from 'vue-i18n'
import { milestones as msApi } from '@/api'
import { useMessage } from '@/composables/useMessage'
import { useConflictsStore } from '@/stores/conflicts'
import { humanizeError } from '@/utils/errors'

export function useStageSync() {
  const { t } = useI18n()
  const message = useMessage()
  const conflicts = useConflictsStore()

  // report reads one PATCH/retry answer; returns the gl_sync status (or '').
  function report(data) {
    const sync = data?.gl_sync
    if (!sync) return ''
    if (sync.status === 'conflict') {
      message.warning(t('milestones.gitlab.conflictToast'))
      conflicts.load()
      conflicts.openResolver(null, { milestoneId: data.id })
    } else if (sync.status === 'failed') {
      message.warning(t('milestones.gitlab.failedToast', { error: sync.error || '' }))
    }
    return sync.status
  }

  // retry re-sends a refused write («повторить»).
  async function retry(stage) {
    try {
      const { data } = await msApi.retryGitlab(stage.id)
      const status = report(data)
      if (status === 'ok') message.success(t('milestones.gitlab.retried'))
      return data
    } catch (e) {
      message.error(humanizeError(e))
      return null
    }
  }

  // The conflicts window, focused on this stage.
  function resolve(stage) {
    conflicts.openResolver(null, { milestoneId: stage.id })
  }

  return { report, retry, resolve }
}
