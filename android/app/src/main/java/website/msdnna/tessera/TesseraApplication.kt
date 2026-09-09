package website.msdnna.tessera

import android.app.Application
import android.os.Build
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.decode.GifDecoder
import coil.decode.ImageDecoderDecoder
import coil.decode.SvgDecoder
import coil.disk.DiskCache
import coil.memory.MemoryCache
import kotlinx.coroutines.launch
import okhttp3.Call
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.api.TlsTrust
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
        val client = TlsTrust.Holder {
            addInterceptor { chain ->
                val token = RetrofitClient.authToken
                val req = if (token.isNotBlank()) {
                    chain.request().newBuilder().addHeader("Authorization", "Bearer $token").build()
                } else {
                    chain.request()
                }
                chain.proceed(req)
            }
        }
        return ImageLoader.Builder(this)
            // Фабрика, а не клиент: загрузчик Coil — синглтон на весь процесс, и
            // клиент, вшитый в него один раз, пережил бы переключение «не проверять
            // сертификат» — аватарки продолжали бы падать до перезапуска (#2896).
            .callFactory(Call.Factory { request -> client.get().newCall(request) })
            .components {
                add(SvgDecoder.Factory())
                // Без явного декодера Coil разбирает GIF через BitmapFactory и
                // показывает первый кадр — статичную картинку вместо анимации
                // (#2894). ImageDecoder умеет ещё и анимированные WebP/AVIF, но
                // появился в API 28; ниже остаётся Movie-декодер, только GIF.
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    add(ImageDecoderDecoder.Factory())
                } else {
                    add(GifDecoder.Factory())
                }
            }
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
