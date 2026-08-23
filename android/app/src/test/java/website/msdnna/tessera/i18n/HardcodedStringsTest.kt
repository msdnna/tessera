package website.msdnna.tessera.i18n

import com.google.common.truth.Truth.assertWithMessage
import java.io.File
import org.junit.Test

/**
 * Храповик извлечения строк (#2803, этап 7 #2796). Экраны переводятся волнами, и
 * главный риск — не в том, что осталось, а в том, что уже переведённый экран
 * тихо обрастает новыми русскими литералами.
 *
 * Тест держит список файлов, где кириллица в литералах ещё допустима. Список
 * лежит рядом в `resources/i18n/untranslated.txt` и сравнивается **точно**:
 * новый файл с литералами — красный тест, вычищенный файл обязан исчезнуть из
 * списка. Список сокращается волна за волной до пустого.
 */
class HardcodedStringsTest {
    @Test
    fun `only files still awaiting a wave carry cyrillic literals`() {
        val root = sourceRoot()
        val actual = root.walkTopDown()
            .filter { it.isFile && it.extension == "kt" }
            .filter { cyrillicLiterals(it.readText()).isNotEmpty() }
            .map { it.relativeTo(root).path.replace(File.separatorChar, '/') }
            .toSortedSet()

        val expected = expectedList()
        val leaked = actual - expected
        val cleaned = expected - actual

        assertWithMessage(
            "в этих файлах появились русские литералы — вынесите их в res/values/strings.xml " +
                "(и values-en) через stringResource",
        ).that(leaked).isEmpty()
        assertWithMessage(
            "эти файлы уже без литералов — уберите их из app/src/test/resources/i18n/untranslated.txt, " +
                "иначе храповик перестанет их сторожить",
        ).that(cleaned).isEmpty()
    }

    /** Метка точечная: она снимает охрану со своей строки и только с неё. */
    @Test
    fun `the data marker excuses its own line only`() {
        val source = """
            val re = Regex("рассмотр") // i18n-data
            val label = "Переименовать"
        """.trimIndent()
        assertWithMessage("метка сняла охрану с соседней строки")
            .that(cyrillicLiterals(source)).containsExactly("Переименовать")
    }

    /** Комментарии кириллицей — норма, сканер их не считает. */
    @Test
    fun `comments are not literals`() {
        val source = """
            // Переименовать колонку
            /* Удалить */
            val a = 1
        """.trimIndent()
        assertWithMessage("кириллица в комментарии посчиталась литералом")
            .that(cyrillicLiterals(source)).isEmpty()
    }

    private fun expectedList(): Set<String> {
        val stream = javaClass.classLoader?.getResourceAsStream(LIST)
            ?: error("не найден список $LIST в тестовых ресурсах")
        return stream.bufferedReader().readLines()
            .map { it.substringBefore('#').trim() }
            .filter { it.isNotEmpty() }
            .toSortedSet()
    }

    private companion object {
        const val LIST = "i18n/untranslated.txt"
    }
}

/** Каталог Kotlin-исходников приложения (рабочий каталог тестов — модуль `app`). */
internal fun sourceRoot(): File {
    val candidates = listOf(
        File("src/main/java"),
        File("app/src/main/java"),
        File("android/app/src/main/java"),
    )
    return candidates.firstOrNull { it.isDirectory }
        ?: error("не найдены исходники, пробовали: ${candidates.map { it.absolutePath }}")
}

/**
 * Строковые литералы Kotlin, содержащие кириллицу.
 *
 * Мини-сканер, а не регулярка: комментарии (в них кириллицы полно и это нормально),
 * символьные литералы и escape-последовательности иначе дают ложные срабатывания.
 *
 * Строка на помеченной `i18n-data` строке файла пропускается: не весь русский текст в
 * коде — интерфейс. Имена колонок приходят с сервера и не локализуются (подводный
 * камень 3 плана #2796), значит и шаблон, который по ним ищет, обязан остаться русским.
 * Метка ставится точечно, на ту же строку, — в отличие от списка файлов она не
 * выключает охрану всего экрана.
 */
internal fun cyrillicLiterals(source: String): List<String> = LiteralScanner(source).scan()

/** Разбор посимвольно: комментарии, символьные литералы и `"""` требуют своих правил. */
private class LiteralScanner(private val source: String) {
    private val out = mutableListOf<String>()
    private val lines = source.lines()
    private val dataLines = lines.withIndex().filter { DATA_MARKER in it.value }.map { it.index + 1 }.toSet()

    /** Смещение начала каждой строки: по нему литерал сопоставляется со своей строкой
     *  файла — курсор разбора прыгает через комментарии, считать `\n` по пути нельзя. */
    private val lineStarts = ArrayList<Int>(lines.size).apply {
        var at = 0
        for (l in lines) {
            add(at)
            at += l.length + 1
        }
    }
    private var i = 0

    fun scan(): List<String> {
        while (i < source.length) step()
        return out
    }

    private fun step() {
        when {
            source.startsWith("//", i) -> skipLineComment()
            source.startsWith("/*", i) -> skipBlockComment()
            source[i] == '\'' -> skipCharLiteral()
            source.startsWith("\"\"\"", i) -> readRawString()
            source[i] == '"' -> readString()
            else -> i++
        }
    }

    private fun skipLineComment() {
        while (i < source.length && source[i] != '\n') i++
    }

    private fun skipBlockComment() {
        i += 2
        while (i + 1 < source.length && !(source[i] == '*' && source[i + 1] == '/')) i++
        i = minOf(i + 2, source.length)
    }

    /** Символьный литерал: `'"'` и `'\\'` иначе сбивают разбор строк. */
    private fun skipCharLiteral() {
        i++
        while (i < source.length && source[i] != '\'') i += if (source[i] == '\\') 2 else 1
        i++
    }

    private fun readRawString() {
        val end = source.indexOf("\"\"\"", i + 3)
        val stop = if (end < 0) source.length else end
        keep(i, source.substring(i + 3, stop))
        i = if (end < 0) source.length else end + 3
    }

    private fun readString() {
        val start = i
        i++
        val sb = StringBuilder()
        while (i < source.length && source[i] != '"') {
            if (source[i] == '\\' && i + 1 < source.length) {
                sb.append(source[i + 1])
                i += 2
            } else {
                sb.append(source[i])
                i++
            }
        }
        i++
        keep(start, sb.toString())
    }

    private fun keep(offset: Int, text: String) {
        if (hasCyrillic(text) && lineAt(offset) !in dataLines) out += text
    }

    private fun lineAt(offset: Int): Int =
        lineStarts.binarySearch(offset).let { if (it >= 0) it + 1 else -it - 1 }
}

private fun hasCyrillic(text: String) = text.any { it in 'А'..'я' || it == 'ё' || it == 'Ё' }

/** Метка «эта кириллица — данные, а не интерфейс» (ставится в комментарии той же строки). */
private const val DATA_MARKER = "i18n-data"
