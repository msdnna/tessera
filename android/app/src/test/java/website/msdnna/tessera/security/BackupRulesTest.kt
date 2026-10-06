package website.msdnna.tessera.security

import com.google.common.truth.Truth.assertWithMessage
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Test
import org.w3c.dom.Element
import org.w3c.dom.Node

/**
 * Храповик правил бэкапа (#2932).
 *
 * Токены сессии лежат в DataStore, а каталог данных приложения по умолчанию
 * целиком уезжает в Auto Backup и в перенос устройство→устройство. Значит
 * refresh-токен оказывается в Google Drive пользователя, и восстановление такого
 * бэкапа на другом аппарате даёт живую сессию без пароля.
 *
 * Чинится это тремя файлами, ни один из которых ничего не ломает, если его
 * потерять: приложение собирается и работает одинаково с правилами и без них, а
 * утечка видна только при восстановлении бэкапа. Поэтому — тест.
 *
 * Проверка идёт от факта «токены в DataStore», а не от жёстко вбитого имени
 * файла: если хранилище переедет, красным станет первый же тест, а не молчание.
 */
class BackupRulesTest {

    @Test
    fun `session tokens live in the datastore directory`() {
        val prefs = File(moduleRoot(), "src/main/java/website/msdnna/tessera/data/preferences/AppPreferences.kt")
        assertWithMessage("не найден AppPreferences.kt — проверьте путь в тесте")
            .that(prefs.isFile).isTrue()
        val source = prefs.readText()

        for (key in listOf("auth_token", "refresh_token")) {
            assertWithMessage(
                "ключ $key больше не объявлен в AppPreferences — если токены переехали в другое " +
                    "хранилище, правила бэкапа надо переписать под него",
            ).that(source).contains("\"$key\"")
        }
        assertWithMessage(
            "AppPreferences больше не на preferencesDataStore — исключение каталога datastore " +
                "из бэкапа перестало закрывать токены",
        ).that(source).contains("preferencesDataStore(")
    }

    @Test
    fun `a backup-enabled app declares both rule files`() {
        val manifest = manifestElement()
        val app = manifest.childElement("application")
            ?: error("в манифесте нет <application>")

        if (app.getAttribute("android:allowBackup") == "false") return

        for (attr in listOf("android:dataExtractionRules", "android:fullBackupContent")) {
            assertWithMessage(
                "allowBackup включён, но $attr не задан: каталог данных (вместе с токенами) " +
                    "уезжает в бэкап целиком",
            ).that(app.getAttribute(attr)).isNotEmpty()
        }
    }

    @Test
    fun `the rules keep the datastore out of cloud backup and device transfer`() {
        val rules = mapOf(
            "data_extraction_rules.xml" to listOf("cloud-backup", "device-transfer"),
            // Android 11 и ниже: там действует fullBackupContent, и исключения
            // лежат прямо в корне документа.
            "backup_rules.xml" to listOf(""),
        )
        for ((file, sections) in rules) {
            val root = parse(File(moduleRoot(), "src/main/res/xml/$file"))
            for (section in sections) {
                val scope = if (section.isEmpty()) root else root.childElement(section)
                assertWithMessage("в $file нет секции <$section>").that(scope).isNotNull()
                assertWithMessage(
                    "в $file секция <$section> не исключает каталог datastore — " +
                        "refresh-токен уедет в бэкап",
                ).that(excludedFilePaths(scope!!)).contains("datastore")
            }
        }
    }

    /** Пути `<exclude domain="file" path="…"/>` внутри секции, без ведущих слэшей. */
    private fun excludedFilePaths(scope: Element): List<String> =
        scope.children()
            .filter { it.tagName == "exclude" && it.getAttribute("domain") == "file" }
            .map { it.getAttribute("path").trim().trim('/') }

    private fun manifestElement(): Element = parse(File(moduleRoot(), "src/main/AndroidManifest.xml"))

    private fun parse(file: File): Element {
        assertWithMessage("не найден ${file.path}").that(file.isFile).isTrue()
        val factory = DocumentBuilderFactory.newInstance().apply {
            // Тест читает только файлы репозитория, но внешние сущности тут не
            // нужны никому — а парсер по умолчанию их резолвит.
            setFeature("http://apache.org/xml/features/disallow-doctype-decl", true)
            isXIncludeAware = false
            isExpandEntityReferences = false
        }
        return factory.newDocumentBuilder().parse(file).documentElement
    }

    private fun Element.children(): List<Element> =
        (0 until childNodes.length)
            .map { childNodes.item(it) }
            .filter { it.nodeType == Node.ELEMENT_NODE }
            .map { it as Element }

    private fun Element.childElement(tag: String): Element? = children().firstOrNull { it.tagName == tag }

    /** Рабочий каталог тестов — модуль `app`; из корня репозитория путь длиннее. */
    private fun moduleRoot(): File = listOf(File("."), File("app"), File("android/app"))
        .firstOrNull { File(it, "src/main/AndroidManifest.xml").isFile }
        ?: error("не найден модуль app")
}
