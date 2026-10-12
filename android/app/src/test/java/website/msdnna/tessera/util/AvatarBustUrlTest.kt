package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Метка, сбивающая кэш превью аватара (#2932).
 *
 * Разделитель здесь — не косметика. `avatar_url` приходит в двух видах: свой
 * загруженный аватар (`/api/users/<id>/avatar`, без строки запроса) и фолбэк на
 * гитлабовский, который сервер отдаёт подписанным прокси-адресом
 * (`/api/gitlab/avatar?ws=…&u=…&sig=<hmac>`). Приклеенный ко второму «?t=»
 * уезжал в значение `sig`, подпись не сходилась, и прокси отвечал 403 —
 * превью в настройках было пустым, хотя тот же аватар в подвале меню рисовался.
 */
class AvatarBustUrlTest {
    private val root = "https://tessera.example"

    /** Свой аватар: строки запроса нет, метка начинает её. */
    @Test
    fun `an uploaded avatar gets the buster as the first query parameter`() {
        assertThat(avatarBustUrl(root, "/api/users/u1/avatar", 7L))
            .isEqualTo("https://tessera.example/api/users/u1/avatar?t=7")
    }

    /** Тот самый кадр: подписанный прокси уже несёт «?», метка обязана быть «&». */
    @Test
    fun `a signed gitlab proxy url keeps its signature intact`() {
        val proxy = "/api/gitlab/avatar?ws=00000000-0000-0000-0000-000000000001&u=aHR0cA&sig=deadbeef"
        val url = avatarBustUrl(root, proxy, 7L)

        assertThat(url).isEqualTo("$root$proxy&t=7")
        // Знак вопроса в адресе ровно один — второй сделал бы метку частью sig.
        assertThat(url!!.count { it == '?' }).isEqualTo(1)
        assertThat(url).contains("sig=deadbeef&")
    }

    /** Рисовать нечего — и адреса нет: экран показывает инициалы. */
    @Test
    fun `no avatar yields no url`() {
        assertThat(avatarBustUrl(root, null, 7L)).isNull()
        assertThat(avatarBustUrl(root, "", 7L)).isNull()
        assertThat(avatarBustUrl(root, "   ", 7L)).isNull()
    }

    /** Метка меняется вместе с bust — иначе перезалитый аватар остался бы старым. */
    @Test
    fun `the buster changes with the value it carries`() {
        assertThat(avatarBustUrl(root, "/api/users/u1/avatar", 0L))
            .isNotEqualTo(avatarBustUrl(root, "/api/users/u1/avatar", 1L))
    }
}
