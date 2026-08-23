package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Reminder
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.ReminderDateTimePicker
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.viewmodels.ReminderViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.localDateTimeLabel
import website.msdnna.tessera.util.parseInstantMillis

/**
 * Personal reminders list + composer (web `RemindersView`). Adding/toggling/
 * deleting also re-arms the local alarms (see [ReminderViewModel]) so the
 * device actually fires them — the Android-specific delivery the web lacked.
 */
@Composable
fun RemindersScreen() {
    val c = Tessera.colors
    val vm: ReminderViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    var message by remember { mutableStateOf("") }
    var pickedIso by remember { mutableStateOf<String?>(null) }
    var showPicker by remember { mutableStateOf(false) }

    Column(Modifier.fillMaxSize().background(c.bg)) {
        // ── composer ──
        Column(Modifier.fillMaxWidth().padding(16.dp)) {
            TTextField(value = message, onValueChange = { message = it }, placeholder = stringResource(R.string.reminders_message_placeholder))
            Spacer(Modifier.height(10.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Row(
                    Modifier.weight(1f).clip(RoundedCornerShape(RadiusMd))
                        .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                        .clickableNoRipple { showPicker = true }
                        .padding(horizontal = 12.dp, vertical = 11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    IonIcon(Ion.TIME, size = 16.dp, tint = c.text3)
                    Spacer(Modifier.width(8.dp))
                    Text(
                        pickedIso?.let { localDateTimeLabel(it) } ?: stringResource(R.string.reminders_pick_time),
                        color = if (pickedIso != null) c.text1 else c.placeholder,
                        fontSize = 14.sp,
                    )
                }
                Spacer(Modifier.width(10.dp))
                TButton(
                    stringResource(R.string.reminders_add),
                    enabled = message.isNotBlank() && pickedIso != null,
                    onClick = {
                        val iso = pickedIso ?: return@TButton
                        vm.create(iso, message.trim())
                        message = ""
                        pickedIso = null
                    },
                )
            }
        }

        when {
            state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                TesseraLoader()
            }

            state.items.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    IonIcon(Ion.ALARM, size = 40.dp, tint = c.text3)
                    Spacer(Modifier.height(10.dp))
                    Text(stringResource(R.string.reminders_empty), color = c.text3, fontSize = 14.sp)
                }
            }

            else -> LazyColumn(
                Modifier.fillMaxSize().padding(horizontal = 16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(state.items, key = { it.id }) { reminder ->
                    ReminderRow(
                        reminder = reminder,
                        onToggle = { vm.toggleDone(reminder) },
                        onDelete = { vm.delete(reminder) },
                    )
                }
                item { Spacer(Modifier.height(16.dp)) }
            }
        }
    }

    if (showPicker) {
        ReminderDateTimePicker(
            initialIso = pickedIso,
            onPick = { pickedIso = it },
            onDismiss = { showPicker = false },
        )
    }
}

@Composable
private fun ReminderRow(reminder: Reminder, onToggle: () -> Unit, onDelete: () -> Unit) {
    val c = Tessera.colors
    var confirmDelete by remember { mutableStateOf(false) }
    val overdue = !reminder.done &&
        (parseInstantMillis(reminder.remindAt) ?: Long.MAX_VALUE) < System.currentTimeMillis()
    val borderColor = if (overdue) TesseraDanger else c.border

    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.cardSurface)
            .border(1.dp, borderColor, RoundedCornerShape(RadiusMd)).padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // Done toggle: a check-circle when done, an empty ellipse otherwise.
        Box(
            Modifier.size(24.dp).clickableNoRipple(onClick = onToggle),
            contentAlignment = Alignment.Center,
        ) {
            if (reminder.done) {
                IonIcon(Ion.CHECK_CIRCLE, size = 22.dp, tint = c.primary, gradient = true)
            } else {
                IonIcon(Ion.ELLIPSE, size = 22.dp, tint = c.text3)
            }
        }
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                reminder.message.ifBlank { stringResource(R.string.reminders_untitled) },
                color = if (reminder.done) c.text3 else c.text1,
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                textDecoration = if (reminder.done) TextDecoration.LineThrough else null,
            )
            Spacer(Modifier.height(3.dp))
            Text(
                localDateTimeLabel(reminder.remindAt),
                color = if (overdue) TesseraDanger else c.text3,
                fontSize = 12.sp,
            )
        }
        Box {
            IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 18.dp, tint = c.text3)
            TConfirmPopover(
                expanded = confirmDelete,
                message = stringResource(R.string.reminders_delete_confirm),
                onConfirm = {
                    confirmDelete = false
                    onDelete()
                },
                onDismiss = { confirmDelete = false },
            )
        }
    }
}
