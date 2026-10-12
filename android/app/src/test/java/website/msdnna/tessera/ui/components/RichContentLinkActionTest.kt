package website.msdnna.tessera.ui.components

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import org.junit.Test

/**
 * #2932: куда уходит нажатая в описании/комментарии ссылка.
 *
 * Ссылка на файл задачи (`/api/attachments/<uuid>/download`) уходила в браузер
 * наравне с любой другой — а маршрут скачивания сидит в защищённой группе
 * роутера, и токен живёт только в памяти приложения. Браузер приносил 401:
 * приложенный к комментарию файл на телефоне было нечем открыть, хотя тот же
 * файл во вкладке «Файлы» скачивался. На web этот клик перехватывает
 * `RichContent.vue` ровно по той же причине.
 *
 * Проверяется решение, а не WebView: `shouldOverrideUrlLoading` — тонкая обёртка
 * над [linkAction].
 */
class RichContentLinkActionTest {
    private val root = "https://tessera.example"
    private val id = "0f3c7f1e-6b2a-4c5d-9e8f-1a2b3c4d5e6f"

    @Test
    fun `our own attachment link is downloaded, not opened in a browser`() {
        val url = "$root/api/attachments/$id/download"
        val action = linkAction(url, "https", root)
        assertWithMessage("ссылка на файл задачи снова уехала в браузер — там она даст 401")
            .that(action).isEqualTo(LinkAction.Attachment(url))
    }

    @Test
    fun `an ordinary link still goes to the browser`() {
        val url = "https://example.com/spec"
        assertThat(linkAction(url, "https", root)).isEqualTo(LinkAction.External(url))
    }

    @Test
    fun `an inline image url is not an attachment link`() {
        // `/api/uploads/…` раздаётся публично и приезжает через
        // shouldInterceptRequest — перехватывать его как файл нечего.
        val url = "$root/api/uploads/$id.png"
        assertThat(linkAction(url, "https", root)).isEqualTo(LinkAction.External(url))
    }

    @Test
    fun `a look-alike host does not get our token`() {
        // Путь можно написать любой: «своё» решается по origin (isOwnServerUrl),
        // иначе `![](…)` в задаче увёл бы Bearer на чужой хост.
        listOf(
            "https://tessera.example.attacker.test/api/attachments/$id/download",
            "https://tessera.example@attacker.test/api/attachments/$id/download",
            "http://tessera.example/api/attachments/$id/download", // другая схема — другой origin
        ).forEach { url ->
            assertWithMessage("чужой хост принят за свой: $url")
                .that(linkAction(url, url.substringBefore(":"), root))
                .isEqualTo(LinkAction.External(url))
        }
    }

    @Test
    fun `non-http schemes are left to the WebView`() {
        // Документ грузится с file:///android_asset/, и его собственные переходы
        // (якоря, about:blank) перехватывать нельзя.
        listOf("file", "about", "data", null).forEach { scheme ->
            assertWithMessage("схема $scheme перехвачена")
                .that(linkAction("$scheme:///whatever", scheme, root))
                .isEqualTo(LinkAction.None)
        }
    }
}
