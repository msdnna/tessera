package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Правило пароля, которое на самом деле применяет сервер (#2932) — зеркало
 * `frontend/src/utils/password.js` и `backend/internal/auth.CheckPasswordPolicy`.
 *
 * Единицы у двух концов правила РАЗНЫЕ, и в этом вся суть: минимум сервер
 * считает рунами (валидатор gin), потолок принадлежит bcrypt и меряется в
 * БАЙТАХ. `String.length` в Kotlin — не то и не другое (UTF-16), поэтому
 * экранное `next.length >= 8` расходилось с сервером на обоих концах.
 */
class PasswordTest {
    // ── потолок bcrypt: его не знал никто ────────────────────────────────────

    /** 38 русских букв — 76 байт. Раньше это уезжало на сервер и возвращалось
     *  «internal error» (а теперь вернулось бы 400 с английским сентинелом). */
    @Test
    fun `a Russian passphrase over 72 bytes is rejected before it is sent`() {
        val pw = "парольнаякоторуюялегкозапоминаюсразуже"
        assertThat(pw.length).isLessThan(MAX_PASSWORD_BYTES) // 38 символов — «коротко» по length
        assertThat(passwordBytes(pw)).isGreaterThan(MAX_PASSWORD_BYTES)
        assertThat(passwordProblem(pw)).isEqualTo(PasswordProblem.LONG)
        assertThat(isPasswordValid(pw)).isFalse()
    }

    @Test
    fun `the ceiling is measured in bytes, not characters`() {
        assertThat(passwordBytes("a".repeat(72))).isEqualTo(72)
        assertThat(passwordBytes("я".repeat(36))).isEqualTo(72)
        assertThat(passwordBytes("😀".repeat(18))).isEqualTo(72)
    }

    /** Ровно на границе — годится: предел bcrypt включающий. */
    @Test
    fun `exactly 72 bytes is still storable`() {
        assertThat(passwordProblem("a".repeat(72))).isNull()
        assertThat(passwordProblem("я".repeat(36))).isNull()
        assertThat(passwordProblem("a".repeat(73))).isEqualTo(PasswordProblem.LONG)
        assertThat(passwordProblem("я".repeat(36) + "a")).isEqualTo(PasswordProblem.LONG)
    }

    // ── минимум: руны, а не UTF-16 ───────────────────────────────────────────

    /** Восемь эмодзи — это 16 единиц UTF-16, но 8 рун: сервер их принимает,
     *  и клиент обязан принимать тоже, иначе кнопка заперта без причины. */
    @Test
    fun `the minimum counts code points`() {
        assertThat(passwordChars("😀".repeat(8))).isEqualTo(8)
        assertThat(passwordProblem("😀".repeat(8))).isNull()
    }

    /** И наоборот: пять эмодзи — 10 единиц UTF-16, но всего 5 рун. Экранное
     *  `length >= 8` пускало такой пароль на сервер, где его ждал отказ. */
    @Test
    fun `five emoji are too short even though length says ten`() {
        val pw = "😀".repeat(5)
        assertThat(pw.length).isAtLeast(MIN_PASSWORD_CHARS)
        assertThat(passwordChars(pw)).isLessThan(MIN_PASSWORD_CHARS)
        assertThat(passwordProblem(pw)).isEqualTo(PasswordProblem.SHORT)
    }

    // ── контроли: то, что работало и должно продолжать ───────────────────────

    @Test
    fun `an ordinary password passes`() {
        assertThat(passwordProblem("hunter22")).isNull()
        assertThat(passwordProblem("пароль12")).isNull()
        assertThat(isPasswordValid("correct horse battery staple")).isTrue()
    }

    @Test
    fun `a short password is still short`() {
        assertThat(passwordProblem("short")).isEqualTo(PasswordProblem.SHORT)
        assertThat(passwordProblem("7 chars")).isEqualTo(PasswordProblem.SHORT)
        assertThat(passwordProblem("")).isEqualTo(PasswordProblem.SHORT)
        assertThat(passwordProblem(null)).isEqualTo(PasswordProblem.SHORT)
    }

    /** Пробелы — часть парольной фразы, а не оформление: обрезать их нельзя. */
    @Test
    fun `spaces count as characters`() {
        assertThat(passwordProblem("        ")).isNull()
        assertThat(passwordChars(" a b c ")).isEqualTo(7)
    }
}
