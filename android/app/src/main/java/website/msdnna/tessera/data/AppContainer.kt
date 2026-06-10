package website.msdnna.tessera.data

import android.content.Context
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.api.ApiService
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.preferences.AppPreferences

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

    fun init(context: Context) {
        prefs = AppPreferences(context.applicationContext)
    }

    /** The Retrofit service bound to the current [serverUrl]. */
    fun api(): ApiService = RetrofitClient.getService(serverUrl)
}
