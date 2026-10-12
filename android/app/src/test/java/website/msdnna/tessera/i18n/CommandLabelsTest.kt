package website.msdnna.tessera.i18n

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Test
import org.w3c.dom.Element
import website.msdnna.tessera.util.BUILTIN_COMMAND_DESCRIPTIONS

/**
 * Витрина быстрых действий («/due», «/assign») — единственное место, где текст
 * приходит с бэкенда уже написанным: `internal/quickact/registry.go` захардкодил
 * Description по-русски и отдаёт его клиентам как есть. На английском интерфейсе
 * попап оттого читался целиком по-русски, хотя сам парсер двуязычен.
 *
 * Тест сторожит обе стороны шва: каждая команда реестра должна иметь описание в
 * [BUILTIN_COMMAND_DESCRIPTIONS] и строку в обеих локалях.
 */
class CommandLabelsTest {
    private val registryKeys: List<String> = run {
        val source = registryFile().readText()
        val body = source.substringAfter("var Registry = []Command{").substringBefore("\n// byKey maps")
        Regex("""\{Key:\s*"([a-z_]+)"""").findAll(body).map { it.groupValues[1] }.toList()
    }

    @Test
    fun `the backend registry is read, not guessed`() {
        assertThat(registryKeys.size).isGreaterThan(20)
        assertThat(registryKeys).contains("assign")
        assertThat(registryKeys).contains("unparent")
    }

    @Test
    fun `every built-in command has a localizable description`() {
        assertWithMessage("команды реестра без записи в BUILTIN_COMMAND_DESCRIPTIONS")
            .that(registryKeys - BUILTIN_COMMAND_DESCRIPTIONS.keys)
            .isEmpty()
        assertWithMessage("записи, которых нет в реестре бэкенда")
            .that(BUILTIN_COMMAND_DESCRIPTIONS.keys - registryKeys.toSet())
            .isEmpty()
    }

    @Test
    fun `both locales carry a string for every command`() {
        val ru = stringKeys("values")
        val en = stringKeys("values-en")
        registryKeys.forEach { key ->
            val name = "cmd_${key}_desc"
            assertWithMessage("русская строка $name").that(ru).contains(name)
            assertWithMessage("английская строка $name").that(en).contains(name)
        }
    }

    /** Английское описание, оставшееся кириллицей, — ровно тот баг, от которого тест. */
    @Test
    fun `the english strings are actually in english`() {
        val cyrillic = Regex("[А-Яа-яЁё]")
        strings("values-en")
            .filterKeys { it.startsWith("cmd_") && it.endsWith("_desc") }
            .forEach { (name, value) ->
                assertWithMessage("$name: «$value»").that(cyrillic.containsMatchIn(value)).isFalse()
            }
    }

    private fun stringKeys(dir: String): Set<String> = strings(dir).keys

    private fun strings(dir: String): Map<String, String> {
        val file = File(resDir(), "$dir/strings.xml")
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file)
        val nodes = doc.getElementsByTagName("string")
        return (0 until nodes.length).associate {
            val e = nodes.item(it) as Element
            e.getAttribute("name") to e.textContent
        }
    }

    private fun registryFile(): File =
        listOf(
            File("../../backend/internal/quickact/registry.go"),
            File("../backend/internal/quickact/registry.go"),
            File("backend/internal/quickact/registry.go"),
        ).firstOrNull { it.isFile }
            ?: error("registry.go не найден от ${File(".").absolutePath}")
}
