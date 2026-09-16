package website.msdnna.tessera.data.model

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test

/**
 * `has_password` на модели профиля (#2932).
 *
 * У аккаунта, заведённого через GitLab, хеша пароля нет, и смена пароля может
 * ответить ему только «текущий пароль неверен» — форма непроходима, а отказ
 * читается как опечатка. Экран настроек прячет её по этому флагу, поэтому важны
 * обе стороны разбора: `false` должен доехать, а ОТСУТСТВИЕ поля — дать `true`,
 * иначе профиль, закешированный сборкой постарше, отберёт форму у аккаунта с
 * паролем. Второе держится на том, что все поля [User] имеют значения по
 * умолчанию: Kotlin порождает беспараметрический конструктор, и Gson берёт его
 * вместо обхода через Unsafe (при котором значения по умолчанию не применяются
 * и вышло бы ровно `false`).
 */
class UserHasPasswordTest {
    private val gson = Gson()

    @Test
    fun `an account provisioned through gitlab reports no password`() {
        val json = """
            {"id":"u-1","email":"oauth@example.com","name":"Мария","provider":"gitlab",
             "email_verified":true,"has_password":false}
        """.trimIndent()

        val user = gson.fromJson(json, User::class.java)

        assertThat(user.hasPassword).isFalse()
        assertThat(user.provider).isEqualTo("gitlab")
    }

    @Test
    fun `an ordinary account reports a password`() {
        val json = """{"id":"u-2","email":"local@example.com","has_password":true}"""

        assertThat(gson.fromJson(json, User::class.java).hasPassword).isTrue()
    }

    @Test
    fun `a profile cached before the field existed keeps its form`() {
        val json = """{"id":"u-3","email":"cached@example.com","name":"Пётр"}"""

        assertThat(gson.fromJson(json, User::class.java).hasPassword).isTrue()
    }
}
