package website.msdnna.tessera.util

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.TaskEvent

/**
 * Строка истории задачи собирается из `kind` + `data` — как в вебе
 * (`utils/taskFeed.js eventText`).
 *
 * До этого Android знал только `kind`, а поля `data` в модели не было вовсе. Отсюда
 * две потери. Первая: пять видов, которые бэкенд пишет (`start`, `estimate`,
 * `relation_removed`, `commands`, `synced`), не были разобраны и падали в
 * `else -> kind` — в истории стояли сырые латинские слова посреди русского текста.
 * Вторая: цель события терялась — переименование не говорило на что, перенос не
 * называл колонку, связь — номер задачи, а снятие срока читалось теми же словами,
 * что и его установка.
 */
@RunWith(RobolectricTestRunner::class)
class TaskJournalTest {
    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    private fun res(language: String) = appContext.withLanguage(language).resources

    private fun event(kind: String, data: String = "{}"): TaskEvent =
        Gson().fromJson("""{"id":"e1","kind":"$kind","data":$data}""", TaskEvent::class.java)

    private fun text(language: String, kind: String, data: String = "{}"): String =
        journalText(res(language), event(kind, data))

    @Test
    fun `every kind the backend writes reads as a sentence, not as its key`() {
        // Список — все вызовы logEvent/logEventActor на бэкенде (handlers/*.go).
        val kinds = listOf(
            "created", "renamed", "description", "priority", "due", "start", "estimate",
            "completed", "reopened", "recurred", "moved", "assigned", "unassigned",
            "archived", "restored", "comment", "relation", "relation_removed",
            "tagged", "untagged", "commands", "synced", "attachment", "attachment_removed",
            "milestone", "milestone_removed", "code_change",
        )
        for (language in listOf("ru", "en")) {
            for (kind in kinds) {
                assertThat(text(language, kind)).isNotEqualTo(kind)
            }
        }
    }

    @Test
    fun `clearing a date is not worded like setting it`() {
        assertThat(text("ru", "due", """{"set":true}""")).isEqualTo("установил(а) срок")
        assertThat(text("ru", "due", """{"set":false}""")).isEqualTo("убрал(а) срок")
        assertThat(text("en", "start", """{"set":true}""")).isEqualTo("set the start date")
        assertThat(text("en", "start", """{"set":false}""")).isEqualTo("cleared the start date")
        assertThat(text("ru", "estimate", """{"set":false}""")).isEqualTo("убрал(а) оценку")
    }

    @Test
    fun `a record older than the flag keeps the neutral wording`() {
        // `set` появился вместе с событием, но пустой `data` приезжает и от более
        // старых строк (бэкенд подставляет `{}`): нейтральная фраза лучше, чем
        // наугад выбранная половина.
        assertThat(text("ru", "due")).isEqualTo("изменил(а) срок")
        assertThat(text("en", "estimate")).isEqualTo("changed the estimate")
    }

    @Test
    fun `the target of the change is named`() {
        assertThat(text("ru", "renamed", """{"to":"Починить чайник"}"""))
            .isEqualTo("переименовал(а) → «Починить чайник»")
        assertThat(text("en", "renamed", """{"to":"Fix the kettle"}"""))
            .isEqualTo("renamed it to “Fix the kettle”")
        assertThat(text("ru", "attachment", """{"filename":"смета.pdf"}"""))
            .isEqualTo("прикрепил(а) файл «смета.pdf»")
        // Снятие вложения уносит и байты, и строку, и удалить файл может любой
        // участник — имя в записи остаётся единственным следом того, что пропало.
        assertThat(text("ru", "attachment_removed", """{"filename":"смета.pdf"}"""))
            .isEqualTo("удалил(а) файл «смета.pdf»")
        assertThat(text("en", "attachment_removed", """{"filename":"estimate.pdf"}"""))
            .isEqualTo("removed the file “estimate.pdf”")
        assertThat(text("ru", "relation", """{"related":42}""")).isEqualTo("добавил(а) связь с #42")
        assertThat(text("en", "relation_removed", """{"related":42}"""))
            .isEqualTo("removed the link to #42")
        // Тег записан ИМЕНЕМ, а не id: строка истории должна читаться и после
        // того, как тег переименовали или удалили совсем.
        assertThat(text("ru", "tagged", """{"tag":"срочно"}""")).isEqualTo("навесил(а) тег «срочно»")
        // Этап — тоже названием: tasks.milestone_id стоит ON DELETE SET NULL, так
        // что id повис бы ровно тогда, когда старая запись и нужна.
        assertThat(text("ru", "milestone", """{"title":"Этап 1"}"""))
            .isEqualTo("привязал(а) к этапу «Этап 1»")
        assertThat(text("en", "milestone_removed", """{"title":"Stage 1"}"""))
            .isEqualTo("unlinked it from the milestone “Stage 1”")
        assertThat(text("en", "untagged", """{"tag":"urgent"}"""))
            .isEqualTo("removed the tag “urgent”")
    }

