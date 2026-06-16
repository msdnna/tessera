package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.ChannelRequest
import website.msdnna.tessera.data.model.NotificationChannel
import website.msdnna.tessera.data.model.NotificationRoute
import website.msdnna.tessera.data.model.RouteMatcher
import website.msdnna.tessera.data.model.RouteOptions
import website.msdnna.tessera.data.model.RouteRequest
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.dashedBorder
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.NotificationSettingsViewModel
import website.msdnna.tessera.util.Ion

private val TYPE_LABELS = mapOf(
    "email" to "Email", "telegram" to "Telegram", "webhook" to "Webhook",
    "shoutrrr" to "Shoutrrr", "device" to "Системные уведомления",
)
private val ADDABLE_TYPES = listOf(
    "email" to "Email", "telegram" to "Telegram", "webhook" to "Webhook", "shoutrrr" to "Shoutrrr (любой сервис)",
)
private val KIND_OPTIONS = listOf(
    "assigned" to "Назначения", "comment" to "Комментарии", "mention" to "Упоминания",
    "due_soon" to "Дедлайны", "reminder" to "Напоминания",
)
private val LEAD_OPTIONS = listOf(
    0 to "В срок", 15 to "За 15 мин", 30 to "За 30 мин", 60 to "За час", 180 to "За 3 часа", 1440 to "За день",
)
private val REPEAT_OPTIONS = listOf(
    0 to "Однократно", 60 to "Каждый час", 180 to "Каждые 3 часа", 360 to "Каждые 6 часов", 1440 to "Каждый день",
)
private val DIGEST_OPTIONS = listOf(0 to "Выключено", 5 to "5 минут", 15 to "15 минут", 30 to "30 минут", 60 to "Раз в час")
private val TIME_OPTIONS = (0 until 48).map { i ->
    val m = i * 30
    m to "%02d:%02d".format(m / 60, m % 60)
}
private val TEMPLATE_FIELDS = listOf(
    "{{.Text}}", "{{.Title}}", "{{.TaskNumber}}", "{{.TaskTitle}}", "{{.Actor}}", "{{.Workspace}}", "{{.Link}}",
)

/** Notification settings: delivery channels, routing rules and per-user schedule.
 *  Mirrors the web `NotificationSettings.vue` (without the template editor). */
@Composable
fun NotificationSettingsScreen(
    deviceId: String,
    vm: NotificationSettingsViewModel = viewModel(key = "notif-settings"),
) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { vm.loadAll() }

    var channelEdit by remember { mutableStateOf<ChannelEdit?>(null) }
    var routeEdit by remember { mutableStateOf<RouteEdit?>(null) }

    if (state.loading) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }
        return
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp)) {
        Text("Уведомления", color = c.text1, fontSize = 20.sp, fontWeight = FontWeight.Bold)
        Text(
            "Внутренние уведомления приходят всегда. Здесь — внешние каналы и правила доставки.",
            color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp),
        )
        state.error?.let {
            Spacer(Modifier.height(10.dp))
            TFormError(it)
        }
        state.message?.let {
            Spacer(Modifier.height(8.dp))
            Text(it, color = c.primary, fontSize = 13.sp)
        }
        Spacer(Modifier.height(16.dp))

        // ── Channels ──────────────────────────────────────────────────────────
        SectionLabel("Каналы")
        state.channels.forEach { ch ->
            ChannelRow(
                ch = ch,
                isThisDevice = ch.type == "device" && ch.config?.get("device_id") == deviceId,
                testing = state.testingId == ch.id,
                onToggle = { vm.toggleChannel(ch) },
                onTest = { vm.testChannel(ch.id) },
                onEdit = { channelEdit = ChannelEdit(ch) },
                onDelete = { vm.deleteChannel(ch.id) },
            )
            Spacer(Modifier.height(8.dp))
        }
        if (channelEdit == null) {
            DashedAddButton("Канал") { channelEdit = ChannelEdit(null) }
        } else {
            ChannelEditor(
                edit = channelEdit!!,
                saving = state.saving,
                vm = vm,
                onSave = { vm.saveChannel(channelEdit!!.id, channelEdit!!.toRequest()) { channelEdit = null } },
                onCancel = { channelEdit = null },
            )
        }

        Spacer(Modifier.height(20.dp))

        // ── Routes ────────────────────────────────────────────────────────────
        SectionLabel("Правила маршрутизации")
        Text(
            "Сверху вниз — срабатывает первое подходящее.",
            color = c.text3, fontSize = 12.sp, modifier = Modifier.padding(bottom = 8.dp),
        )
        state.routes.forEach { r ->
            RouteRow(
                r = r,
                channels = state.channels,
                onToggle = { vm.toggleRoute(r) },
                onEdit = { routeEdit = RouteEdit(r) },
                onDelete = { vm.deleteRoute(r.id) },
            )
            Spacer(Modifier.height(8.dp))
        }
        if (routeEdit == null) {
            if (state.channels.isNotEmpty()) DashedAddButton("Правило") { routeEdit = RouteEdit(null) }
        } else {
            RouteEditor(
                edit = routeEdit!!,
                channels = state.channels,
                workspaces = state.workspaces.map { it.id to it.name },
                saving = state.saving,
                onSave = { vm.saveRoute(routeEdit!!.id, routeEdit!!.toRequest()) { routeEdit = null } },
                onCancel = { routeEdit = null },
            )
        }

        Spacer(Modifier.height(20.dp))

        // ── Schedule ──────────────────────────────────────────────────────────
        ScheduleCard(state.prefs, state.saving, onSave = { vm.savePrefs(it) })
        Spacer(Modifier.height(40.dp))
    }
}

