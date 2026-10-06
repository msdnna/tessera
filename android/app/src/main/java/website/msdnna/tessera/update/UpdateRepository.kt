package website.msdnna.tessera.update

import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import com.google.gson.Gson
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Request
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.TlsTrust
import website.msdnna.tessera.data.model.LatestRelease

/**
 * In-app self-update. The signed release APK + a `latest.json` manifest are
 * served at `<server>/apks/` by the frontend nginx; we compare the manifest's
 * versionCode to this build's, download the newer APK to the cache, and hand it
 * to the system package installer.
 *
 * Dev note: against a backend-only server (emulator `10.0.2.2`) there is no
 * `/apks/` — the check just fails quietly and no update is offered.
 */
object UpdateRepository {
    private val http = TlsTrust.Holder()
    private val gson = Gson()

    /** `<server>/apks` — derived from the configured server URL (sans `/api`). */
    private fun apksBase(): String =
        AppContainer.serverUrl.trimEnd('/').removeSuffix("/api").trimEnd('/') + "/apks"

    /** Столько символов хватает любому имени вида `tessera-1.2.3.apk`. */
    private const val MAX_APK_NAME = 128

    /**
     * Имя файла APK из манифеста — если ему можно верить, иначе null (#2932).
     *
     * `latest.json` целиком приходит с сервера, а его поле `apk` попадает не
     * только в URL, но и в `File(dir, …)`. Разделитель или `..` внутри уводят
     * запись из `cacheDir/updates` в любую точку приватного каталога
     * приложения — `../../shared_prefs/…`, `../../databases/…`, — и подпись APK
     * от этого не спасает: установщик до проверки просто не доходит, файл уже
     * лежит там, куда его попросили положить. То есть сервер (а при включённом
     * [TlsTrust.insecure] — и любой, кто встал между) получает запись в чужое
     * приватное хранилище, хотя обновить приложение чужой подписью не может.
     *
     * Поэтому имя принимается только как есть: один сегмент, без разделителей и
     * управляющих символов, не скрытый, с расширением `.apk`.
     */
    internal fun safeApkName(name: String): String? {
        val trimmed = name.trim()
        if (trimmed.isEmpty() || trimmed.length > MAX_APK_NAME) return null
        if (!trimmed.endsWith(".apk", ignoreCase = true)) return null
        if (trimmed.startsWith(".")) return null
        if (trimmed.any { it == '/' || it == '\\' || it.isISOControl() }) return null
        return trimmed
    }

    /** The newer release if one is available for this build, else null. */
    suspend fun checkForUpdate(): LatestRelease? = withContext(Dispatchers.IO) {
        val latest = fetchLatest() ?: return@withContext null
        // Манифест с негодным именем файла — не «обновление, которое не скачалось»,
        // а обновление, которого нет: предлагать его некуда.
        if (latest.versionCode > BuildConfig.VERSION_CODE && safeApkName(latest.apk) != null) latest else null
    }

    private fun fetchLatest(): LatestRelease? = runCatching {
        val req = Request.Builder().url("${apksBase()}/latest.json").build()
        http.get().newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return@use null
            val body = resp.body.string().takeIf { it.isNotBlank() } ?: return@use null
            gson.fromJson(body, LatestRelease::class.java)
        }
    }.getOrNull()

    /** Streams the release APK into `cacheDir/updates`, reporting 0f..1f progress. */
    suspend fun download(
        cacheDir: File,
        release: LatestRelease,
        onProgress: (Float) -> Unit,
    ): File = withContext(Dispatchers.IO) {
        // Второй рубеж: скачать могут попросить и минуя checkForUpdate — из
        // сайдбара по уже полученному релизу.
        val apkName = requireNotNull(safeApkName(release.apk)) { "unexpected apk name" }
        val dir = File(cacheDir, "updates").apply { mkdirs() }
        dir.listFiles()?.forEach { it.delete() } // keep only the current download
        val out = File(dir, apkName)
        val req = Request.Builder().url("${apksBase()}/$apkName").build()
        http.get().newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "HTTP ${resp.code}" }
            val body = resp.body
            val total = body.contentLength()
            body.byteStream().use { input ->
                out.outputStream().use { output ->
                    val buf = ByteArray(64 * 1024)
                    var downloaded = 0L
                    while (true) {
                        val read = input.read(buf)
                        if (read == -1) break
                        output.write(buf, 0, read)
                        downloaded += read
                        if (total > 0) onProgress((downloaded.toFloat() / total).coerceIn(0f, 1f))
                    }
                }
            }
        }
        out
    }

    /** Launches the system package installer for a downloaded APK. */
    fun install(context: Context, apk: File) {
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", apk)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        runCatching { context.startActivity(intent) }
    }
}