    @Test
    fun `a target the record does not carry leaves the bare sentence`() {
        // Фраза без цели читается как обычное событие; фраза с дыркой на месте
        // цели — как баг (тот же приём в NotificationText.kt).
        assertThat(text("ru", "renamed")).isEqualTo("переименовал(а) задачу")
        assertThat(text("ru", "moved")).isEqualTo("переместил(а) задачу")
        assertThat(text("ru", "attachment")).isEqualTo("прикрепил(а) файл")
        assertThat(text("ru", "attachment_removed")).isEqualTo("удалил(а) файл")
        assertThat(text("ru", "relation")).isEqualTo("добавил(а) связь")
        assertThat(text("ru", "tagged")).isEqualTo("навесил(а) тег")
        assertThat(text("ru", "milestone_removed")).isEqualTo("отвязал(а) от этапа")
    }

    @Test
    fun `a seeded column is captioned, a user-named one is verbatim`() {
        val seeded = """{"to":"В процессе","to_key":"in_progress"}"""
        assertThat(text("en", "moved", seeded)).isEqualTo("moved it to “In progress”")
        assertThat(text("ru", "moved", """{"to":"Бэклог"}""")).isEqualTo("переместил(а) → «Бэклог»")
    }

    @Test
    fun `priority reads as a label, an out-of-range level stays raw`() {
        assertThat(text("ru", "priority", """{"to":3}""")).isEqualTo("изменил(а) приоритет → Высокий")
        assertThat(text("en", "priority", """{"to":0}""")).isEqualTo("changed the priority to No priority")
        // Номер, который пользователь может назвать, полезнее молчаливого «нет
        // приоритета» — паритет с вебом.
        assertThat(text("ru", "priority", """{"to":9}""")).isEqualTo("изменил(а) приоритет → 9")
    }

    @Test
    fun `quick actions name the commands they ran`() {
        // Одна запись на комментарий, поэтому строка перечисляет команды: молчаливый
        // «/done» иначе читается как «ничего не произошло» (причина записана в вебе).
        assertThat(text("ru", "commands", """{"applied":["due","done"]}"""))
            .isEqualTo("выполнил(а) команды: /due, /done")
        assertThat(text("ru", "commands", """{"failed":["assignee"]}"""))
            .isEqualTo("не смог(ла) выполнить команды: /assignee")
        assertThat(text("en", "commands", """{"applied":["due"],"failed":["label"]}"""))
            .isEqualTo("ran /due (failed: /label)")
        assertThat(text("ru", "commands")).isEqualTo("выполнил(а) быстрые команды")
    }

    @Test
    fun `an integration is named through the shared source dictionary`() {
        assertThat(text("ru", "synced", """{"source":"gitlab"}"""))
            .isEqualTo("синхронизировал(а) с GitLab")
        assertThat(text("en", "synced", """{"source":"gitlab"}"""))
            .isEqualTo("synced it with GitLab")
    }

    @Test
    fun `a merge request line names the MR, its branch and what happened (#2964)`() {
        assertThat(text("ru", "code_change", """{"source":"gitlab","number":418,"state":"open","branch":"feat/board-v2"}"""))
            .isEqualTo("открыт !418 в feat/board-v2")
        assertThat(text("ru", "code_change", """{"number":418,"state":"merged"}""")).isEqualTo("смержен !418")
        assertThat(text("en", "code_change", """{"number":9,"state":"closed"}""")).isEqualTo("closed !9")
        assertThat(text("en", "code_change", """{"number":5}""")).isEqualTo("opened !5")
    }

    @Test
    fun `a kind newer than this build shows its key`() {
        // Сервер ушёл вперёд клиента — сырой ключ показать честнее, чем промолчать.
        assertThat(text("ru", "warp_drive")).isEqualTo("warp_drive")
    }
}
