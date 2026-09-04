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
 * Язык интерфейса из трёх источников, по убыванию силы (веб-паритет, #2818/#2855):
 *
 * 1. [server] — `user_preferences.language` вошедшего пользователя. Профиль всегда
 *    сильнее устройства: язык един для всех клиентов и живёт в профиле.
 * 2. [device] — до-авторизационный выбор тумблером на экране входа. До логина
 *    профиля нет, а показать экран входа на понятном языке нужно уже сейчас.
 * 3. [system] — локаль телефона при первом запуске. Веб на этом месте берёт
 *    `navigator.languages` (`detectBrowserLocale`), а не молча предполагает русский.
 *
 * Пустая строка на любом уровне = «не выбрано», а не «выберите русский»: только так
 * системная локаль вообще может сработать. Незнакомый язык на КАЖДОМ уровне уводит
 * не в дефолт, а на следующий источник — телефон на немецком с пустым профилем
 * должен доехать до системного фолбэка, а не застрять на `de → ru`.
 *
 * Отличие от веба, осознанное: там `reset()` при выходе возвращает язык к системному,
 * потому что профиль и устройство делят один слот в localStorage и слот надо чистить.
 * Здесь слоты разные, поэтому выход из аккаунта возвращает ровно тот язык, который
 * человек выбрал на экране входа, — его выбор никто не отменял.
 */
fun resolveLanguage(server: String?, device: String?, system: String?): String {
    val pick = { value: String? ->
        val tag = value?.trim()?.lowercase(Locale.ROOT).orEmpty()
            .substringBefore('-').substringBefore('_')
        tag.takeIf { it in SupportedLanguages }
    }
    return pick(server) ?: pick(device) ?: pick(system) ?: DEFAULT_LANGUAGE
}

/** Следующий язык в цикле — тумблер на экране входа перебирает [SupportedLanguages]
 *  по кругу. На двух языках это кнопка-переключатель, а не список (веб-паритет). */
fun nextLanguage(current: String?): String {
    val idx = SupportedLanguages.indexOf(normalizeLanguage(current))
    return SupportedLanguages[(idx + 1) % SupportedLanguages.size]
}

/**
 * Контекст с ресурсами на [language] — им резолвятся строки вне Compose
 * (уведомления, тосты, будущие виджеты), где нет `LocalResources`.
 *
 * Язык приложения намеренно **не следует** за системной локалью: телефон на
 * английском и веб на русском разъехались бы, а источник истины один —
 * профиль пользователя. Системная локаль участвует ровно в одном месте —
 * как первичная догадка до логина, см. [resolveLanguage].
 */
fun Context.withLanguage(language: String): Context {
    val locale = Locale.forLanguageTag(normalizeLanguage(language))
    val config = Configuration(resources.configuration)
    config.setLocale(locale)
    config.setLayoutDirection(locale)
    return createConfigurationContext(config)
}
