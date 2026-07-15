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

    @Suppress("ReturnCount")
    private fun tryRefresh(baseUrl: String): String? {
        val current = refreshToken
        if (current.isBlank()) return null
        synchronized(refreshLock) {
            if (refreshToken != current) return authToken.ifBlank { null }
            val client = refreshHttpClient ?: return null
            val bodyJson = gson.toJson(RefreshRequest(current))
            val req = Request.Builder()
                .url(baseUrl + "auth/refresh")
                .post(bodyJson.toRequestBody("application/json".toMediaType()))
                .build()
            return try {
                client.newCall(req).execute().use { resp ->
                    if (!resp.isSuccessful) return null
                    val payload = resp.body.string()
                    if (payload.isBlank()) return null
                    val parsed = gson.fromJson(payload, AuthResponse::class.java) ?: return null
                    if (parsed.accessToken.isBlank()) return null
                    authToken = parsed.accessToken
                    if (parsed.refreshToken.isNotBlank()) refreshToken = parsed.refreshToken
                    onTokensRefreshed?.invoke(parsed.accessToken, parsed.refreshToken)
                    parsed.accessToken
                }
            } catch (_: IOException) {
                null
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
                    val newAccess = tryRefresh(baseUrl)
                    if (newAccess != null) {
                        response.close()
                        response = chain.proceed(chain.request().withAuth(newAccess))
                    } else {
                        authToken = ""
                        refreshToken = ""
                        onUnauthorized?.invoke()
                    }
                }
                response
            }
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(15, TimeUnit.SECONDS)
            .callTimeout(60, TimeUnit.SECONDS)
            .build()

        refreshHttpClient = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(15, TimeUnit.SECONDS)
            .callTimeout(20, TimeUnit.SECONDS)
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
