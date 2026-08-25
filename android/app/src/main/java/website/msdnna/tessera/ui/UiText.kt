package website.msdnna.tessera.ui

import android.content.res.Resources
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource

/**
 * Текст, который ViewModel отдаёт экрану, не резолвя его сама.
 *
 * У ViewModel нет `LocalResources`, поэтому готовая строка в состоянии застывает
 * на языке, который стоял в момент её появления: переключение языка в профиле
 * перерисовывает экран, но не пересоздаёт состояние — и тост «Сохранено» остался
 * бы русским на английском интерфейсе.
 *
 * Два случая, оба нужны: [Res] — наш текст, живёт в ресурсах и резолвится в
 * композиции; [Raw] — текст, пришедший с сервера (предупреждение канала, редкая
 * ошибка без известного кода), который локализовать нечем и незачем.
 */
sealed interface UiText {
    /** [args] — позиционные подстановки ресурса (`%1$d` в «Ошибка 500»). Список, а
     *  не `vararg`: массив в data-классе сравнивается по ссылке, и два одинаковых
     *  состояния перестали бы быть равными — лишняя рекомпозиция на ровном месте. */
    data class Res(@StringRes val id: Int, val args: List<Any> = emptyList()) : UiText

    data class Raw(val text: String) : UiText
}

/** Резолв в композиции — там, где язык профиля уже подставлен [AppLocale]. */
@Composable
fun UiText.resolve(): String = when (this) {
    is UiText.Res -> stringResource(id, *args.toTypedArray())
    is UiText.Raw -> text
}

/** То же вне композиции — с явными [res] (уведомления, тесты). */
fun UiText.resolve(res: Resources): String = when (this) {
    is UiText.Res -> res.getString(id, *args.toTypedArray())
    is UiText.Raw -> text
}
