package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Инициалы аватара — ОДНО правило на всё приложение (#2932), зеркало
 * `frontend/src/utils/initials.js`.
 *
 * Правило жило пятью приватными копиями (карточка, модалка, MemberAvatar,
 * профиль, админ-список), и две из них знали только «два слова» и «первые два
 * символа». Для точечного хэндла — ровно того, каким приходит логин из ростера
 * GitLab, — такая копия выдавала `take(2)` сырой строки: «A.», буква и ТОЧКА, на
 * тех самых экранах, где человек смотрит на свой собственный аватар. А там, где
 * имя пустое, подставляется e-mail, так что точка в строке — случай не редкий,
 * а обычный.
 */
class AvatarInitialsTest {
    /** Точечный хэндл: части хэндла здесь и есть «слова». */
    @Test
    fun `a dot handle gives a letter per part, not a letter and a dot`() {
        assertThat(avatarInitials("a.fokin")).isEqualTo("AF")
        assertThat(avatarInitials("v.sokolov")).isEqualTo("VS")
        // Это и есть регрессия: урезанная копия отдавала первые два символа.
        assertThat(avatarInitials("a.fokin")).isNotEqualTo("a.fokin".take(2).uppercase())
    }

    /** Подстановка e-mail вместо пустого имени — профиль и подвал сайдбара
     *  делают именно так. */
    @Test
    fun `an e-mail fallback reads as the person, not as the first syllable`() {
        assertThat(avatarInitials("ivan.petrov@example.com")).isEqualTo("IP")
    }

    @Test
    fun `two words give a letter from each`() {
        assertThat(avatarInitials("Василий Соколов")).isEqualTo("ВС")
        // Лишние пробелы между словами не создают пустых «слов».
        assertThat(avatarInitials("  Василий   Соколов  ")).isEqualTo("ВС")
    }

    @Test
    fun `a single token gives its first two letters`() {
        assertThat(avatarInitials("msdnna")).isEqualTo("MS")
    }

    /** Имя из двух слов важнее точки внутри слова: точка тут — конец инициала. */
    @Test
    fun `a trailing dot does not turn a two-word name into a handle`() {
        assertThat(avatarInitials("Иван П.")).isEqualTo("ИП")
    }

    @Test
    fun `an empty name falls back to a question mark`() {
        assertThat(avatarInitials("")).isEqualTo("?")
        assertThat(avatarInitials("   ")).isEqualTo("?")
        assertThat(avatarInitials(null)).isEqualTo("?")
        // Сам «?» уже является запасным значением у вызывающих — не ломаем его.
        assertThat(avatarInitials("?")).isEqualTo("?")
    }
}
