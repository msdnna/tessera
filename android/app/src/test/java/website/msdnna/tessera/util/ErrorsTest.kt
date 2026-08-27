package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import java.io.IOException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import retrofit2.HttpException
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.ui.resolve

/**
 * Ошибки вызовов API едут до экрана как [UiText] (#2803, волна 12), поэтому тест
 * идёт под Robolectric и проверяет обе локали: подстановка кода в «Ошибка %1$d»
 * и наличие английского перевода иначе ломались бы только в рантайме.
 */
@RunWith(RobolectricTestRunner::class)
class ErrorsTest {
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val ru: Resources get() = res("ru")
    private val en: Resources get() = res("en")

    private fun httpException(code: Int, body: String): HttpException {
        val raw = Response.Builder()
            .code(code)
            .message("err")
            .protocol(Protocol.HTTP_1_1)
            .request(Request.Builder().url("http://localhost/").build())
            .build()
        val errBody = body.toResponseBody("application/json".toMediaType())
        return HttpException(retrofit2.Response.error<Any>(errBody, raw.newBuilder().code(code).build()))
    }

    // ── humanizeError ────────────────────────────────────────────────────────
    @Test
    fun `humanizeError null and blank yield null`() {
        assertThat(humanizeError(null)).isNull()
        assertThat(humanizeError("")).isNull()
        assertThat(humanizeError("   ")).isNull()
    }

    @Test
    fun `humanizeError maps sentinels case-insensitively`() {
        assertThat(humanizeError("invalid credentials")?.resolve(ru)).isEqualTo("Неверный email или пароль")
        assertThat(humanizeError("INVALID CREDENTIALS")?.resolve(ru)).isEqualTo("Неверный email или пароль")
        assertThat(humanizeError("email already registered")?.resolve(ru)).isEqualTo("Этот email уже зарегистрирован")
        assertThat(humanizeError("forbidden")?.resolve(ru)).isEqualTo("Недостаточно прав")
    }

    @Test
    fun `humanizeError sentinels speak English too`() {
        assertThat(humanizeError("invalid credentials")?.resolve(en)).isEqualTo("Wrong email or password")
        assertThat(humanizeError("forbidden")?.resolve(en)).isEqualTo("Not enough permissions")
    }

    @Test
    fun `humanizeError validator email`() {
        val raw = "Key: 'Req.Email' Error:Field validation for 'Email' failed on the 'email' tag"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Введите корректный email")
    }

    @Test
    fun `humanizeError validator required only`() {
        val raw = "Key: 'Req.Name' Error:Field validation for 'Name' failed on the 'required' tag"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Заполните все обязательные поля")
    }

    @Test
    fun `humanizeError validator min`() {
        val raw = "Key: 'Req.Pw' Error:Field validation for 'Pw' failed on the 'min' tag"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Значение слишком короткое")
    }

    @Test
    fun `humanizeError validation without recognisable tag falls back`() {
        val raw = "Key: 'X' validation something weird"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Проверьте правильность заполнения полей")
    }

    @Test
    fun `humanizeError network hints`() {
        assertThat(humanizeError("dial tcp: connection timeout")?.resolve(ru))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
        assertThat(humanizeError("econnrefused 127.0.0.1")?.resolve(ru))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
        assertThat(humanizeError("unable to resolve host example.com")?.resolve(ru))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
    }

    /** Незнакомое сообщение сервера — [UiText.Raw]: переводить его нечем, и в
     *  обеих локалях оно должно доехать до экрана дословно. */
    @Test
    fun `humanizeError unknown short message passes through as raw`() {
        val out = humanizeError("Что-то пошло не так")
        assertThat(out).isEqualTo(UiText.Raw("Что-то пошло не так"))
        assertThat(out?.resolve(en)).isEqualTo("Что-то пошло не так")
    }

    // ── errorMessage ─────────────────────────────────────────────────────────
    @Test
    fun `errorMessage IOException`() {
        assertThat(errorMessage(IOException("boom")).resolve(ru)).isEqualTo("Нет соединения с сервером")
        assertThat(errorMessage(IOException("boom")).resolve(en)).isEqualTo("No connection to the server")
    }

    @Test
    fun `errorMessage generic throwable humanized`() {
        assertThat(errorMessage(RuntimeException("user not found")).resolve(ru)).isEqualTo("Пользователь не найден")
        assertThat(errorMessage(RuntimeException()).resolve(ru)).isEqualTo("Неизвестная ошибка")
    }

    @Test
    fun `errorMessage HttpException with error envelope`() {
        val ex = httpException(400, """{"error":"invalid credentials"}""")
        assertThat(errorMessage(ex).resolve(ru)).isEqualTo("Неверный email или пароль")
    }

    @Test
    fun `errorMessage HttpException with empty envelope falls back to code`() {
        val ex = httpException(500, """{"error":""}""")
        assertThat(errorMessage(ex).resolve(ru)).isEqualTo("Ошибка 500")
        assertThat(errorMessage(httpException(500, """{"error":""}""")).resolve(en)).isEqualTo("Error 500")
    }

    @Test
    fun `errorMessage HttpException with non-json body falls back to code`() {
        val ex = httpException(503, "gateway down")
        assertThat(errorMessage(ex).resolve(ru)).isEqualTo("Ошибка 503")
    }

    // ── rawErrorText ─────────────────────────────────────────────────────────

    /** По сырому тексту ветвится логика (приглашение вместо добавления участника),
     *  и он обязан остаться английским ответом сервера, а не переводом. */
    @Test
    fun `rawErrorText keeps the server wording`() {
        val ex = httpException(404, """{"error":"no user with this email"}""")
        assertThat(rawErrorText(ex)).isEqualTo("no user with this email")
        assertThat(rawErrorText(RuntimeException("boom"))).isEqualTo("boom")
        assertThat(rawErrorText(httpException(500, "gateway down"))).isNull()
    }

    // ── isAuthError ──────────────────────────────────────────────────────────
    @Test
    fun `isAuthError only for 401 and 403`() {
        assertThat(isAuthError(httpException(401, "{}"))).isTrue()
        assertThat(isAuthError(httpException(403, "{}"))).isTrue()
        assertThat(isAuthError(httpException(500, "{}"))).isFalse()
        assertThat(isAuthError(IOException())).isFalse()
    }
}