@Composable
private fun ChannelRow(
    ch: NotificationChannel,
    isThisDevice: Boolean,
    testing: Boolean,
    onToggle: () -> Unit,
    onTest: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    val c = Tessera.colors
    TCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        ch.label.ifBlank { TYPE_LABELS[ch.type] ?: ch.type },
                        color = c.text1, fontSize = 14.sp, fontWeight = FontWeight.Medium,
                    )
                    if (ch.verified) {
                        Spacer(Modifier.width(6.dp))
                        Pill("проверен", c.primary)
                    }
                    if (isThisDevice) {
                        Spacer(Modifier.width(6.dp))
                        Pill("это устройство", c.primary)
                    }
                }
                Text(TYPE_LABELS[ch.type] ?: ch.type, color = c.text3, fontSize = 12.sp)
            }
            TSwitch(ch.enabled, { onToggle() })
            if (ch.type != "device") {
                Spacer(Modifier.width(6.dp))
                IonIcon(
                    if (testing) Ion.REFRESH else Ion.SEND, size = 16.dp, tint = c.text3,
                    modifier = Modifier.clickableNoRipple(onClick = onTest),
                )
            }
            Spacer(Modifier.width(10.dp))
            IonIcon(Ion.PENCIL, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onEdit))
            Spacer(Modifier.width(10.dp))
            IonIcon(Ion.TRASH, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onDelete))
        }
    }
}

