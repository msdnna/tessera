package website.msdnna.tessera.data.api

import com.google.common.truth.Truth.assertThat
import javax.net.ssl.SSLSession
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Test

/**
 * Политика доверия TLS (#2896).
 *
 * Проверять здесь стоит ровно две вещи, и обе невидимы на ревью:
 *
 * - **выключенный тумблер не трогает клиент.** Тот, кому мы «на всякий случай»
 *   подставили свой менеджер доверия, вёл бы себя точно так же в обычной сети —
 *   и молча перестал бы уважать `network_security_config` и добавленные
 *   пользователем CA. Сломать это можно одной строкой, а заметить нельзя;
 * - **включённый действительно пропускает** и негодную цепочку, и несовпадающее
 *   имя хоста. Половина (только цепочка) выглядит рабочей ровно до встречи с
 *   самоподписанным сертификатом, у которого SAN не тот, — то есть до первого
 *   реального применения.
 */
class TlsTrustTest {
    @After
    fun reset() {
        TlsTrust.set(false)
    }

    @Test
    fun `secure by default`() {
        assertThat(TlsTrust.insecure).isFalse()
    }

    @Test
    fun `a secure client keeps okhttp's own verifier`() {
        TlsTrust.set(false)
        val client = OkHttpClient.Builder().applyTrustPolicy().build()
        assertThat(client.hostnameVerifier).isNotSameInstanceAs(TlsTrust.ALLOW_ANY_HOSTNAME)
    }

    @Test
    fun `an insecure client accepts any host name`() {
        TlsTrust.set(true)
        val client = OkHttpClient.Builder().applyTrustPolicy().build()
        assertThat(client.hostnameVerifier).isSameInstanceAs(TlsTrust.ALLOW_ANY_HOSTNAME)
        // Живого рукопожатия нет, сессии тоже — этот проверяющий на неё и не смотрит.
        assertThat(client.hostnameVerifier.verify("sfu.example.invalid", null as SSLSession?)).isTrue()
    }

    @Test
    fun `the permissive trust manager passes an unrooted chain`() {
        // Ровно то, на чём падал звонок: цепочка, которой некуда упереться.
        TlsTrust.TRUST_ALL.checkServerTrusted(emptyArray(), "RSA")
        assertThat(TlsTrust.TRUST_ALL.acceptedIssuers).isEmpty()
    }

    @Test
    fun `a held client is rebuilt when the policy flips`() {
        val holder = TlsTrust.Holder()
        TlsTrust.set(false)
        val secure = holder.get()
        assertThat(holder.get()).isSameInstanceAs(secure)

        TlsTrust.set(true)
        val insecure = holder.get()
        assertThat(insecure).isNotSameInstanceAs(secure)
        assertThat(insecure.hostnameVerifier).isSameInstanceAs(TlsTrust.ALLOW_ANY_HOSTNAME)

        // И обратно: тумблер должен не только чинить, но и переставать чинить —
        // иначе выключение оставляет приложение доверчивым до перезапуска.
        TlsTrust.set(false)
        assertThat(holder.get().hostnameVerifier).isNotSameInstanceAs(TlsTrust.ALLOW_ANY_HOSTNAME)
    }
}
