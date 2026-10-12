package website.msdnna.tessera.data

import android.content.Context
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.api.ApiService
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.preferences.AppPreferences
import website.msdnna.tessera.util.DEFAULT_LANGUAGE
import website.msdnna.tessera.util.normalizeLanguage
import website.msdnna.tessera.util.resolvedTimezone

/**
 * Minimal manual DI. Holds the singletons the app needs and resolves the
 * active [ApiService] from the current server URL. Initialised once in
 * [website.msdnna.tessera.TesseraApplication].
 */
object AppContainer {
    lateinit var prefs: AppPreferences
        private set

    /**
     * The application context, for the few things that need one outside a screen:
     * the LiveKit room and the call's foreground service (#2896 §4). An Activity
     * context would not do — a call outlives the screen that started it.
     */
    lateinit var appContext: Context
        private set

    @Volatile
    var serverUrl: String = BuildConfig.DEFAULT_BASE_URL

    /** Живёт столько же, сколько процесс: сюда уходят фоновые мелочи без своего
     *  владельца — создание канала уведомлений на старте, показ пуша из ресивера. */
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    fun init(context: Context) {
        appContext = context.applicationContext
        prefs = AppPreferences(context.applicationContext)
        // The profile zone, kept in memory for [callerZone] (see there).
        appScope.launch {
            runCatching { prefs.preferences.map { it.timezone }.collect { profileZone = it } }
        }
    }

    /** The profile's timezone as last read from DataStore; null until the first read. */
    @Volatile
    private var profileZone: String? = null

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

    /**
     * Зона для `?tz=` на ручках, которые сервер считает по дням (#2932): преф
     * профиля, иначе зона устройства (см. [resolvedTimezone]). `null` — «сказать
     * нечего»: пустой параметр зоной не является и только стоил бы серверу
     * следующего источника, поэтому Retrofit его не отправляет.
     */
    // Read from memory, not from DataStore: a write path (posting a comment) used to
    // wait on `preferences.first()` before its request even left — under the
    // Robolectric e2e tier that read never came back until the test ended. Before
    // the first read lands this is the device zone, which is what an empty profile
    // zone means anyway.
    fun callerZone(): String? = resolvedTimezone(profileZone).ifEmpty { null }
}
