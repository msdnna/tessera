package website.msdnna.tessera.update

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import java.io.File
import org.junit.Test

/**
 * Имя файла из `latest.json` — недоверенный ввод (#2932).
 *
 * Манифест самообновления приходит с сервера целиком, и его поле `apk`
 * используется дважды: в URL и в `File(cacheDir/updates, apk)`. Второе и есть
 * дыра — подпись APK её не закрывает. Установщик отказывается ставить пакет,
 * подписанный чужим ключом, поэтому подменённый `latest.json` сам по себе
 * кодом на устройстве не оборачивается; а вот запись файла происходит **до**
 * установщика и его проверки не касается вовсе. Имя вида
 * `../../shared_prefs/x.apk` пишет в приватное хранилище приложения — туда,
 * где лежат настройки и база.
 *
 * Дотянуться до манифеста может не только владелец сервера: при включённом
 * тумблере «не проверять TLS» (`TlsTrust.insecure`, #2896) цепочка и имя хоста
 * не проверяются ни в одном клиенте приложения, включая этот.
 */
class UpdateManifestTest {

    @Test
    fun `plain release file names pass`() {
        for (name in listOf("tessera-1.2.3.apk", "tessera-release.apk", "Tessera-2.0.0-RC1.APK")) {
            assertWithMessage("обычное имя релиза отвергнуто: $name")
                .that(UpdateRepository.safeApkName(name)).isEqualTo(name)
        }
    }

    @Test
    fun `names that leave the download directory are refused`() {
        val escaping = listOf(
            "../../shared_prefs/tessera.apk", // расширение верное — одной проверки суффикса мало
            "../databases/tessera.apk",
            "/data/data/website.msdnna.tessera/files/x.apk",
            "sub/dir/tessera.apk",
            "..\\..\\x.apk",
            "tessera\u0000.apk", // нулевой байт в имени — тоже не имя
            ".apk",
            "",
            "   ",
            "tessera.apk.txt",
        )
        for (name in escaping) {
            assertWithMessage("имя принято, хотя не должно: $name")
                .that(UpdateRepository.safeApkName(name)).isNull()
        }
    }

    /**
     * Не про валидатор, а про цену ошибки: то же имя, пропущенное в `File`,
     * действительно уходит из каталога загрузок. Без этой проверки тест выше
     * остался бы зелёным и в мире, где `File` так не умеет.
     */
    @Test
    fun `an unchecked name really escapes the updates directory`() {
        val cache = File(System.getProperty("java.io.tmpdir"), "tessera-update-test")
        val dir = File(cache, "updates")

        val safe = File(dir, "tessera-1.2.3.apk").canonicalPath
        assertThat(safe).startsWith(dir.canonicalPath)

        val escaped = File(dir, "../../shared_prefs/tessera.apk").canonicalPath
        assertWithMessage("обход каталога не воспроизвёлся — проверьте предпосылку теста")
            .that(escaped).doesNotContain("updates")
        assertThat(escaped).endsWith(File("shared_prefs", "tessera.apk").path)
    }

    /** Длина ограничена: имя файла, а не полотно. */
    @Test
    fun `absurdly long names are refused`() {
        assertThat(UpdateRepository.safeApkName("t".repeat(200) + ".apk")).isNull()
    }
}
