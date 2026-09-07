package website.msdnna.tessera.data.api

import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSession
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.X509TrustManager
import okhttp3.OkHttpClient

/**
 * Опциональное отключение проверки цепочки доверия TLS (#2896).
 *
 * Self-hosted Tessera часто стоит за сертификатом, который телефон не проверит:
 * свой CA, самоподпись, истёкшая цепочка. Для REST это видно сразу и лечится
 * сменой адреса на `http://`, а вот SFU конференции адрес получает **от
 * сервера** (`/conferences/{id}/token` отдаёт `wss://…`), поменять его на
 * телефоне нельзя — и звонок упирается в `CertPathValidatorException: Trust
 * anchor for certification path not found` уже внутри комнаты.
 *
 * Поэтому переключатель, а не «всегда доверять»:
 *
 * - **выключен (по умолчанию)** — билдер не трогаем вовсе. Это важнее, чем
 *   выглядит: на Android доверие по умолчанию собирает сама платформа
 *   (системные CA + `network_security_config` + добавленные пользователем),
 *   и любой наш «эквивалентный» `TrustManager` тихо разошёлся бы с ней;
 * - **включён** — цепочка и имя хоста не проверяются **ни в одном** клиенте
 *   приложения. Половинчатый режим («доверяем только конференции») выглядел бы
 *   безопаснее, будучи ровно так же уязвимым к подмене: тот, кто может
 *   подсунуть свой сертификат SFU, подсунет его и REST-ручке.
 *
 * Смена политики поднимает [generation]: клиенты живут в [Holder] и
 * пересобираются при первом же обращении после переключения, поэтому включать
 * и выключать можно на живом приложении, не перезапуская его.
 */
object TlsTrust {
    /** Не проверять цепочку доверия и имя хоста. Устанавливается через [set]. */
    @Volatile
    var insecure: Boolean = false
        private set

    /** Растёт на каждой смене политики — по нему [Holder] понимает, что протух. */
    @Volatile
    private var generation: Int = 0

    fun set(value: Boolean) {
        if (value == insecure) return
        synchronized(this) {
            if (value == insecure) return
            insecure = value
            generation += 1
        }
        // У Retrofit-клиента свой кэш, живущий по адресу сервера, — он про
        // смену политики не узнает сам.
        RetrofitClient.reset()
    }

    /**
     * Клиент, пересобираемый при смене политики.
     *
     * Кэш нужен по той же причине, по какой OkHttp просит переиспользовать
     * клиент: пул соединений и диспетчер живут в нём. Пересборка — только когда
     * [generation] сдвинулся, то есть ровно на переключении тумблера.
     */
    class Holder(private val tune: OkHttpClient.Builder.() -> Unit = {}) {
        @Volatile private var cached: OkHttpClient? = null

        @Volatile private var cachedGeneration: Int = -1

        fun get(): OkHttpClient {
            val current = cached
            if (current != null && cachedGeneration == generation) return current
            return synchronized(this) {
                val stillCurrent = cached
                if (stillCurrent != null && cachedGeneration == generation) {
                    stillCurrent
                } else {
                    val at = generation
                    OkHttpClient.Builder().apply(tune).applyTrustPolicy().build().also {
                        cached = it
                        cachedGeneration = at
                    }
                }
            }
        }
    }

    /** Принимает любую цепочку. Пустой `acceptedIssuers` — то, чего ждёт JSSE
     *  от клиентского менеджера: список CA, которые мы предъявляем серверу. */
    internal val TRUST_ALL: X509TrustManager = object : X509TrustManager {
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) = Unit

        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) = Unit

        override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
    }

    /** Имя хоста тоже не проверяется: у самоподписанного сертификата SAN обычно
     *  не совпадает с адресом, под которым сервер реально живёт, и проверка имени
     *  завалила бы соединение уже после того, как цепочку мы пропустили. */
    internal val ALLOW_ANY_HOSTNAME = HostnameVerifier { _: String?, _: SSLSession? -> true }

    internal fun trustAllSocketFactory(): SSLSocketFactory =
        SSLContext.getInstance("TLS").apply { init(null, arrayOf(TRUST_ALL), SecureRandom()) }
            .socketFactory
}

/**
 * Применяет текущую политику доверия. При выключенном переключателе не делает
 * ничего — клиент остаётся ровно тем, что собрал бы OkHttp сам.
 */
fun OkHttpClient.Builder.applyTrustPolicy(): OkHttpClient.Builder = apply {
    if (!TlsTrust.insecure) return@apply
    sslSocketFactory(TlsTrust.trustAllSocketFactory(), TlsTrust.TRUST_ALL)
    hostnameVerifier(TlsTrust.ALLOW_ANY_HOSTNAME)
}
