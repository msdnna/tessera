package website.msdnna.tessera.ui.components

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.ui.theme.DarkPalette
import website.msdnna.tessera.ui.theme.LightPalette

/**
 * #2897: описание и комментарии появлялись через десяток секунд на любой задаче.
 * Виноват был единственный внешний `<link rel=stylesheet>` на CDN в `<head>` —
 * браузер обязан дождаться его прежде, чем нарисовать первый кадр, и платилось
 * это на каждом рендере независимо от содержимого.
 *
 * Свойство, потерю которого мы ловим: **в отрисовке не участвует сеть**. Оно
 * проверяется чистой функцией — без WebView, эмулятора и интернета, то есть
 * ровно там, где хостовая проверка «в Chrome рендерится отлично» его проглядела.
 */
@RunWith(RobolectricTestRunner::class)
class RichContentHtmlTest {
    private fun html(dark: Boolean, source: String = "просто текст") =
        buildRichHtml(
            source = source,
            c = if (dark) DarkPalette else LightPalette,
            serverRoot = "http://127.0.0.1:8090",
            mentions = emptyList(),
            interactive = false,
            mentionCards = false,
            taskRefs = false,
            helpLinks = false,
        )

    /** Стили в `<head>` рендер-блокирующие: ни один не смеет уходить в сеть. */
    @Test
    fun `no stylesheet in the document points outside the apk`() {
        listOf(false, true).forEach { dark ->
            val links = Regex("""<link[^>]*rel="stylesheet"[^>]*>""").findAll(html(dark)).map { it.value }.toList()
            assertWithMessage("тема подсветки пропала из документа").that(links).isNotEmpty()
            links.forEach {
                assertWithMessage("рендер-блокирующий стиль уходит в сеть: $it")
                    .that(it).contains("file:///android_asset/richcontent/")
            }
        }
    }

    /**
     * Скрипты грузятся асинхронно и первый кадр не блокируют, но без сети код
     * остаётся неподсвеченным — поэтому парсер тоже переехал в ассеты. Mermaid
     * остаётся удалённым осознанно (полмегабайта ради единиц диаграмм).
     */
    @Test
    fun `only mermaid is still fetched from the network`() {
        val remote = Regex("""https?://[^'"\s)]+""").findAll(html(false)).map { it.value }
            // Свой сервер — не «сеть» в этом смысле: это тот же хост, с которого
            // приехал текст, и картинки в нём и так подставляются по нему.
            .filterNot { it.startsWith("http://127.0.0.1:8090") }
            .toSet()
        assertThat(remote).containsExactly("https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js")
    }

    /** Тема светлая/тёмная различает файл, а не только цвета вокруг. */
    @Test
    fun `the light and dark documents pick different hljs themes`() {
        assertThat(html(false)).contains("richcontent/hljs-github.min.css")
        assertThat(html(true)).contains("richcontent/hljs-github-dark.min.css")
    }
}
