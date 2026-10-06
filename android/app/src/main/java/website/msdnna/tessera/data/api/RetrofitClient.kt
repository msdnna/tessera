package website.msdnna.tessera.data.api

import com.google.gson.Gson
import java.io.IOException
import java.lang.reflect.Type
import java.util.concurrent.TimeUnit
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.ResponseBody
import retrofit2.Converter
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import website.msdnna.tessera.data.model.AuthResponse
import website.msdnna.tessera.data.model.RefreshRequest

/**
 * Ответил ли сервер на `/auth/refresh` вердиктом о сессии, а не «не могу сейчас».
 *
 * Правило списано с веба (`frontend/src/api/index.js` refreshFailure, #2750):
 * вердикт — только клиентские коды, и то без 408/429, которые говорят про момент,
 * а не про сессию. Всё остальное (5xx, в том числе 503 «storage unavailable» из
 * бэкендового Refresh) означает, что ответа по существу не было.
 */
internal fun isSessionVerdict(code: Int): Boolean =
    code in 400..499 && code != 408 && code != 429

/**
 * Single owner of the Retrofit/OkHttp stack. Mirrors the web client's auth
 * behaviour (`frontend/src/api/index.js`): Bearer access token on every
 * request, silent refresh-on-401 with a coalesced single in-flight refresh,
 * and a logout signal when refresh is no longer possible.
 *
 * Online-first: there is no offline cache or reachability gate — calls go to
 * the network and surface failures to the caller.
 */
object RetrofitClient {
    @Volatile private var service: ApiService? = null

    @Volatile private var currentBaseUrl: String = ""

    @Volatile var authToken: String = ""

    @Volatile var refreshToken: String = ""

    /** Invoked on an OkHttp background thread when the session is unrecoverable. */
    @Volatile var onUnauthorized: (() -> Unit)? = null

    /** Invoked after a successful silent refresh so the host can persist the pair. */
    @Volatile var onTokensRefreshed: ((access: String, refresh: String) -> Unit)? = null

    private val refreshLock = Any()
    private val gson = Gson()

    @Volatile private var refreshHttpClient: OkHttpClient? = null

    private fun Request.withAuth(token: String): Request =
        if (token.isNotBlank()) newBuilder().header("Authorization", "Bearer $token").build() else this

    /**
     * Чем закончилась попытка обменять refresh-строку на новую пару.
     *
     * Разница между двумя видами неудачи — не косметическая: [NoSession] ведёт на
     * экран входа, [Unavailable] обязан оставить сессию нетронутой.
     */
    private sealed interface RefreshOutcome {
        /** Новая пара получена; внутри — свежий access-токен. */
        data class Fresh(val access: String) : RefreshOutcome

        /** Сервер вынес вердикт: этой сессии больше нет. */
        data object NoSession : RefreshOutcome

        /** Ответа по существу не было — сеть, таймаут, 5xx, мусор в теле. */
        data object Unavailable : RefreshOutcome
    }

    @Suppress("ReturnCount")
    private fun tryRefresh(baseUrl: String): RefreshOutcome {
        val current = refreshToken
        if (current.isBlank()) return RefreshOutcome.NoSession
        synchronized(refreshLock) {
            if (refreshToken != current) {
                // Пока мы ждали замок, обновился кто-то другой: его результат и есть наш.
                return authToken.ifBlank { null }
                    ?.let { RefreshOutcome.Fresh(it) } ?: RefreshOutcome.Unavailable
            }
            val client = refreshHttpClient ?: return RefreshOutcome.Unavailable
            val bodyJson = gson.toJson(RefreshRequest(current))
            val req = Request.Builder()
                .url(baseUrl + "auth/refresh")
                .post(bodyJson.toRequestBody("application/json".toMediaType()))
                .build()
            return try {
                client.newCall(req).execute().use { resp ->
                    if (!resp.isSuccessful) {
                        return if (isSessionVerdict(resp.code)) {
                            RefreshOutcome.NoSession
                        } else {
                            RefreshOutcome.Unavailable
                        }
                    }
                    val payload = resp.body.string()
                    if (payload.isBlank()) return RefreshOutcome.Unavailable
                    val parsed = gson.fromJson(payload, AuthResponse::class.java)
                        ?: return RefreshOutcome.Unavailable
                    if (parsed.accessToken.isBlank()) return RefreshOutcome.Unavailable
                    authToken = parsed.accessToken
                    if (parsed.refreshToken.isNotBlank()) refreshToken = parsed.refreshToken
                    onTokensRefreshed?.invoke(parsed.accessToken, parsed.refreshToken)
                    RefreshOutcome.Fresh(parsed.accessToken)
                }
            } catch (_: IOException) {
                RefreshOutcome.Unavailable
            }
        }
    }

