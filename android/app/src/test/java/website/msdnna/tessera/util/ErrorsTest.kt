package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.io.IOException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Test
import retrofit2.HttpException

class ErrorsTest {
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
        assertThat(humanizeError("invalid credentials")).isEqualTo("Неверный email или пароль")
        assertThat(humanizeError("INVALID CREDENTIALS")).isEqualTo("Неверный email или пароль")
        assertThat(humanizeError("email already registered")).isEqualTo("Этот email уже зарегистрирован")
        assertThat(humanizeError("forbidden")).isEqualTo("Недостаточно прав")
    }

    @Test
    fun `humanizeError validator email`() {
        val raw = "Key: 'Req.Email' Error:Field validation for 'Email' failed on the 'email' tag"
        assertThat(humanizeError(raw)).isEqualTo("Введите корректный email")
    }

    @Test
    fun `humanizeError validator required only`() {
        val raw = "Key: 'Req.Name' Error:Field validation for 'Name' failed on the 'required' tag"
        assertThat(humanizeError(raw)).isEqualTo("Заполните все обязательные поля")
    }

    @Test
    fun `humanizeError validator min`() {
        val raw = "Key: 'Req.Pw' Error:Field validation for 'Pw' failed on the 'min' tag"
        assertThat(humanizeError(raw)).isEqualTo("Значение слишком короткое")
    }

    @Test
    fun `humanizeError validation without recognisable tag falls back`() {
        val raw = "Key: 'X' validation something weird"
        assertThat(humanizeError(raw)).isEqualTo("Проверьте правильность заполнения полей")
    }

    @Test
    fun `humanizeError network hints`() {
        assertThat(humanizeError("dial tcp: connection timeout"))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
        assertThat(humanizeError("econnrefused 127.0.0.1"))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
        assertThat(humanizeError("unable to resolve host example.com"))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
    }

    @Test
    fun `humanizeError unknown short message passes through`() {
        assertThat(humanizeError("Что-то пошло не так")).isEqualTo("Что-то пошло не так")
    }

    // ── errorMessage ─────────────────────────────────────────────────────────
    @Test
    fun `errorMessage IOException`() {
        assertThat(errorMessage(IOException("boom"))).isEqualTo("Нет соединения с сервером")
    }

    @Test
    fun `errorMessage generic throwable humanized`() {
        assertThat(errorMessage(RuntimeException("user not found"))).isEqualTo("Пользователь не найден")
        assertThat(errorMessage(RuntimeException())).isEqualTo("Неизвестная ошибка")
    }

    @Test
    fun `errorMessage HttpException with error envelope`() {
        val ex = httpException(400, """{"error":"invalid credentials"}""")
        assertThat(errorMessage(ex)).isEqualTo("Неверный email или пароль")
    }

    @Test
    fun `errorMessage HttpException with empty envelope falls back to code`() {
        val ex = httpException(500, """{"error":""}""")
        assertThat(errorMessage(ex)).isEqualTo("Ошибка 500")
    }

    @Test
    fun `errorMessage HttpException with non-json body falls back to code`() {
        val ex = httpException(503, "gateway down")
        assertThat(errorMessage(ex)).isEqualTo("Ошибка 503")
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