@Composable
private fun ChannelEditor(
    edit: ChannelEdit,
    saving: Boolean,
    vm: NotificationSettingsViewModel,
    onSave: () -> Unit,
    onCancel: () -> Unit,
) {
    val c = Tessera.colors
    val isNew = edit.id == null
    TCard {
        Column {
            if (isNew) {
                Field("Тип") {
                    LocalSelect(ADDABLE_TYPES.find { it.first == edit.type }?.second ?: "—", ADDABLE_TYPES) { edit.type = it }
                }
            } else {
                Text(TYPE_LABELS[edit.type] ?: edit.type, color = c.text2, fontSize = 13.sp)
                Spacer(Modifier.height(6.dp))
            }
            TTextField(edit.label, { edit.label = it }, label = "Название", placeholder = "Напр. «Мой телеграм»")
            Spacer(Modifier.height(8.dp))
            when (edit.type) {
                "email" -> TTextField(edit.address, { edit.address = it }, label = "Адрес", placeholder = "you@example.com")

                "telegram" -> {
                    TTextField(edit.chatId, { edit.chatId = it }, label = "Chat ID", placeholder = "123456789")
                    Spacer(Modifier.height(8.dp))
                    TTextField(
                        edit.botToken, { edit.botToken = it }, label = "Bot token", isPassword = true,
                        placeholder = if (isNew) "123456:ABC-…" else "пусто — не менять",
                    )
                }

                "webhook" -> {
                    TTextField(edit.url, { edit.url = it }, label = "URL", placeholder = "https://…")
                    Spacer(Modifier.height(8.dp))
                    TTextField(edit.method, { edit.method = it }, label = "Метод", placeholder = "POST")
                    Spacer(Modifier.height(8.dp))
                    TTextField(
                        edit.authHeader, { edit.authHeader = it }, label = "Authorization (необязательно)", isPassword = true,
                        placeholder = if (isNew) "Bearer …" else "пусто — не менять",
                    )
                }

                "shoutrrr" -> {
                    TTextField(
                        edit.shoutrrrUrl, { edit.shoutrrrUrl = it }, label = "Service URL", isPassword = true,
                        placeholder = if (isNew) "slack://… · discord://… · ntfy://…" else "пусто — не менять",
                    )
                }

                "device" -> Text(
                    "Это устройство/браузер. Системные уведомления приходят, когда приложение открыто.",
                    color = c.text3, fontSize = 12.sp,
                )
            }
            if (edit.type != "device") {
                Spacer(Modifier.height(10.dp))
                TemplateEditor(edit, vm)
            }
            Spacer(Modifier.height(10.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Включён", color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
                TSwitch(edit.enabled, { edit.enabled = it })
            }
            Spacer(Modifier.height(12.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                TButton("Отмена", onClick = onCancel, kind = TButtonKind.Secondary, modifier = Modifier.weight(1f))
                TButton("Сохранить", onClick = onSave, loading = saving, modifier = Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun TemplateEditor(edit: ChannelEdit, vm: NotificationSettingsViewModel) {
    val c = Tessera.colors
    var preview by remember { mutableStateOf<String?>(null) }
    var previewErr by remember { mutableStateOf<String?>(null) }
    Text("Шаблон сообщения (пусто = по умолчанию)", color = c.text2, fontSize = 13.sp)
    Spacer(Modifier.height(6.dp))
    TTextField(edit.template, { edit.template = it }, singleLine = false, placeholder = "{{.Text}}")
    Spacer(Modifier.height(6.dp))
    FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        TEMPLATE_FIELDS.forEach { token -> ToggleChip(token, false) { edit.template += token } }
    }
    Spacer(Modifier.height(8.dp))
    TButton(
        "Предпросмотр", kind = TButtonKind.Secondary,
        onClick = {
            vm.previewTemplate(edit.template) { text, err ->
                preview = text
                previewErr = err
            }
        },
    )
    previewErr?.let {
        Spacer(Modifier.height(6.dp))
        TFormError(it)
    }
    preview?.let {
        Spacer(Modifier.height(6.dp))
        Box(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusSm)).padding(10.dp),
        ) { Text(it, color = c.text1, fontSize = 13.sp) }
    }
}

@Composable
private fun RouteRow(
    r: NotificationRoute,
    channels: List<NotificationChannel>,
    onToggle: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    val c = Tessera.colors
    val kinds = r.matcher.kinds
    val kindsLabel = if (kinds.isNullOrEmpty()) "Любые события" else kinds.joinToString(", ") { k -> KIND_OPTIONS.find { it.first == k }?.second ?: k }
    val target = if (r.options.mute) "заглушено" else
        r.channelIds.mapNotNull { id -> channels.find { it.id == id }?.let { it.label.ifBlank { TYPE_LABELS[it.type] ?: it.type } } }
            .joinToString(", ").ifBlank { "— каналы не выбраны" }
    TCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(kindsLabel, color = c.text1, fontSize = 14.sp, fontWeight = FontWeight.Medium)
                Text("→ $target", color = c.text3, fontSize = 12.sp)
            }
            TSwitch(r.enabled, { onToggle() })
            Spacer(Modifier.width(10.dp))
            IonIcon(Ion.PENCIL, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onEdit))
            Spacer(Modifier.width(10.dp))
            IonIcon(Ion.TRASH, size = 16.dp, tint = c.text3, modifier = Modifier.clickableNoRipple(onClick = onDelete))
        }
    }
}

