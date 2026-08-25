package website.msdnna.tessera

import android.app.Application
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.decode.SvgDecoder
import coil.disk.DiskCache
import coil.memory.MemoryCache
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.reminders.ReminderNotifications

class TesseraApplication :
    Application(),
    ImageLoaderFactory {
    override fun onCreate() {
        super.onCreate()
        AppContainer.init(this)
        // Канал на языке профиля — язык читается из DataStore, поэтому не в onCreate,
        // а в фоне: имя существующего канала система обновит следующим вызовом.
        AppContainer.appScope.launch { ReminderNotifications.ensureChannel(this@TesseraApplication) }
    }

    // Coil singleton that forwards the current Bearer token so auth-protected
    // media (avatars, uploaded attachments) loads without 401s.
    override fun newImageLoader(): ImageLoader {
        val client = OkHttpClient.Builder()
            .addInterceptor { chain ->
                val token = RetrofitClient.authToken
                val req = if (token.isNotBlank()) {
                    chain.request().newBuilder().addHeader("Authorization", "Bearer $token").build()
                } else {
                    chain.request()
                }
                chain.proceed(req)
            }
            .build()
        return ImageLoader.Builder(this)
            .okHttpClient(client)
            .components { add(SvgDecoder.Factory()) }
            .memoryCache { MemoryCache.Builder(this).maxSizePercent(0.10).build() }
            .diskCache {
                DiskCache.Builder()
                    .directory(cacheDir.resolve("media"))
                    .maxSizeBytes(16L * 1024 * 1024)
                    .build()
            }
            .build()
    }
}
