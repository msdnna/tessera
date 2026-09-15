package website.msdnna.tessera.ui

import android.content.res.Resources
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalResources
import java.text.NumberFormat

/**
 * Текст, который ViewModel отдаёт экрану, не резолвя его сама.
 *
 * У ViewModel нет `LocalResources`, поэтому готовая строка в состоянии застывает
 * на языке, который стоял в момент её появления: переключение языка в профиле
 * перерисовывает экран, но не пересоздаёт состояние — и тост «Сохранено» остался
 * бы русским на английском интерфейсе.
 *
 * Три случая: [Res] — наш текст, живёт в ресурсах и резолвится в композиции;
 * [Raw] — текст, пришедший с сервера (предупреждение канала, редкая ошибка без
 * известного кода), который локализовать нечем и незачем; [Decimal] — число,
 * которое нельзя сложить в строку заранее по той же причине, что и текст.
 */
sealed interface UiText {
    /** [args] — позиционные подстановки ресурса (`%1$d` в «Ошибка 500»). Список, а
     *  не `vararg`: массив в data-классе сравнивается по ссылке, и два одинаковых
     *  состояния перестали бы быть равными — лишняя рекомпозиция на ровном месте.
     *
     *  Элемент списка может сам быть [UiText] — тогда он резолвится теми же
     *  ресурсами перед подстановкой. Нужно для [Decimal]: `getString` форматирует
     *  `%1$f` локалью ресурсов, но число дробных знаков там вморожено в сам
     *  шаблон, а «4 МБ» и «1,5 МБ» — одна и та же строка. */
    data class Res(@StringRes val id: Int, val args: List<Any> = emptyList()) : UiText

    data class Raw(val text: String) : UiText

    /**
     * Дробное число, чей разделитель — дело локали ЧИТАТЕЛЯ, а не автора кода.
     *
     * Ровно то же правило, что у [website.msdnna.tessera.util.attachmentSizeText]
     * и соседей по вопросу «сколько весит файл»: русскому интерфейсу нужна
     * запятая, английскому точка. Форматируется в момент резолва, а не в
     * ViewModel, — там локаль профиля ещё не видна, и `Locale.US` посреди русской
     * фразы был именно этим.
     *
     * [maxFractionDigits] — потолок, а не норма: целое печатается целым («4», а не
     * «4,0»), лишний ноль соврал бы про точность.
     */
    data class Decimal(val value: Double, val maxFractionDigits: Int = 1) : UiText
}

/** Резолв в композиции — там, где язык профиля уже подставлен [AppLocale]. */
@Composable
fun UiText.resolve(): String = resolve(LocalResources.current)

/** То же вне композиции — с явными [res] (уведомления, тесты). */
fun UiText.resolve(res: Resources): String = when (this) {
    is UiText.Res -> res.getString(id, *args.map { it.resolveArg(res) }.toTypedArray())

    is UiText.Raw -> text

    is UiText.Decimal -> NumberFormat.getInstance(res.configuration.locales[0]).apply {
        maximumFractionDigits = this@resolve.maxFractionDigits
        minimumFractionDigits = 0
    }.format(value)
}

/** Подстановка ресурса: вложенный [UiText] резолвится, всё прочее едет как есть. */
private fun Any.resolveArg(res: Resources): Any = if (this is UiText) resolve(res) else this
