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
     * остаётся удалённым осознанно (3,5 МБ ради единиц диаграмм).
     */
    @Test
    fun `only mermaid is still fetched from the network`() {
        val remote = Regex("""https?://[^'"\s;)]+""").findAll(html(false)).map { it.value }
            // Свой сервер — не «сеть» в этом смысле: это тот же хост, с которого
            // приехал текст, и картинки в нём и так подставляются по нему.
            .filterNot { it.startsWith("http://127.0.0.1:8090") }
            .toSet()
        assertThat(remote).containsExactly(MERMAID_URL)
    }

    /**
     * #2932: этот единственный удалённый скрипт заказывает НЕДОВЕРЕННЫЙ текст —
     * достаточно ```mermaid в описании задачи. Версия была плавающей (`@11`) и
     * без `integrity`, то есть «выполнить в привилегированном WebView то, что
     * CDN отдаст сегодня». Свойство, потерю которого ловим: байты удалённого
     * кода зафиксированы, и подменённый файл выполнен не будет.
     */
    @Test
    fun `the one remote script is pinned by version and by hash`() {
        val doc = html(false)
        assertWithMessage("версия mermaid снова плавающая — CDN решает, какой код выполнится")
            .that(MERMAID_URL).contains("mermaid@$MERMAID_VERSION/")
        assertWithMessage("версия должна быть точной, а не диапазоном")
            .that(MERMAID_VERSION).matches("""\d+\.\d+\.\d+""")
        assertWithMessage("у удалённого скрипта нет integrity — подмена на CDN выполнится как своя")
            .that(doc).contains("m.integrity = '$MERMAID_SRI'")
        assertWithMessage("SRI без crossOrigin=anonymous не проверяется вовсе")
            .that(doc).contains("m.crossOrigin = 'anonymous'")
        assertThat(MERMAID_SRI).matches("""sha(256|384|512)-[A-Za-z0-9+/]+=*""")
    }

    /**
     * #2932: DOMPurify — единственный рубеж перед `innerHTML`, а SVG от mermaid
     * кладётся в `innerHTML` уже мимо него. CSP — второй рубеж: свойство,
     * которое проверяем, — что выполнить в этом WebView можно только код из
     * APK, инлайн по nonce и ровно тот адрес mermaid, что прибит выше.
     */
    @Test
    fun `the document carries a policy that pins where code may come from`() {
        val doc = html(false)
        val csp = Regex("""<meta http-equiv="Content-Security-Policy" content="([^"]+)"""")
            .find(doc)?.groupValues?.get(1)
        assertWithMessage("в документе нет CSP — обход санитайзера ничем не ограничен").that(csp).isNotNull()
        val scriptSrc = csp!!.split(";").map { it.trim() }.first { it.startsWith("script-src ") }
        assertWithMessage("script-src: $scriptSrc").that(scriptSrc).contains("file:")
        assertWithMessage("script-src: $scriptSrc").that(scriptSrc).contains(MERMAID_URL)
        listOf("'unsafe-inline'", "'unsafe-eval'", "'strict-dynamic'", "http:", "https:", "*")
            .forEach { banned ->
                assertWithMessage("script-src снова пускает $banned: $scriptSrc")
                    .that(scriptSrc.split(" ").drop(1)).doesNotContain(banned)
            }
        listOf("object-src 'none'", "base-uri 'none'", "form-action 'none'", "frame-src 'none'")
            .forEach { assertWithMessage("в политике нет «$it»").that(csp).contains(it) }
    }

    /** Nonce обязан быть непредсказуемым и разным у каждого документа — иначе
     *  внедрённый скрипт просто перепишет чужой. */
    @Test
    fun `the inline script nonce is fresh per document`() {
        val nonces = (1..8).map {
            Regex("""<script nonce="([0-9a-f]{32})">""").find(html(false))?.groupValues?.get(1)
        }
        assertWithMessage("инлайновый скрипт не помечен nonce'ом — CSP его не пустит")
            .that(nonces.filterNotNull()).hasSize(8)
        assertWithMessage("nonce переиспользуется между документами").that(nonces.toSet()).hasSize(8)
    }

    /**
     * #2932: markdown задачи допускает сырой HTML, а marked отдаёт его как есть.
     * Здесь это опаснее, чем во вкладке браузера: в WebView включён JS, висит мост
     * `AndroidRich`, а `shouldInterceptRequest` подставляет Bearer к запросам на
     * свой сервер — то есть внедрённый скрипт читает API от лица жертвы. Web гонит
     * тот же текст через DOMPurify; свойство, потерю которого мы ловим, — что и
     * здесь между marked и `innerHTML` стоит санитайзер, причём локальный.
     */
    @Test
    fun `untrusted markup is sanitised before it reaches innerHTML`() {
        val doc = html(false)
        assertWithMessage("санитайзера нет в ассетах — DOMPurify.sanitize упадёт уже в рантайме")
            .that(doc).contains("file:///android_asset/richcontent/purify.min.js")
        assertWithMessage("разметка попадает в innerHTML в обход DOMPurify")
            .that(doc).contains("el.innerHTML = DOMPurify.sanitize(")
    }

    /** Тема светлая/тёмная различает файл, а не только цвета вокруг. */
    @Test
    fun `the light and dark documents pick different hljs themes`() {
        assertThat(html(false)).contains("richcontent/hljs-github.min.css")
        assertThat(html(true)).contains("richcontent/hljs-github-dark.min.css")
    }
}
