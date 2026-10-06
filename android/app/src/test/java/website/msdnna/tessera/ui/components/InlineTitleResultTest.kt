package website.msdnna.tessera.ui.components

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * Холостой коммит встроенного редактора заголовка (#2932).
 *
 * Редактор отправляет введённое не только по Enter, но и по потере фокуса, поэтому
 * «открыл и тапнул мимо» — его самый частый исход. Для карточки это был лишний PATCH
 * и лишнее `task.updated` всем, кто смотрит доску, а для колонки — потеря данных:
 * у засеянной колонки поле открывается ПОДПИСЬЮ из ресурсов (`name_key`, #2800),
 * тогда как сервер хранит рядом собственное русское имя. У читателя с английским
 * интерфейсом холостой коммит присылал «In progress» как новое имя, и UpdateColumn
 * снимал `name_key` навсегда — колонка оставалась английской для ВСЕХ, включая тех,
 * для кого она только что перестала переводиться.
 *
 * Тест на чистой функции намеренно: композицию с текстовым полем Robolectric
 * до состояния idle не доводит (AppNotIdleException), и такой тест валил бы соседей
 * по JVM, ничего при этом не проверяя.
 */
class InlineTitleResultTest {
    /** Текст не тронули — ничего не отправляем. */
    @Test
    fun unchangedTitleIsCancelled() {
        assertThat(inlineTitleResult("В процессе", "В процессе")).isNull()
    }

    /**
     * Тот самый сценарий: у колонки с `name_key` редактор открыт английской подписью,
     * человек ничего не печатал. Отправить это = снять ключ.
     */
    @Test
    fun unchangedLocalisedCaptionIsCancelled() {
        assertThat(inlineTitleResult("In progress", "In progress")).isNull()
    }

    /** Пустая строка (и строка из пробелов) именем колонки стать не может. */
    @Test
    fun blankIsCancelled() {
        assertThat(inlineTitleResult("В процессе", "")).isNull()
        assertThat(inlineTitleResult("В процессе", "   ")).isNull()
    }

    /** Приписанные пробелы — не правка: после trim текст тот же. */
    @Test
    fun whitespaceOnlyEditIsCancelled() {
        assertThat(inlineTitleResult("В процессе", "  В процессе  ")).isNull()
    }

    /** Исходное само пришло с пробелами по краям — сравниваем по обрезанным. */
    @Test
    fun paddedInitialComparesTrimmed() {
        assertThat(inlineTitleResult("  В процессе  ", "В процессе")).isNull()
    }

    /** Настоящее переименование доезжает, уже обрезанным. */
    @Test
    fun realRenameCommitsTrimmed() {
        assertThat(inlineTitleResult("In progress", "  Doing  ")).isEqualTo("Doing")
    }

    /** Регистр — это правка: «в процессе» и «В процессе» не одно и то же. */
    @Test
    fun caseChangeIsARename() {
        assertThat(inlineTitleResult("В процессе", "в процессе")).isEqualTo("в процессе")
    }

    /** Пустое исходное (новая строка) не мешает завести имя. */
    @Test
    fun emptyInitialAcceptsAnyName() {
        assertThat(inlineTitleResult("", "Бэклог")).isEqualTo("Бэклог")
    }
}
