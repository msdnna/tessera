package website.msdnna.tessera.update

import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import com.google.gson.Gson
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.AppContainer
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
    private val http by lazy { OkHttpClient() }
    private val gson = Gson()

    /** `<server>/apks` — derived from the configured server URL (sans `/api`). */
    private fun apksBase(): String =
        AppContainer.serverUrl.trimEnd('/').removeSuffix("/api").trimEnd('/') + "/apks"

    /** The newer release if one is available for this build, else null. */
    suspend fun checkForUpdate(): LatestRelease? = withContext(Dispatchers.IO) {
        val latest = fetchLatest() ?: return@withContext null
        if (latest.versionCode > BuildConfig.VERSION_CODE && latest.apk.isNotBlank()) latest else null
    }

    private fun fetchLatest(): LatestRelease? = runCatching {
        val req = Request.Builder().url("${apksBase()}/latest.json").build()
        http.newCall(req).execute().use { resp ->
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
        val dir = File(cacheDir, "updates").apply { mkdirs() }
        dir.listFiles()?.forEach { it.delete() } // keep only the current download
        val out = File(dir, release.apk)
        val req = Request.Builder().url("${apksBase()}/${release.apk}").build()
        http.newCall(req).execute().use { resp ->
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
