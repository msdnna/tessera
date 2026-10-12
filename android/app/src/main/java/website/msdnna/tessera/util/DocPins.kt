package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.WorkspaceMilestone

/*
 * Чипы над текстом документа (#3032): этап, к которому он привязан (#3044), и
 * встреча, повесткой которой он служит (#3046). Паритет web `DocPins.vue`, но
 * только для чтения: привязку меняют в веб-клиенте (там пикеры у чипов и пункт
 * «Привязать к встрече» в меню документа), а телефон показывает, к чему документ
 * относится.
 *
 * В самом документе — только id; подписи берутся из списков пространства (этапы,
 * встречи), которые грузятся, лишь когда документу есть что показать.
 */

/** Подписи чипов открытого документа; null — чипа нет. */
data class DocPins(
    val stage: String? = null,
    /** Проект этапа — второй строкой не рисуется, но нужен для подсказки. */
    val stageProject: String? = null,
    val meeting: String? = null,
) {
    val isEmpty: Boolean get() = stage == null && meeting == null
}

/**
 * Собирает подписи по id документа и спискам пространства. Привязка к этапу или
 * встрече, которой в списке нет (удалена, другой список ещё не загружен), чипа
 * не даёт — пустой чип без названия хуже, чем никакого.
 */
fun docPins(doc: Document?, stages: List<WorkspaceMilestone>, meetings: List<Conference>): DocPins {
    if (doc == null) return DocPins()
    val stage = doc.milestoneId?.let { id -> stages.firstOrNull { it.id == id } }
    val meeting = doc.conferenceId?.let { id -> meetings.firstOrNull { it.id == id } }
    return DocPins(
        stage = stage?.title,
        stageProject = stage?.projectName?.ifBlank { null },
        meeting = meeting?.title,
    )
}

/** Нужно ли вообще идти за списками — у документа есть хоть одна привязка. */
fun docHasPins(doc: Document?): Boolean = doc?.milestoneId != null || doc?.conferenceId != null