    fun getService(serverUrl: String): ApiService {
        val apiUrl = buildApiUrl(serverUrl)
        if (service == null || apiUrl != currentBaseUrl) {
            synchronized(this) {
                if (service == null || apiUrl != currentBaseUrl) {
                    currentBaseUrl = apiUrl
                    service = buildService(apiUrl)
                }
            }
        }
        return service!!
    }

    /** Server root without the trailing `/api/`. Used to expand relative media URLs. */
    val serverRoot: String
        get() = currentBaseUrl.removeSuffix("/").removeSuffix("/api")

    fun reset() {
        synchronized(this) { service = null }
    }

    /**
     * Trades the refresh token for a fresh access token, right now.
     *
     * Blocking, so it belongs on an IO thread. Exists for the embedded document
     * editor (#2894 §4): that page holds an access token of ours and no refresh
     * token of its own, so when its token expires mid-edit the *app* is the only
     * one who can produce a new one. Returns null when there is no session left
     * to refresh — the caller then has nothing to hand over.
     */
    fun refreshAccess(): String? {
        val baseUrl = currentBaseUrl
        if (baseUrl.isBlank()) return null
        return (tryRefresh(baseUrl) as? RefreshOutcome.Fresh)?.access
    }

    private fun buildApiUrl(url: String): String {
        val trimmed = url.trimEnd('/')
        return if (trimmed.endsWith("/api")) "$trimmed/" else "$trimmed/api/"
    }

    /** Public `<server>/api/` base for a server URL — used to build the OAuth
     *  authorize URL opened in a Custom Tab (which bypasses Retrofit). */
    fun apiBaseUrl(serverUrl: String): String = buildApiUrl(serverUrl)

    private fun buildService(baseUrl: String): ApiService {
        val client = OkHttpClient.Builder()
            .addInterceptor { chain ->
                val firstReq = chain.request().withAuth(authToken)
                var response = chain.proceed(firstReq)
                if (response.code == 401 && !firstReq.url.encodedPath.endsWith("/auth/refresh")) {
                    when (val outcome = tryRefresh(baseUrl)) {
                        is RefreshOutcome.Fresh -> {
                            response.close()
                            response = chain.proceed(chain.request().withAuth(outcome.access))
                        }

                        RefreshOutcome.NoSession -> {
                            authToken = ""
                            refreshToken = ""
                            onUnauthorized?.invoke()
                        }

                        // Обновиться не вышло, но и вердикта не было: сессия жива,
                        // просто сейчас недоступна. Выбросить человека на экран
                        // входа из-за мигнувшей сети — это и есть та регрессия,
                        // которую веб чинил в #2750. Запрос уходит вызывающему
                        // своей ошибкой, токены остаются на месте.
                        RefreshOutcome.Unavailable -> Unit
                    }
                }
                response
            }
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(15, TimeUnit.SECONDS)
            .callTimeout(60, TimeUnit.SECONDS)
            .applyTrustPolicy()
            .build()

        refreshHttpClient = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(15, TimeUnit.SECONDS)
            .callTimeout(20, TimeUnit.SECONDS)
            .applyTrustPolicy()
            .build()

        val gson = GsonConverterFactory.create()
        return Retrofit.Builder()
            .baseUrl(baseUrl)
            .client(client)
            // Coalesce a JSON `null`/empty body into an empty list for List
            // endpoints — the Go backend serialises empty slices as `null`, and
            // Retrofit (erasing Kotlin nullability) would otherwise throw
            // "response body was null but declared non-null". Must precede gson.
            .addConverterFactory(NullSafeListConverterFactory(gson))
            .addConverterFactory(gson)
            .build()
            .create(ApiService::class.java)
    }

    /** Wraps the gson body converter so a `List` response is never null. */
    private class NullSafeListConverterFactory(
        private val delegate: Converter.Factory,
    ) : Converter.Factory() {
        override fun responseBodyConverter(
            type: Type,
            annotations: Array<out Annotation>,
            retrofit: Retrofit,
        ): Converter<ResponseBody, *>? {
            if (!List::class.java.isAssignableFrom(getRawType(type))) return null
            val inner = delegate.responseBodyConverter(type, annotations, retrofit) ?: return null
            return Converter<ResponseBody, Any> { body -> inner.convert(body) ?: emptyList<Any>() }
        }
    }
}
