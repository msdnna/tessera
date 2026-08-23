package website.msdnna.tessera.i18n

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Test
import org.w3c.dom.Element

/**
 * Гвард полноты локалей. Пропущенный ключ в `values-en` не падает и не подсвечивается —
 * ресурс просто отдаёт русскую строку из базовой локали, и англоязычный пользователь
 * видит вперемешку два языка. Такое замечают последним; пусть замечает тест.
 */
class StringResourcesTest {
    private val res = resDir()

    @Test
    fun `ru and en declare the same string keys`() {
        val ru = stringKeys("values") - UNTRANSLATABLE
        val en = stringKeys("values-en")
        assertWithMessage("ключи, которых нет в базовой локали").that(en - ru).isEmpty()
        assertWithMessage("ключи без английского перевода").that(ru - en).isEmpty()
    }

    @Test
    fun `ru and en declare the same plurals`() {
        assertThat(pluralQuantities("values-en").keys).isEqualTo(pluralQuantities("values").keys)
    }

    /** У русского четыре формы (one/few/many/other), у английского две. Недостающая
     *  форма молча подставляет `other` — «1 из 21 подзадач» вместо «подзадачи». */
    @Test
    fun `each locale declares every plural form its language needs`() {
        pluralQuantities("values").forEach { (name, quantities) ->
            assertWithMessage("ru plurals/$name").that(quantities).containsAtLeastElementsIn(RU_QUANTITIES)
        }
        pluralQuantities("values-en").forEach { (name, quantities) ->
            assertWithMessage("en plurals/$name").that(quantities).containsAtLeastElementsIn(EN_QUANTITIES)
        }
    }

    /** Позиционные аргументы (`%1$s`) должны совпадать: перевод с лишним `%2$d`
     *  роняет форматирование в рантайме, а не на сборке. */
    @Test
    fun `translations keep the same format arguments`() {
        val ru = formatArgs("values")
        val en = formatArgs("values-en")
        en.forEach { (key, args) ->
            assertWithMessage("аргументы формата в $key").that(args).isEqualTo(ru[key])
        }
    }

    private fun stringKeys(dir: String): Set<String> =
        elements(dir, "string").map { it.getAttribute("name") }.toSet()

    private fun pluralQuantities(dir: String): Map<String, Set<String>> =
        elements(dir, "plurals").associate { plural ->
            val items = plural.getElementsByTagName("item")
            plural.getAttribute("name") to (0 until items.length)
                .map { (items.item(it) as Element).getAttribute("quantity") }
                .toSet()
        }

    /** Ключ → набор `%n$x`-подстановок в нём (у plurals — по всем формам). */
    private fun formatArgs(dir: String): Map<String, Set<String>> {
        val out = mutableMapOf<String, Set<String>>()
        elements(dir, "string").forEach {
            out[it.getAttribute("name")] = ARG.findAll(it.textContent).map { m -> m.value }.toSet()
        }
        elements(dir, "plurals").forEach { plural ->
            val items = plural.getElementsByTagName("item")
            out[plural.getAttribute("name")] = (0 until items.length)
                .flatMap { ARG.findAll(items.item(it).textContent).map { m -> m.value } }
                .toSet()
        }
        return out
    }

    private fun elements(dir: String, tag: String): List<Element> {
        val file = File(res, "$dir/strings.xml")
        assertWithMessage("не найден $file").that(file.isFile).isTrue()
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file)
        val nodes = doc.documentElement.getElementsByTagName(tag)
        return (0 until nodes.length).map { nodes.item(it) as Element }
    }

    companion object {
        /** Имя бренда одинаково во всех локалях и намеренно живёт только в базовой. */
        private val UNTRANSLATABLE = setOf("app_name")
        private val RU_QUANTITIES = setOf("one", "few", "many", "other")
        private val EN_QUANTITIES = setOf("one", "other")
        private val ARG = Regex("""%\d+\$[a-zA-Z]""")
    }
}

/** Каталог ресурсов; unit-тесты Gradle стартуют из каталога модуля, но из IDE
 *  рабочим может оказаться корень проекта — пробуем оба, молча не пропускаем. */
internal fun resDir(): File {
    val candidates = listOf(
        File("src/main/res"),
        File("app/src/main/res"),
        File("android/app/src/main/res"),
    )
    return candidates.firstOrNull { it.isDirectory }
        ?: error("не найден каталог ресурсов, пробовали: ${candidates.map { it.absolutePath }}")
}
