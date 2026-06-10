package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.CreateReminderRequest
import website.msdnna.tessera.data.model.Reminder
import website.msdnna.tessera.data.model.UpdateReminderRequest

/** CRUD over personal reminders (user-scoped, not workspace-scoped). */
class ReminderRepository {
    private val api get() = AppContainer.api()

    suspend fun list(): List<Reminder> = api.reminders().orEmpty()

    suspend fun create(remindAt: String, message: String, taskId: String? = null): Reminder =
        api.createReminder(CreateReminderRequest(remindAt = remindAt, message = message, taskId = taskId))

    suspend fun update(reminder: Reminder, remindAt: String, message: String, done: Boolean): Reminder =
        api.updateReminder(reminder.id, UpdateReminderRequest(remindAt = remindAt, message = message, done = done))

    suspend fun delete(id: String) = api.deleteReminder(id)
}