@Composable
private fun RouteEditor(
    edit: RouteEdit,
    channels: List<NotificationChannel>,
    workspaces: List<Pair<String, String>>,
    saving: Boolean,
    onSave: () -> Unit,
    onCancel: () -> Unit,
) {
    val c = Tessera.colors
    TCard {
        Column {
            Text("События (пусто = любые)", color = c.text2, fontSize = 13.sp)
            Spacer(Modifier.height(6.dp))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                KIND_OPTIONS.forEach { (k, label) ->
                    ToggleChip(label, edit.kinds.contains(k)) {
                        if (edit.kinds.contains(k)) edit.kinds.remove(k) else edit.kinds.add(k)
                    }
                }
            }
            Spacer(Modifier.height(10.dp))
            Text("Пространство", color = c.text2, fontSize = 13.sp)
            Spacer(Modifier.height(6.dp))
            val wsOpts = listOf("" to "Все пространства") + workspaces
            LocalSelect(wsOpts.find { it.first == (edit.workspaceId ?: "") }?.second ?: "Все пространства", wsOpts) {
                edit.workspaceId = it.ifBlank { null }
            }
            Spacer(Modifier.height(10.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Заглушить (не отправлять)", color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
                TSwitch(edit.mute, { edit.mute = it })
            }
            if (!edit.mute) {
                Spacer(Modifier.height(10.dp))
                Text("Каналы доставки", color = c.text2, fontSize = 13.sp)
                Spacer(Modifier.height(6.dp))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    channels.forEach { ch ->
                        val name = ch.label.ifBlank { TYPE_LABELS[ch.type] ?: ch.type }
                        ToggleChip(name, edit.channelIds.contains(ch.id)) {
                            if (edit.channelIds.contains(ch.id)) edit.channelIds.remove(ch.id) else edit.channelIds.add(ch.id)
                        }
                    }
                }
            }
            Spacer(Modifier.height(12.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                TButton("Отмена", onClick = onCancel, kind = TButtonKind.Secondary, modifier = Modifier.weight(1f))
                TButton("Сохранить", onClick = onSave, loading = saving, modifier = Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun ScheduleCard(
    prefs: website.msdnna.tessera.data.model.NotificationPrefs,
    saving: Boolean,
    onSave: (website.msdnna.tessera.data.model.NotificationPrefs) -> Unit,
) {
    val c = Tessera.colors
    var dueEnabled by remember(prefs) { mutableStateOf(prefs.dueEnabled) }
    var lead by remember(prefs) { mutableStateOf(prefs.dueLeadMinutes) }
    var repeat by remember(prefs) { mutableStateOf(prefs.dueRepeatMinutes) }
    var reminderEnabled by remember(prefs) { mutableStateOf(prefs.reminderEnabled) }
    var digest by remember(prefs) { mutableStateOf(prefs.digestMinutes) }
    var quietEnabled by remember(prefs) { mutableStateOf(prefs.quietEnabled) }
    var quietStart by remember(prefs) { mutableStateOf(prefs.quietStartMinutes) }
    var quietEnd by remember(prefs) { mutableStateOf(prefs.quietEndMinutes) }

    SectionLabel("Дедлайны и напоминания")
    TCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Уведомлять о дедлайнах", color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
                TSwitch(dueEnabled, { dueEnabled = it })
            }
            IntSelect("Напоминать", lead, LEAD_OPTIONS) { lead = it }
            IntSelect("Повтор", repeat, REPEAT_OPTIONS) { repeat = it }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Напоминания во внешние каналы", color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
                TSwitch(reminderEnabled, { reminderEnabled = it })
            }
            IntSelect("Дайджест", digest, DIGEST_OPTIONS) { digest = it }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Тихие часы", color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
                TSwitch(quietEnabled, { quietEnabled = it })
            }
            if (quietEnabled) {
                IntSelect("С", quietStart, TIME_OPTIONS) { quietStart = it }
                IntSelect("До", quietEnd, TIME_OPTIONS) { quietEnd = it }
            }
            Spacer(Modifier.height(12.dp))
            TButton(
                "Сохранить", loading = saving, modifier = Modifier.fillMaxWidth(),
                onClick = {
                    val tz = prefs.quietTz.ifBlank { java.util.TimeZone.getDefault().id }
                    onSave(
                        prefs.copy(
                            dueEnabled = dueEnabled, dueLeadMinutes = lead, dueRepeatMinutes = repeat,
                            reminderEnabled = reminderEnabled, digestMinutes = digest,
                            quietEnabled = quietEnabled, quietStartMinutes = quietStart, quietEndMinutes = quietEnd,
                            quietTz = tz,
                        ),
                    )
                },
            )
        }
    }
}

// ── small helpers ────────────────────────────────────────────────────────────

@Composable
private fun Pill(text: String, color: androidx.compose.ui.graphics.Color) {
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm)).background(color.copy(alpha = 0.15f)).padding(horizontal = 6.dp, vertical = 1.dp),
    ) { Text(text, color = color, fontSize = 10.sp, fontWeight = FontWeight.SemiBold) }
}

@Composable
private fun ToggleChip(label: String, selected: Boolean, onToggle: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm))
            .background(if (selected) c.primary.copy(alpha = 0.16f) else c.surface)
            .border(1.dp, if (selected) c.primary else c.border, RoundedCornerShape(RadiusSm))
            .clickableNoRipple(onClick = onToggle)
            .padding(horizontal = 10.dp, vertical = 6.dp),
    ) {
        Text(label, color = if (selected) c.primary else c.text2, fontSize = 12.sp, fontWeight = FontWeight.Medium)
    }
}

