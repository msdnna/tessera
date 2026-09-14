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

    /**
     * Кириллица в `values-en` — русский текст, забытый под английским ключом. Паритет
     * ключей такое не ловит: строка на месте, просто не переведена. Самый вредный случай
     * не опечатка, а имя сущности, вписанное в подсказку «как на сервере»: подписи
     * засеянных колонок и личного пространства клиент с #2800 собирает по `name_key` из
     * своих ресурсов, так что вписанное «К работе» на английском интерфейсе называет
     * колонку, которой на экране нет (тур #2860).
     */
    @Test
    fun `no russian text is left in the english locale`() {
        val leftovers = mutableListOf<String>()
        elements("values-en", "string").forEach {
            if (it.getAttribute("name") !in SELF_NAMED && CYRILLIC.containsMatchIn(it.textContent)) {
                leftovers += it.getAttribute("name")
            }
        }
        listOf("plurals", "string-array").forEach { tag ->
            elements("values-en", tag).forEach {
                if (CYRILLIC.containsMatchIn(it.textContent)) leftovers += "$tag/${it.getAttribute("name")}"
            }
        }
        assertWithMessage("непереведённые строки в values-en").that(leftovers).isEmpty()
    }

    /**
     * Шаг тура и его подстановки. `bodyArgs` разрешаются в [TourOverlay] через
     * `getString(bodyRes, *args)`: лишний аргумент молча пропадёт, а недостающий покажет
     * пользователю сырое «%1$s» вместо названия колонки. Сценарий читается из исходника,
     * как реестр команд в [CommandLabelsTest], — сверять нужно именно объявленную пару.
     */
    @Test
    fun `every tour step supplies exactly the arguments its text takes`() {
        val steps = TOUR_STEP.findAll(tourFile().readText()).toList()
        assertWithMessage("шаги тура не распознаны в Tour.kt").that(steps.size).isGreaterThan(10)
        val ru = formatArgs("values")
        steps.forEach { m ->
            val key = m.groupValues[1]
            val declared = m.groupValues[2].let { if (it.isBlank()) 0 else it.split(",").size }
            assertWithMessage("подстановки шага с текстом $key")
                .that(declared)
                .isEqualTo(ru.getValue(key).size)
        }
    }

    /** Массивы (названия месяцев, дни недели) паритетом ключей не покрыты: недостающий
     *  `string-array` в `values-en` так же молча отдаёт русский, а разъехавшаяся длина
     *  роняет обращение по индексу уже в рантайме. */
    @Test
    fun `ru and en declare the same string arrays`() {
        val ru = stringArrays("values")
        val en = stringArrays("values-en")
        assertWithMessage("массивы, которых нет в базовой локали").that(en.keys - ru.keys).isEmpty()
        assertWithMessage("массивы без английского перевода").that(ru.keys - en.keys).isEmpty()
        ru.forEach { (name, size) ->
            assertWithMessage("длина массива $name").that(en[name]).isEqualTo(size)
        }
    }

    /**
     * Порядок полей в английских подписях даты. Все они собираются как `(день,
     * месяц[, год])`, и локаль вольна их переставить — но английский в проекте это
     * en-GB, а не en-US: веб рисует его через `localeTag('en') = 'en-GB'` ровно
     * затем, чтобы день стоял перед месяцем во всех локалях, которые мы отдаём
     * (frontend/src/utils/format.js).
     *
     * Гвард ресурсный, а не по подписи: строк с этим порядком несколько, они
     * добавлялись разными волнами (#2803 — сборка, #2857 — пресеты), и разъехались
     * они именно так — `medium` (он берёт `dates_long`) печатал «Jun 4, 2026»
     * посреди экрана, где `short` и `long` уже стояли днём вперёд.
     */
    @Test
    fun `english date labels keep the day before the month`() {
        val strings = elements("values-en", "string").associate {
            it.getAttribute("name") to it.textContent
        }
        DAY_FIRST_DATES.forEach { key ->
            val value = strings[key] ?: error("нет строки $key в values-en")
            val day = value.indexOf("%1\$")
            val month = value.indexOf("%2\$")
            assertWithMessage("в $key = \"$value\" день должен стоять перед месяцем")
                .that(day in 0 until month)
                .isTrue()
        }
    }

    /** Имя массива → число элементов. */
    private fun stringArrays(dir: String): Map<String, Int> =
        elements(dir, "string-array").associate {
            it.getAttribute("name") to it.getElementsByTagName("item").length
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

    /** Сценарий тура; пути — как у [resDir], рабочим каталогом может быть и модуль,
     *  и корень проекта. */
    private fun tourFile(): File {
        val tour = "java/website/msdnna/tessera/util/Tour.kt"
        return listOf(File("src/main/$tour"), File("app/src/main/$tour"), File("android/app/src/main/$tour"))
            .firstOrNull { it.isFile }
            ?: error("Tour.kt не найден от ${File(".").absolutePath}")
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

        /** Язык называет себя на себе же — «Русский» в английском списке языков
         *  правильный, это не забытый перевод. */
        private val SELF_NAMED = setOf("language_ru")

        /** Подписи даты, которые склеиваются из `(день, месяц[, год])`. */
        private val DAY_FIRST_DATES = listOf(
            "dates_short",
            "dates_short_year",
            "dates_long",
            "dates_preset_short",
            "dates_preset_long",
        )
        private val CYRILLIC = Regex("""[Ѐ-ӿ]""")
        private val RU_QUANTITIES = setOf("one", "few", "many", "other")
        private val EN_QUANTITIES = setOf("one", "other")
        private val ARG = Regex("""%\d+\$[a-zA-Z]""")

        /** `bodyRes = R.string.<ключ>` и следующая за ним необязательная строка
         *  `bodyArgs = listOf(…)` одного и того же [website.msdnna.tessera.util.TourStep]. */
        private val TOUR_STEP = Regex(
            """bodyRes = R\.string\.(\w+),\s*(?:bodyArgs = listOf\(([^)]*)\),)?""",
        )
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
