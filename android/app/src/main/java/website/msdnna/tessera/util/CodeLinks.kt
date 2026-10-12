package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.CodeLink
import website.msdnna.tessera.data.model.TaskEventData

/*
 * «Разработка» окна задачи (#2964) — Android-зеркало веба
 * (`frontend/src/utils/codeLinks.js`). Сервер отдаёт связанные изменения кода
 * провайдеро-нейтрально: состояние (open | merged | closed) с флагом черновика и
 * статус последнего CI-прогона ('' | success | running | failed | canceled |
 * manual). Здесь — во что это превращается на экране: подпись и оттенок пилла,
 * подпись и оттенок строки пайплайна, строка веток и фраза истории.
 */

/** Пилл состояния: строка и оттенок; у черновика оттенка нет — он нейтральный. */
data class ChangeStatus(val label: Int, val hex: String?)

fun changeStatus(link: CodeLink): ChangeStatus = when {
    link.state == "merged" -> ChangeStatus(R.string.task_dev_state_merged, "#1f75cb")
    link.state == "closed" -> ChangeStatus(R.string.task_dev_state_closed, "#d03050")
    link.draft -> ChangeStatus(R.string.task_dev_state_draft, null)
    else -> ChangeStatus(R.string.task_dev_state_open, "#18a058")
}

/** Строка пайплайна: подпись, иконка из набора и оттенок (null — text3). */
data class CiStatus(val label: Int, val icon: String, val hex: String?)

fun ciStatus(status: String?): CiStatus? = when (status) {
    "success" -> CiStatus(R.string.task_dev_ci_success, Ion.CHECK_CIRCLE, "#18a058")
    "running" -> CiStatus(R.string.task_dev_ci_running, Ion.REFRESH, "#2f80ed")
    "failed" -> CiStatus(R.string.task_dev_ci_failed, Ion.WARNING, "#d03050")
    "canceled" -> CiStatus(R.string.task_dev_ci_canceled, Ion.CLOSE, null)
    "manual" -> CiStatus(R.string.task_dev_ci_manual, Ion.TIME, "#f0a020")
    else -> null
}

/** «feat/board-v2 → main»; сторону, которую GitLab не назвал, пропускаем. */
fun branchLine(link: CodeLink): String {
    val from = link.sourceBranch
    val to = link.targetBranch
    return when {
        from.isNotBlank() && to.isNotBlank() -> "$from → $to"
        else -> from.ifBlank { to }
    }
}

/** Фраза записи `code_change`: «открыт !418 в feat/board-v2», «смержен !418». */
fun codeChangeText(res: Resources, d: TaskEventData): String {
    val ref = "!${d.number ?: ""}"
    val branch = d.branch?.trim().orEmpty()
    return when {
        d.state == "merged" -> res.getString(R.string.task_event_code_merged, ref)
        d.state == "closed" -> res.getString(R.string.task_event_code_closed, ref)
        branch.isNotEmpty() -> res.getString(R.string.task_event_code_opened_on, ref, branch)
        else -> res.getString(R.string.task_event_code_opened, ref)
    }
}
