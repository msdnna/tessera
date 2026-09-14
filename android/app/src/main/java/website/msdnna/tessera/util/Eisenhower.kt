package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Task

/*
 * Квадрант матрицы Эйзенхауэра — порт `frontend/src/components/BoardMatrixView.vue`
 * (derive) и `frontend/src/utils/taskFilter.js` (isDueWithinDays).
 *
 * Вынесено из BoardViews.kt в util не ради красоты: ось «Срочно» — это окно по
 * календарным дням пользователя, то есть та же машинерия, которой считает фасет
 * «Срок» ([dueDayKey] / [dayKeyIn]), и её надо было не переписывать заново, а
 * позвать. Чистые функции — значит граница окна покрывается тестом, а не глазами.
 */

/** Ширина окна срочности по умолчанию — неделя, как у веба (`urgentWindowDays`). */
const val EISENHOWER_URGENT_DAYS = 7

/**
 * Попадает ли срок в ближайшие [days] КАЛЕНДАРНЫХ дней зоны пользователя.
 *
 * Полуоткрытое [сегодня, сегодня+days), просроченное считается попавшим: то, что
 * нужно было сделать вчера, не менее срочно того, что нужно завтра. При days = 7
 * это ровно окно фасета «Срок: неделя» ([matchesDue] с [DueFilter.Week]) — в этом
 * и смысл общей функции.
 *
 * Раньше матрица спрашивала `due <= System.currentTimeMillis() + days * 86_400_000L`,
 * то есть сравнивала ИНСТАНТЫ. Задача со сроком ровно через семь дней попадала в
 * «Срочно», как только её время суток оказывалось раньше текущего, — а у date-only
 * срока (UTC-полночь: issue из GitLab, срок «на весь день») это выполнено всегда.
 * Тот же фасет «неделя» на той же доске, плитка «На неделе» на главной и SQL-счётчик
 * сервера при этом отвечали «не на этой неделе».
 */
fun isDueWithinDays(
    dueDate: String?,
    days: Int,
    fmt: DateFormatPrefs = DateFormatPrefs.Default,
    nowMillis: Long = System.currentTimeMillis(),
): Boolean {
    val due = dueDayKey(dueDate, fmt)
    return due.isNotEmpty() && due < dayKeyIn(fmt.zone, days, nowMillis)
}

/**
 * Квадрант без ручного закрепления: важность — из приоритета (высокий/срочный),
 * срочность — из близости срока. Кодировка совпадает с бэкендом (0 — срочно+важно,
 * 1 — важно, 2 — срочно, 3 — ни то ни другое; миграция 0028).
 */
fun deriveQuadrant(
    task: Task,
    fmt: DateFormatPrefs = DateFormatPrefs.Default,
    nowMillis: Long = System.currentTimeMillis(),
): Int {
    val important = task.priority >= 3
    val urgent = isDueWithinDays(task.dueDate, EISENHOWER_URGENT_DAYS, fmt, nowMillis)
    return if (important) (if (urgent) 0 else 1) else (if (urgent) 2 else 3)
}

/** Закреплённый квадрант задачи, иначе выведенный ([deriveQuadrant]). */
fun quadrantOf(
    task: Task,
    fmt: DateFormatPrefs = DateFormatPrefs.Default,
    nowMillis: Long = System.currentTimeMillis(),
): Int = task.eisenhowerQuadrant ?: deriveQuadrant(task, fmt, nowMillis)
