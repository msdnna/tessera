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
 */
internal fun cyrillicLiterals(source: String): List<String> {
    val out = mutableListOf<String>()
    var i = 0
    while (i < source.length) {
        when {
            source.startsWith("//", i) -> {
                while (i < source.length && source[i] != '\n') i++
            }

            source.startsWith("/*", i) -> {
                i += 2
                while (i + 1 < source.length && !(source[i] == '*' && source[i + 1] == '/')) i++
                i = minOf(i + 2, source.length)
            }

            // Символьный литерал: '"' и '\\' иначе сбивают разбор строк.
            source[i] == '\'' -> {
                i++
                while (i < source.length && source[i] != '\'') i += if (source[i] == '\\') 2 else 1
                i++
            }

            source.startsWith("\"\"\"", i) -> {
                val end = source.indexOf("\"\"\"", i + 3)
                val stop = if (end < 0) source.length else end
                source.substring(i + 3, stop).takeIf(::hasCyrillic)?.let { out += it }
                i = if (end < 0) source.length else end + 3
            }

            source[i] == '"' -> {
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
                sb.toString().takeIf(::hasCyrillic)?.let { out += it }
            }

            else -> i++
        }
    }
    return out
}

private fun hasCyrillic(text: String) = text.any { it in 'А'..'я' || it == 'ё' || it == 'Ё' }
