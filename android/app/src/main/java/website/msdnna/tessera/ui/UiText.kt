package website.msdnna.tessera.ui

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
 * композиции; [Raw] — текст, пришедший с сервера (предупреждение канала), который
 * локализовать нечем и незачем.
 */
sealed interface UiText {
    data class Res(@StringRes val id: Int) : UiText

    data class Raw(val text: String) : UiText
}

/** Резолв в композиции — там, где язык профиля уже подставлен [AppLocale]. */
@Composable
fun UiText.resolve(): String = when (this) {
    is UiText.Res -> stringResource(id)
    is UiText.Raw -> text
}
