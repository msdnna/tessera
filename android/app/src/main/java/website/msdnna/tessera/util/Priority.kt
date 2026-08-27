package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R

/*
 * Подписи приоритета задачи. Индекс 0..4 — тот же, что у `PriorityColors` в
 * ui/theme/Tokens.kt, и тот же, что приходит с сервера в поле `priority`.
 *
 * Строки лежат в `R.array.task_priority_labels`, а не списком в коде: список на
 * верхнем уровне вычисляется один раз при загрузке класса и застыл бы на языке
 * первого обращения — смена языка в профиле его бы не переехала. Внутри
 * композиции берите `stringArrayResource(R.array.task_priority_labels)`, здесь —
 * вариант для мест, где композиции нет (форматтеры журнала, разбор конфликтов).
 */

/** Подпись приоритета, либо null — индекс вне диапазона (вызывающий покажет сырое). */
fun priorityLabel(res: Resources, index: Int): String? =
    res.getStringArray(R.array.task_priority_labels).getOrNull(index)