@Composable
private fun DashedAddButton(label: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.clip(RoundedCornerShape(RadiusSm)).dashedBorder(c.primary, RadiusSm)
            .clickableNoRipple(onClick = onClick).padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        IonIcon(Ion.ADD, size = 14.dp, tint = c.primary, gradient = true)
        Text("Добавить $label", style = TextStyle(brush = accentGradient(c.primary)), fontSize = 13.sp, fontWeight = FontWeight.Medium)
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(text.uppercase(), color = Tessera.colors.text3, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, letterSpacing = 0.4.sp)
    Spacer(Modifier.height(8.dp))
}

/** A [Field] whose control is an int-keyed [LocalSelect] (lead/repeat/digest/time). */
@Composable
private fun IntSelect(label: String, value: Int, options: List<Pair<Int, String>>, onSelect: (Int) -> Unit) {
    Field(label) {
        LocalSelect(
            options.find { it.first == value }?.second ?: "—",
            options.map { it.first.toString() to it.second },
        ) { onSelect(it.toInt()) }
    }
}

@Composable
private fun Field(label: String, content: @Composable () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, color = Tessera.colors.text2, fontSize = 13.sp, modifier = Modifier.width(140.dp))
        Box(Modifier.weight(1f)) { content() }
    }
}

/** Bordered dropdown "select" (copy of the GitLab screen's, kept private here). */
@Composable
private fun LocalSelect(value: String, options: List<Pair<String, String>>, onSelect: (String) -> Unit) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm)).background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusSm)).clickableNoRipple { open = true }
                .padding(horizontal = 10.dp, vertical = 9.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(value, color = c.text1, fontSize = 13.sp, modifier = Modifier.weight(1f))
            IonIcon(Ion.CHEVRON_DOWN, size = 13.dp, tint = c.text3)
        }
        TDropdown(expanded = open, onDismiss = { open = false }) {
            options.forEach { (k, label) ->
                TMenuItem(label, onClick = {
                    open = false
                    onSelect(k)
                })
            }
        }
    }
}

// ── editable holders (Compose-observable) ────────────────────────────────────

private class ChannelEdit(c: NotificationChannel?) {
    val id = c?.id
    var type by mutableStateOf(c?.type ?: "email")
    var label by mutableStateOf(c?.label ?: "")
    var address by mutableStateOf(c?.config?.get("address") ?: "")
    var chatId by mutableStateOf(c?.config?.get("chat_id") ?: "")
    var url by mutableStateOf(c?.config?.get("url") ?: "")
    var method by mutableStateOf(c?.config?.get("method") ?: "")
    var botToken by mutableStateOf("")
    var authHeader by mutableStateOf("")
    var shoutrrrUrl by mutableStateOf("")
    var template by mutableStateOf(c?.template ?: "")
    var enabled by mutableStateOf(c?.enabled ?: true)
    private val existingConfig = c?.config ?: emptyMap()

    fun toRequest(): ChannelRequest {
        val config = mutableMapOf<String, String>()
        val secret = mutableMapOf<String, String>()
        when (type) {
            "email" -> config["address"] = address.trim()

            "telegram" -> {
                config["chat_id"] = chatId.trim()
                if (botToken.isNotBlank()) secret["bot_token"] = botToken.trim()
            }

            "webhook" -> {
                config["url"] = url.trim()
                if (method.isNotBlank()) config["method"] = method.trim()
                if (authHeader.isNotBlank()) secret["auth_header"] = authHeader.trim()
            }

            "shoutrrr" -> if (shoutrrrUrl.isNotBlank()) secret["url"] = shoutrrrUrl.trim()

            "device" -> existingConfig.forEach { (k, v) -> config[k] = v } // keep device_id/platform
        }
        return ChannelRequest(type, label.trim(), config, secret, template, enabled)
    }
}

private class RouteEdit(r: NotificationRoute?) {
    val id = r?.id
    private val position = r?.position
    val kinds = mutableStateListOf<String>().apply { addAll(r?.matcher?.kinds ?: emptyList()) }
    var workspaceId by mutableStateOf(r?.matcher?.workspaceId)
    val channelIds = mutableStateListOf<String>().apply { addAll(r?.channelIds ?: emptyList()) }
    var mute by mutableStateOf(r?.options?.mute ?: false)
    var enabled by mutableStateOf(r?.enabled ?: true)

    fun toRequest(): RouteRequest {
        val matcher = RouteMatcher(kinds = kinds.toList().ifEmpty { null }, workspaceId = workspaceId)
        return RouteRequest(matcher, if (mute) emptyList() else channelIds.toList(), RouteOptions(mute), enabled, position)
    }
}
