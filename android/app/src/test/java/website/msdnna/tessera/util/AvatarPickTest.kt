package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Правила пикера аватара (#2932) — зеркало `frontend/src/utils/avatar.js` и,
 * через него, серверного `handlers/users.go` (UploadMyAvatar).
 *
 * До правки правил у телефона не было вовсе: снимок с камеры уезжал целиком —
 * по мобильной сети тоже, — чтобы вернуться отказом 413. Сам отказ экран при этом
 * печатал `it.message`, то есть retrofit-строку «HTTP 413 Request Entity Too
 * Large», а не то, что сказал сервер.
 */
class AvatarPickTest {
    /** Ровно тот кадр, ради которого правило и заведено. */
    @Test
    fun `a camera shot over the ceiling is refused before it is sent`() {
        assertThat(avatarPickError(4L * 1024 * 1024, "image/jpeg")).isEqualTo(AvatarPickError.TooLarge)
        // Потолок — это «не больше», а не «меньше»: ровно 2 МиБ сервер берёт.
        assertThat(avatarPickError(AVATAR_MAX_BYTES, "image/jpeg")).isNull()
        assertThat(avatarPickError(AVATAR_MAX_BYTES + 1, "image/jpeg")).isEqualTo(AvatarPickError.TooLarge)
    }

    /** HEIF с современного телефона проходит фильтр системного пикера («image/»
     *  любого подтипа), но сервер его не опознаёт ни при каком размере. */
    @Test
    fun `a type the server never accepts is named as a type, not as a size`() {
        assertThat(avatarPickError(100_000, "image/heif")).isEqualTo(AvatarPickError.BadType)
        assertThat(avatarPickError(100_000, "image/svg+xml")).isEqualTo(AvatarPickError.BadType)
    }

    @Test
    fun `the four types the server sniffs for pass`() {
        for (type in AVATAR_TYPES) {
            assertThat(avatarPickError(100_000, type)).isNull()
        }
        // Регистр и параметры — того же типа дело: их приписывает провайдер, не человек.
        assertThat(avatarPickError(100_000, "IMAGE/PNG")).isNull()
        assertThat(avatarPickError(100_000, "image/jpeg; charset=utf-8")).isNull()
    }

    /**
     * Проверка типа односторонняя: провайдер, который типа не знает, отдаёт null,
     * а сервер смотрит на БАЙТЫ. Завернуть такой файл здесь значило бы сделать
     * пикер строже сервера.
     */
    @Test
    fun `a file with no type is handed over, not refused`() {
        assertThat(avatarPickError(100_000, null)).isNull()
        assertThat(avatarPickError(100_000, "")).isNull()
        // Но размер проверяется и у него — байты сервер считает независимо от типа.
        assertThat(avatarPickError(4L * 1024 * 1024, null)).isEqualTo(AvatarPickError.TooLarge)
    }
}
