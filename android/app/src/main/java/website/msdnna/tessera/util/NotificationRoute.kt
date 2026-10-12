package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Notification

/**
 * Куда ведёт тап по уведомлению — зеркало веба (`utils/notificationRoute.js`).
 *
 * Адресов два, и порядок проверки не произволен:
 *
 *  - приглашение в конференцию не указывает ни на какую задачу вовсе
 *    (`notifications.task_id` — внешний ключ на tasks, а звонок не задача), id
 *    звонка едет в `payload.conference_id`. Оно проверяется первым: откатиться
 *    такой строке не на что, доски у неё нет;
 *  - всё остальное про задачу и открывает её на своей доске.
 *
 * [None] — строка не ведёт никуда: отчёт синхронизации интеграции или задача,
 * доску которой список не смог подтянуть. Тап тогда только помечает прочитанным.
 */
sealed interface NotificationRoute {
    data class Conference(val conferenceId: String) : NotificationRoute

    data class Task(val boardId: String, val taskId: String) : NotificationRoute

    data object None : NotificationRoute

    companion object {
        fun of(n: Notification): NotificationRoute {
            val conf = n.payload?.conferenceId?.trim()
            if (!conf.isNullOrEmpty()) return Conference(conf)
            val task = n.taskId?.trim()
            val board = n.taskBoardId?.trim()
            if (!task.isNullOrEmpty() && !board.isNullOrEmpty()) return Task(board, task)
            return None
        }
    }
}
