package website.msdnna.tessera.data

import android.content.Context
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.api.ApiService
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.preferences.AppPreferences
import website.msdnna.tessera.util.DEFAULT_LANGUAGE
import website.msdnna.tessera.util.normalizeLanguage

/**
 * Minimal manual DI. Holds the singletons the app needs and resolves the
 * active [ApiService] from the current server URL. Initialised once in
 * [website.msdnna.tessera.TesseraApplication].
 */
object AppContainer {
    lateinit var prefs: AppPreferences
        private set

    @Volatile
    var serverUrl: String = BuildConfig.DEFAULT_BASE_URL

    /** Живёт столько же, сколько процесс: сюда уходят фоновые мелочи без своего
     *  владельца — создание канала уведомлений на старте, показ пуша из ресивера. */
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    fun init(context: Context) {
        prefs = AppPreferences(context.applicationContext)
    }

    /** The Retrofit service bound to the current [serverUrl]. */
    fun api(): ApiService = RetrofitClient.getService(serverUrl)

    /**
     * Язык интерфейса для кода вне Compose. Значение живёт в DataStore, то есть
     * читается только из корутины: у уведомлений и будильников нет `LocalResources`,
     * зато есть suspend-контекст.
     *
     * Падать здесь нечему: до инициализации контейнера (или на пустом DataStore)
     * это русский — та же форма падения, что и у [normalizeLanguage].
     */
    suspend fun language(): String =
        runCatching { prefs.language.first() }.getOrDefault(DEFAULT_LANGUAGE)
}
