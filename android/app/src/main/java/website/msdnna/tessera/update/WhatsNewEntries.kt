package website.msdnna.tessera.update

import website.msdnna.tessera.util.WhatsNewEntry
import website.msdnna.tessera.util.WhatsNewSpotlight

/**
 * Curated, user-facing "What's New" highlights for the **Android** app — newest
 * first, keyed by the `android/VERSION` they shipped in.
 *
 * Deliberately NOT the raw `android/CHANGELOG.md` (developer-facing and noisy):
 * only what is worth interrupting someone with after an update. A couple of short
 * bullets per release, visible features only, no Markdown — the card renders the
 * strings as-is.
 *
 * The web keeps its own list (`frontend/src/data/whatsNew.js`) because the two
 * components version independently: the same feature ships under different
 * numbers, and a shared list would show the wrong ones on one of the clients.
 * Add an entry here when an Android release ships something a user should notice
 * (the bump step is the place to remember — see the tessera-ship skill).
 *
 * `spotlight` queues a one-shot arrow at a sidebar item after the card is
 * dismissed; `navKey` must match a sidebar nav key (`activeNav`) — "documents",
 * "milestones", "notes", "reminders", "home", "admin" — or the hint is skipped.
 */
val WhatsNewEntries: List<WhatsNewEntry> = listOf(
    WhatsNewEntry(
        version = "0.70.0",
        date = "2026-08-20",
        title = "Задачи — как в вебе",
        items = listOf(
            "Описание задачи вынесено первой вкладкой — до комментариев и подзадач теперь один тап.",
            "Ответы в комментариях собираются веткой: отступ, сворачивание, «Ответить» подставляет автора.",
            "Тап по @упоминанию открывает карточку человека, а по «#123» — саму задачу.",
            "Редактор описаний с панелью форматирования, продолжением списков и полноэкранным режимом.",
        ),
    ),
    WhatsNewEntry(
        version = "0.69.0",
        date = "2026-08-17",
        title = "Документы рабочего пространства",
        items = listOf(
            "Новый раздел «Документы»: дерево вики-страниц с вложенностью, таблицами, чекбоксами и картинками.",
            "На телефоне документы пока только для чтения — редактирование остаётся в вебе.",
        ),
        spotlight = WhatsNewSpotlight(
            navKey = "documents",
            title = "Загляните в «Документы»",
            body = "Вики-страницы рабочего пространства доступны прямо с телефона.",
        ),
    ),
    WhatsNewEntry(
        version = "0.68.0",
        date = "2026-08-12",
        title = "Уведомления при закрытом приложении",
        items = listOf(
            "Уведомления доходят, даже когда приложение закрыто, и открывают нужную задачу по тапу.",
        ),
    ),
)
