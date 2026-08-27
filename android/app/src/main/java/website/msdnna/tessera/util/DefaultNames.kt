package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Workspace

/*
 * Подписи для имён, которые рождаются на сервере: личное пространство, засеваемое
 * при регистрации, и четыре колонки каждой новой доски.
 *
 * Раньше они приезжали готовой русской строкой — переводить на клиенте было нечего,
 * и на английском интерфейсе доска оставалась русской. Теперь рядом с именем едет
 * стабильный ключ `name_key` (#2800), а подпись собирается здесь из ресурсов
 * читателя. Строка остаётся фолбэком: ключ, которого этот APK ещё не знает (сервер
 * новее клиента), читается фразой, а не голым ключом, — тот же приём, что в
 * Errors.kt и в notify.Sentence на бэкенде.
 *
 * Имя, которое задал пользователь, ключа не имеет — сервер гасит ключ при
 * переименовании, — поэтому показывается как есть на любом языке. В этом и весь
 * смысл различия: «Бэклог» не должен превращаться в «To do» при смене языка.
 *
 * Resources приходит явным параметром (как в Dates.kt и Estimation.kt): подписи
 * собираются и вне композиции — дорожки доски строятся внутри `remember`, — а язык
 * там берётся из тех же ресурсов, что подменил AppLocale.
 */

private val WORKSPACE_NAMES = mapOf(
    "personal" to R.string.default_workspace_personal,
)

private val COLUMN_NAMES = mapOf(
    "todo" to R.string.default_column_todo,
    "in_progress" to R.string.default_column_in_progress,
    "review" to R.string.default_column_review,
    "done" to R.string.default_column_done,
)

private fun caption(res: Resources, table: Map<String, Int>, key: String?, fallback: String): String {
    val id = key?.let { table[it] } ?: return fallback
    return res.getString(id)
}

/** Подпись пространства: по ключу, иначе — присланное имя. */
fun workspaceCaption(res: Resources, ws: Workspace?): String =
    if (ws == null) "" else caption(res, WORKSPACE_NAMES, ws.nameKey, ws.name)

/** Подпись колонки доски: по ключу, иначе — присланное имя. */
fun columnCaption(res: Resources, col: BoardColumn?): String =
    if (col == null) "" else caption(res, COLUMN_NAMES, col.nameKey, col.name)

/**
 * Значок статуса колонки: ключ говорит, какая это из четырёх засеянных, не
 * заглядывая в имя. Колонка без ключа (её завёл или переименовал пользователь)
 * по-прежнему опознаётся по словам в имени — web `columnStatus.js` паритет.
 */
fun isReviewColumn(nameKey: String?, name: String): Boolean =
    if (nameKey != null) nameKey == "review" else REVIEW_RE.containsMatchIn(name)

/**
 * Слова, по которым колонка без ключа считается «на рассмотрении».
 *
 * Русские слова здесь — данные, а не интерфейс: это то, как названы строки в БД
 * (в языке, на котором пространство завели), поэтому таблица не переезжает в
 * ресурсы и помечена `i18n-data`.
 */
private val REVIEW_RE = Regex("рассмотр|ревью|review|проверк", RegexOption.IGNORE_CASE) // i18n-data
