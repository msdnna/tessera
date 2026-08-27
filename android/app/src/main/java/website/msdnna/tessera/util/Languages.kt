package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Configuration
import java.util.Locale

/**
 * Языки интерфейса, которые приложение везёт с собой (`res/values` = ru,
 * `res/values-en` = en). Список зеркалит неймспейсы `frontend/src/locales` — язык един
 * для всех клиентов, потому что живёт в профиле, а не в устройстве.
 */
val SupportedLanguages = listOf("ru", "en")

/** Язык по умолчанию: базовая локаль ресурсов и фолбэк для всего незнакомого. */
const val DEFAULT_LANGUAGE = "ru"

/**
 * Приводит значение `user_preferences.language` к одному из [SupportedLanguages].
 *
 * Значение приходит с сервера и пишется любым клиентом, так что здесь бывает и
 * `en-US`, и пустая строка, и язык, которого у нас нет. Незнакомое — это [DEFAULT_LANGUAGE]:
 * показать интерфейс на русском заведомо лучше, чем упасть или показать ключи ресурсов.
 */
fun normalizeLanguage(value: String?): String {
    val tag = value?.trim()?.lowercase(Locale.ROOT).orEmpty()
    if (tag.isEmpty()) return DEFAULT_LANGUAGE
    val base = tag.substringBefore('-').substringBefore('_')
    return if (base in SupportedLanguages) base else DEFAULT_LANGUAGE
}

/**
 * Контекст с ресурсами на [language] — им резолвятся строки вне Compose
 * (уведомления, тосты, будущие виджеты), где нет `LocalResources`.
 *
 * Язык приложения намеренно **не** привязан к системной локали: телефон на
 * английском и веб на русском разъехались бы, а источник истины один —
 * профиль пользователя.
 */
fun Context.withLanguage(language: String): Context {
    val locale = Locale.forLanguageTag(normalizeLanguage(language))
    val config = Configuration(resources.configuration)
    config.setLocale(locale)
    config.setLayoutDirection(locale)
    return createConfigurationContext(config)
}
