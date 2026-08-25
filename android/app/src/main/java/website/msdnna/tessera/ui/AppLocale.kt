package website.msdnna.tessera.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import website.msdnna.tessera.util.normalizeLanguage
import website.msdnna.tessera.util.withLanguage

/**
 * Подставляет всему дереву ресурсы на языке из профиля: `stringResource` и
 * `pluralStringResource` читают [LocalResources], поэтому достаточно подменить
 * его (и [LocalConfiguration], чтобы локаль видели те, кто смотрит в конфиг).
 *
 * [LocalContext] намеренно оставлен как есть: `createConfigurationContext`
 * возвращает не Activity, а обёртку, и подмена тихо сломала бы места вида
 * `LocalContext.current as? Activity` (свернуть приложение по «назад»,
 * доступ к окну). Кому нужны строки вне Compose — берут
 * `Context.withLanguage()` явно.
 *
 * Смена языка перестраивает только композицию, без пересоздания Activity, —
 * состояние экрана и позиция скролла переживают переключение.
 */
@Composable
fun AppLocale(language: String, content: @Composable () -> Unit) {
    val base = LocalContext.current
    val tag = normalizeLanguage(language)
    val resources = remember(base, tag) { base.withLanguage(tag).resources }
    CompositionLocalProvider(
        LocalResources provides resources,
        LocalConfiguration provides resources.configuration,
        content = content,
    )
}
