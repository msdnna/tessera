package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import java.util.Calendar
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.model.Recurrence
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.columnCaption
import website.msdnna.tessera.util.dueShort
import website.msdnna.tessera.util.millisDayKey
import website.msdnna.tessera.util.millisToUtcIso
import website.msdnna.tessera.util.occurrenceKeys
import website.msdnna.tessera.util.parseInstantMillis

/** Заголовки столбцов календаря — тот же строчный словарь, что у оси таймлайна. */
@Composable
private fun weekdayLabels(weekStart: Int): List<String> {
    val days = stringArrayResource(R.array.dates_weekdays_short).toList()
    return if (weekStart == 0) listOf(days.last()) + days.dropLast(1) else days
}

// Списки уровня файла держат id ресурсов, а не готовый текст: со строками они бы
// застыли на языке первого рендера и не пережили смену языка в профиле.
// Frequency chips (empty = no repeat).
private val FreqChips = listOf(
    "" to R.string.recur_freq_none, "daily" to R.string.recur_freq_daily, "weekly" to R.string.recur_freq_weekly,
    "monthly" to R.string.recur_freq_monthly, "yearly" to R.string.recur_freq_yearly,
    "custom" to R.string.recur_freq_custom,
)
private val TriggerOptions = listOf(
    "complete" to R.string.recur_trigger_complete,
    "column" to R.string.recur_trigger_column,
    "schedule" to R.string.recur_trigger_schedule,
)
private val NotifyEnabledOpts = listOf<Pair<String?, Int>>(
    "inherit" to R.string.due_notify_inherit, "on" to R.string.due_notify_on, "off" to R.string.due_notify_off,
)
private val NotifyLeadOpts = listOf<Pair<String?, Int>>(
    "-1" to R.string.due_notify_inherit, "0" to R.string.due_notify_lead_0, "15" to R.string.due_notify_lead_15,
    "60" to R.string.due_notify_lead_60, "180" to R.string.due_notify_lead_180,
    "1440" to R.string.due_notify_lead_1440,
)
private val NotifyRepeatOpts = listOf<Pair<String?, Int>>(
    "-1" to R.string.due_notify_inherit, "0" to R.string.due_notify_repeat_0,
    "60" to R.string.due_notify_repeat_60, "180" to R.string.due_notify_repeat_180,
    "1440" to R.string.due_notify_repeat_1440,
)
private val UnitPlurals = mapOf(
    "daily" to R.plurals.recur_unit_daily,
    "weekly" to R.plurals.recur_unit_weekly,
    "monthly" to R.plurals.recur_unit_monthly,
    "yearly" to R.plurals.recur_unit_yearly,
)

// weekday index for the chips, 0=Sun..6=Sat, ordered by week-start.
private fun weekdayOrder(weekStart: Int): List<Int> =
    if (weekStart == 0) listOf(0, 1, 2, 3, 4, 5, 6) else listOf(1, 2, 3, 4, 5, 6, 0)

/** Чипы дней недели индексируются с воскресенья, а общий массив начинается с
 *  понедельника — отсюда сдвиг, а не второй такой же список в ресурсах. */
@Composable
private fun weekdayChipLabel(day: Int): String =
    stringArrayResource(R.array.calendar_weekdays_short)[(day + 6) % 7]

/**
 * Due-date picker with a real time-of-day and a full recurrence rule. The chosen
 * day+time is a real instant (local→UTC). Below the calendar/time it offers the
 * recurrence options (frequency incl. custom calendar-picked dates, interval,
 * weekly weekdays, trigger + columns, duplicate/forever/skip-weekend toggles) and
 * highlights upcoming occurrences on the grid. «Готово» commits due + rule.
 */
@Composable
fun DueDateTimePicker(
    initialIso: String?,
    initialStartIso: String?,
    initialRecurrence: Recurrence?,
    columns: List<BoardColumn>,
    notifyEnabled: Boolean?,
    notifyLead: Int?,
    notifyRepeat: Int?,
    onApply: (dueIso: String?, startIso: String?, recurrence: Recurrence?) -> Unit,
    onNotify: (lead: Int?, repeat: Int?, enabled: Boolean?) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    // Не `res`: ниже так зовётся @StringRes-элемент TriggerOptions, а это ресурсы
    // читателя — из них собираются подписи засеянных колонок (#2800).
    val colRes = LocalResources.current
    val ws = AppContainer.prefs.preferences.collectAsStateWithLifecycle(initialValue = Preferences()).value.weekStart
    // The calendar edits one endpoint at a time; `editTarget` says which. Each
    // endpoint keeps its own working calendar (start = left bar edge, due = right).
    val dueField = remember { DateField(initialDueCal(initialIso), initialIso != null) }
    val startField = remember { DateField(initialDueCal(initialStartIso), initialStartIso != null) }
    var editTarget by remember { mutableStateOf("due") } // 'start' | 'due'
    val active = if (editTarget == "start") startField else dueField

    var freq by remember { mutableStateOf(initialRecurrence?.freq ?: "") }
    var interval by remember { mutableIntStateOf(initialRecurrence?.interval?.coerceAtLeast(1) ?: 1) }
    val weekdaySet = remember { mutableStateListOf<Int>().apply { initialRecurrence?.weekdays?.let { addAll(it) } } }
    val customDates = remember { mutableStateListOf<String>().apply { initialRecurrence?.dates?.let { addAll(it) } } }
    var trigger by remember { mutableStateOf(initialRecurrence?.trigger ?: "complete") }
    var triggerColumn by remember { mutableStateOf(initialRecurrence?.triggerColumn) }
    var targetColumn by remember { mutableStateOf(initialRecurrence?.targetColumn) }
    var createNew by remember { mutableStateOf(initialRecurrence?.createNew ?: false) }
    var forever by remember { mutableStateOf(!(initialRecurrence?.once ?: false)) }
    var skipWeekends by remember { mutableStateOf(initialRecurrence?.skipWeekends ?: false) }

    // Notification overrides (string sentinels; "-1"/"inherit" = user default).
    var notifyEnabledSel by remember {
        mutableStateOf(if (notifyEnabled == null) "inherit" else if (notifyEnabled) "on" else "off")
    }
    var notifyLeadSel by remember { mutableStateOf((notifyLead ?: -1).toString()) }
    var notifyRepeatSel by remember { mutableStateOf((notifyRepeat ?: -1).toString()) }
    fun emitNotify() {
        val lead = notifyLeadSel.toIntOrNull()?.takeIf { it >= 0 }
        val repeat = notifyRepeatSel.toIntOrNull()?.takeIf { it >= 0 }
        val enabled = when (notifyEnabledSel) {
            "on" -> true
            "off" -> false
            else -> null
        }
        onNotify(lead, repeat, enabled)
    }

    fun stepMonth(delta: Int) {
        val m = active.month + delta
        active.year += Math.floorDiv(m, 12)
        active.month = Math.floorMod(m, 12)
        val maxDay = daysInMonth(active.year, active.month)
        if (active.day > maxDay) active.day = maxDay
    }

    fun buildRule(): Recurrence? {
        if (freq.isBlank()) return null
        if (freq == "custom" && customDates.isEmpty()) return null
        return Recurrence(
            freq = freq,
            interval = interval.coerceAtLeast(1),
            weekdays = if (freq == "weekly" && weekdaySet.isNotEmpty()) weekdaySet.sorted() else null,
            dates = if (freq == "custom") customDates.sorted() else null,
            trigger = if (trigger != "complete") trigger else null,
            triggerColumn = if (trigger == "column") triggerColumn else null,
            targetColumn = targetColumn,
            createNew = createNew,
            once = !forever,
            skipWeekends = skipWeekends && (freq == "daily" || freq == "weekly"),
        )
    }

    // The selected due instant (custom = earliest picked date at the chosen time).
    fun dueMillis(): Long? {
        if (freq == "custom") {
            val first = customDates.minOrNull() ?: return null
            val (yy, mm, dd) = first.split("-").map { it.toInt() }
            return localCalOf(yy, mm - 1, dd, dueField.hour, dueField.minute).timeInMillis
        }
        if (!dueField.set) return null
        return localCalOf(dueField.year, dueField.month, dueField.day, dueField.hour, dueField.minute).timeInMillis
    }
    fun startMillis(): Long? {
        if (!startField.set) return null
        return localCalOf(startField.year, startField.month, startField.day, startField.hour, startField.minute).timeInMillis
    }

    val occKeys = occurrenceKeys(buildRule(), dueMillis(), 24)
    // The actively-edited endpoint is the filled "selected" day; the other endpoint
    // is an outlined marker, with the days strictly between shaded as the bar span.
    val isCustomDue = editTarget == "due" && freq == "custom"
    val selectedKey = if (!isCustomDue && active.set) {
        millisDayKey(localCalOf(active.year, active.month, active.day, active.hour, active.minute).timeInMillis)
    } else {
        ""
    }
    val startDayMs = startMillis()?.let { dayStartMs(it) }
    val dueDayMs = dueMillis()?.let { dayStartMs(it) }
    val rangeLo = if (startDayMs != null && dueDayMs != null) minOf(startDayMs, dueDayMs) else null
    val rangeHi = if (startDayMs != null && dueDayMs != null) maxOf(startDayMs, dueDayMs) else null
    val altKey = if (editTarget == "start") {
        dueDayMs?.let { millisDayKey(it) } ?: ""
    } else {
        startDayMs?.let { millisDayKey(it) } ?: ""
    }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier
                .popupAppear(TransformOrigin.Center)
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface)
                .heightIn(max = 620.dp)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        ) {
            // ── start / due target tabs ──
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                TargetTab(
                    label = stringResource(R.string.due_picker_tab_start),
                    value = dueShort(LocalResources.current, startMillis()?.let { millisToUtcIso(it) })
                        .ifBlank { stringResource(R.string.due_picker_start_unset) },
                    active = editTarget == "start",
                    modifier = Modifier.weight(1f),
                ) { editTarget = "start" }
                Text("→", color = c.text3, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 6.dp))
                TargetTab(
                    label = stringResource(R.string.due_picker_tab_due),
                    value = dueShort(LocalResources.current, dueMillis()?.let { millisToUtcIso(it) })
                        .ifBlank { stringResource(R.string.due_picker_due_unset) },
                    active = editTarget == "due",
                    modifier = Modifier.weight(1f),
                ) { editTarget = "due" }
            }
            Spacer(Modifier.height(12.dp))

            // ── calendar header ──
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                NavBtn(double = true, forward = false) { active.year-- }
                NavBtn(double = false, forward = false) { stepMonth(-1) }
                Text(
                    "${stringArrayResource(R.array.calendar_months)[active.month]} ${active.year}",
                    color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.Center, modifier = Modifier.weight(1f),
                )
                NavBtn(double = false, forward = true) { stepMonth(1) }
                NavBtn(double = true, forward = true) { active.year++ }
            }
            Spacer(Modifier.height(8.dp))

            Row(Modifier.fillMaxWidth()) {
                weekdayLabels(ws).forEach { w ->
                    Text(
                        w, color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium,
                        textAlign = TextAlign.Center, modifier = Modifier.weight(1f),
                    )
                }
            }
            Spacer(Modifier.height(4.dp))

            val gridStart = monthGridStart(active.year, active.month, ws)
            for (week in 0 until 6) {
                Row(Modifier.fillMaxWidth()) {
                    for (dow in 0 until 7) {
                        val cell = (gridStart.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, week * 7 + dow) }
                        val inMonth = cell.get(Calendar.MONTH) == active.month
                        val cellDay = cell.get(Calendar.DAY_OF_MONTH)
                        val key = millisDayKey(cell.timeInMillis)
                        val cellDayMs = dayStartMs(cell.timeInMillis)
                        val isSel = key == selectedKey
                        val isEnd = !isSel && key.isNotEmpty() && key == altKey
                        val isRange = !isSel && !isEnd && rangeLo != null && rangeHi != null &&
                            cellDayMs > rangeLo && cellDayMs < rangeHi
                        DayCell(
                            day = cellDay,
                            inMonth = inMonth,
                            isSelected = isSel,
                            isEndpoint = isEnd,
                            isRange = isRange,
                            isOccurrence = !isSel && !isEnd && occKeys.contains(key),
                            modifier = Modifier.weight(1f),
                            onClick = {
                                if (isCustomDue) {
                                    if (customDates.contains(key)) customDates.remove(key) else customDates.add(key)
                                } else {
                                    active.year = cell.get(Calendar.YEAR)
                                    active.month = cell.get(Calendar.MONTH)
                                    active.day = cellDay
                                    active.set = true
                                }
                            },
                        )
                    }
                }
            }

            Spacer(Modifier.height(12.dp))
            // ── time steppers ──
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically) {
                Stepper(value = active.hour, onChange = {
                    active.hour = Math.floorMod(it, 24)
                    active.set = true
                })
                Text(":", color = c.text1, fontSize = 26.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 8.dp))
                Stepper(value = active.minute, onChange = {
                    active.minute = Math.floorMod(it, 60)
                    active.set = true
                }, step = 5)
            }

            Spacer(Modifier.height(14.dp))
            // ── recurrence ──
            Row(verticalAlignment = Alignment.CenterVertically) {
                IonIcon(Ion.REPEAT, size = 14.dp, tint = c.text3)
                Spacer(Modifier.width(6.dp))
                Text(
                    stringResource(R.string.due_picker_repeat),
                    color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium,
                )
            }
            Spacer(Modifier.height(8.dp))
            // Frequency chips wrap onto two rows.
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                FreqChips.chunked(3).forEach { rowChips ->
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        rowChips.forEach { (value, labelRes) ->
                            RecurChip(
                                label = stringResource(labelRes),
                                selected = freq == value,
                                modifier = Modifier.weight(1f),
                            ) { freq = value }
                        }
                    }
                }
            }

            if (freq.isNotBlank()) {
                if (freq != "custom") {
                    Spacer(Modifier.height(10.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(R.string.due_picker_every), color = c.text3, fontSize = 13.sp)
                        Spacer(Modifier.width(10.dp))
                        IntervalStepper(value = interval, onChange = { interval = it.coerceIn(1, 99) })
                        Spacer(Modifier.width(10.dp))
                        // Форму единицы («2 недели» / «5 недель») выбирает сам Android:
                        // ручной ruPlural здесь дал бы русские правила и на английском.
                        Text(
                            pluralStringResource(UnitPlurals.getValue(freq), interval),
                            color = c.text3, fontSize = 13.sp,
                        )
                    }
                }
                if (freq == "weekly") {
                    Spacer(Modifier.height(10.dp))
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        weekdayOrder(ws).forEach { d ->
                            RecurChip(
                                label = weekdayChipLabel(d),
                                selected = weekdaySet.contains(d),
                                modifier = Modifier.weight(1f),
                            ) { if (weekdaySet.contains(d)) weekdaySet.remove(d) else weekdaySet.add(d) }
                        }
                    }
                }
                if (freq == "custom") {
                    Spacer(Modifier.height(8.dp))
                    Text(stringResource(R.string.due_picker_custom_hint), color = c.text3, fontSize = 12.sp)
                }

                Spacer(Modifier.height(12.dp))
                SelectField(
                    stringResource(R.string.due_picker_trigger),
                    TriggerOptions.map { (v, res) -> v as String? to stringResource(res) },
                    trigger,
                ) { trigger = it ?: "complete" }

                if (trigger == "column") {
                    Spacer(Modifier.height(8.dp))
                    SelectField(
                        stringResource(R.string.due_picker_trigger_column),
                        columns.map { it.id as String? to columnCaption(colRes, it) },
                        triggerColumn,
                        placeholder = stringResource(R.string.due_picker_trigger_column_hint),
                    ) { triggerColumn = it }
                }

                Spacer(Modifier.height(8.dp))
                SelectField(
                    stringResource(R.string.due_picker_move_to),
                    listOf<Pair<String?, String>>(null to stringResource(R.string.due_picker_first_column)) +
                        columns.map { it.id as String? to columnCaption(colRes, it) },
                    targetColumn,
                ) { targetColumn = it }

                Spacer(Modifier.height(10.dp))
                ToggleRow(stringResource(R.string.due_picker_duplicate), createNew) { createNew = it }
                ToggleRow(stringResource(R.string.due_picker_forever), forever) { forever = it }
                if (freq == "daily" || freq == "weekly") {
                    ToggleRow(stringResource(R.string.due_picker_skip_weekends), skipWeekends) { skipWeekends = it }
                }
            }

            // ── notifications (shared with the card, like web) ──
            Spacer(Modifier.height(14.dp))
            Box(Modifier.fillMaxWidth().height(1.dp).background(SolidColor(c.border)))
            Spacer(Modifier.height(12.dp))
            SelectField(
                stringResource(R.string.due_notify_title),
                NotifyEnabledOpts.map { (v, res) -> v to stringResource(res) },
                notifyEnabledSel,
            ) {
                notifyEnabledSel = it ?: "inherit"
                emitNotify()
            }
            Spacer(Modifier.height(8.dp))
            SelectField(
                stringResource(R.string.due_notify_lead),
                NotifyLeadOpts.map { (v, res) -> v to stringResource(res) },
                notifyLeadSel,
            ) {
                notifyLeadSel = it ?: "-1"
                emitNotify()
            }
            Spacer(Modifier.height(8.dp))
            SelectField(
                stringResource(R.string.due_notify_repeat),
                NotifyRepeatOpts.map { (v, res) -> v to stringResource(res) },
                notifyRepeatSel,
            ) {
                notifyRepeatSel = it ?: "-1"
                emitNotify()
            }

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    stringResource(
                        if (editTarget == "start") R.string.due_picker_clear_start else R.string.due_picker_clear_due,
                    ),
                    color = c.text3, fontSize = 14.sp,
                    modifier = Modifier.clickableNoRipple {
                        // Clear only the actively-edited endpoint; stay open.
                        active.set = false
                        if (editTarget == "due" && freq == "custom") customDates.clear()
                    },
                )
                Spacer(Modifier.weight(1f))
                Text(
                    stringResource(R.string.common_cancel),
                    color = c.text3, fontSize = 14.sp,
                    modifier = Modifier.clickableNoRipple { onDismiss() },
                )
                Spacer(Modifier.width(18.dp))
                TButton(stringResource(R.string.common_done), onClick = {
                    val rule = buildRule()
                    onApply(
                        dueMillis()?.let { millisToUtcIso(it) },
                        startMillis()?.let { millisToUtcIso(it) },
                        rule,
                    )
                    onDismiss()
                })
            }
        }
    }
}

@Composable
private fun SelectField(
    title: String,
    options: List<Pair<String?, String>>,
    value: String?,
    placeholder: String = "—",
    onPick: (String?) -> Unit,
) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    val label = options.firstOrNull { it.first == value }?.second ?: placeholder
    Column {
        Text(title, color = c.text3, fontSize = 12.sp)
        Spacer(Modifier.height(3.dp))
        Box {
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt)
                    .clickableNoRipple { open = true }.padding(horizontal = 12.dp, vertical = 9.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(label, color = c.text1, fontSize = 13.sp, modifier = Modifier.weight(1f))
                IonIcon(Ion.CHEVRON_FORWARD, size = 13.dp, tint = c.text3, modifier = Modifier.graphicsLayer { rotationZ = 90f })
            }
            TDropdown(expanded = open, onDismiss = { open = false }) {
                options.forEach { (v, l) ->
                    Row(
                        Modifier.fillMaxWidth().clickableNoRipple {
                            onPick(v)
                            open = false
                        }
                            .padding(horizontal = 14.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(l, color = if (v == value) c.primary else c.text1, fontSize = 13.sp)
                    }
                }
            }
        }
    }
}

@Composable
private fun ToggleRow(label: String, checked: Boolean, onChange: (Boolean) -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clickableNoRipple { onChange(!checked) }.padding(vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = c.text2, fontSize = 13.sp, modifier = Modifier.weight(1f))
        TSwitch(checked = checked, onCheckedChange = onChange)
    }
}

@Composable
private fun RecurChip(label: String, selected: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        modifier.height(30.dp).clip(RoundedCornerShape(RadiusSm))
            .background(if (selected) accentGradient(c.primary) else SolidColor(c.surfaceAlt))
            .clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            label,
            color = if (selected) c.onPrimary else c.text2,
            fontSize = 12.sp,
            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
        )
    }
}

@Composable
private fun IntervalStepper(value: Int, onChange: (Int) -> Unit) {
    val c = Tessera.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        GlyphBtn("−") { onChange(value - 1) }
        Box(
            Modifier.size(width = 44.dp, height = 36.dp).clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt),
            contentAlignment = Alignment.Center,
        ) {
            Text(value.toString(), color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        }
        GlyphBtn("+") { onChange(value + 1) }
    }
}

@Composable
private fun GlyphBtn(glyph: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(34.dp).clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text(glyph, color = c.text2, fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
private fun Stepper(value: Int, onChange: (Int) -> Unit, step: Int = 1) {
    val c = Tessera.colors
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        StepArrow(up = true) { onChange(value + step) }
        Box(
            Modifier.size(width = 56.dp, height = 44.dp).clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt),
            contentAlignment = Alignment.Center,
        ) {
            Text(value.toString().padStart(2, '0'), color = c.text1, fontSize = 24.sp, fontWeight = FontWeight.SemiBold)
        }
        StepArrow(up = false) { onChange(value - step) }
    }
}

@Composable
private fun StepArrow(up: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(40.dp).clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        IonIcon(Ion.CHEVRON_FORWARD, size = 16.dp, tint = c.text2, modifier = Modifier.graphicsLayer { rotationZ = if (up) -90f else 90f })
    }
}

@Composable
private fun DayCell(
    day: Int,
    inMonth: Boolean,
    isSelected: Boolean,
    isEndpoint: Boolean,
    isRange: Boolean,
    isOccurrence: Boolean,
    modifier: Modifier,
    onClick: () -> Unit,
) {
    val c = Tessera.colors
    Box(modifier.aspectRatio(1f).clickableNoRipple(onClick = onClick), contentAlignment = Alignment.Center) {
        Box(
            Modifier.size(36.dp).clip(RoundedCornerShape(RadiusMd)).then(
                when {
                    isSelected -> Modifier.background(accentGradient(c.primary))

                    isEndpoint ->
                        Modifier
                            .background(SolidColor(c.primary.copy(alpha = 0.12f)))
                            .border(1.5.dp, c.primary, RoundedCornerShape(RadiusMd))

                    isRange -> Modifier.background(SolidColor(c.primary.copy(alpha = 0.09f)))

                    isOccurrence -> Modifier.background(SolidColor(c.primary.copy(alpha = 0.16f)))

                    else -> Modifier
                },
            ),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                day.toString(),
                color = when {
                    isSelected -> c.onPrimary
                    isEndpoint || isOccurrence -> c.primary
                    !inMonth -> c.text3
                    else -> c.text1
                },
                fontSize = 14.sp,
                fontWeight = if (isSelected || isEndpoint || isOccurrence) FontWeight.SemiBold else FontWeight.Normal,
            )
        }
    }
}

/** Small Start/Due tab over the calendar: label on top, current value below. */
@Composable
private fun TargetTab(label: String, value: String, active: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val c = Tessera.colors
    Column(
        modifier
            .clip(RoundedCornerShape(RadiusMd))
            .background(if (active) SolidColor(c.primary.copy(alpha = 0.14f)) else SolidColor(c.surfaceAlt))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 6.dp),
    ) {
        Text(label, color = if (active) c.primary else c.text3, fontSize = 11.sp)
        Text(
            value,
            color = if (active) c.primary else c.text2,
            fontSize = 12.sp,
            fontWeight = FontWeight.Medium,
        )
    }
}

/** A working calendar for one bar endpoint (start or due). `set` is whether the
 *  endpoint currently has a value; year/month still default to today for display. */
private class DateField(cal: Calendar, hasValue: Boolean) {
    var year by mutableIntStateOf(cal.get(Calendar.YEAR))
    var month by mutableIntStateOf(cal.get(Calendar.MONTH))
    var day by mutableIntStateOf(cal.get(Calendar.DAY_OF_MONTH))
    var hour by mutableIntStateOf(cal.get(Calendar.HOUR_OF_DAY))
    var minute by mutableIntStateOf(cal.get(Calendar.MINUTE))
    var set by mutableStateOf(hasValue)
}

/** Local-midnight epoch ms for the calendar day containing [millis]. */
private fun dayStartMs(millis: Long): Long =
    Calendar.getInstance().apply {
        timeInMillis = millis
        set(Calendar.HOUR_OF_DAY, 0)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }.timeInMillis

@Composable
private fun NavBtn(double: Boolean, forward: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.size(34.dp).clip(RoundedCornerShape(RadiusSm)).clickableNoRipple(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy((-8).dp)) {
            repeat(if (double) 2 else 1) {
                IonIcon(
                    Ion.CHEVRON_FORWARD, size = 16.dp, tint = c.text2,
                    modifier = if (forward) Modifier else Modifier.graphicsLayer { scaleX = -1f },
                )
            }
        }
    }
}

// ── local-zone Calendar helpers (real instants, no java.time on minSdk 24) ──

// Initial picker state from the stored due ISO:
//  - none → today at 00:00 (no surprise "current time");
//  - a pure UTC-midnight value (GitLab/legacy date-only) → that calendar date at
//    local 00:00 (not the tz-shifted 03:00 / wrong day);
//  - otherwise a real instant → local zone.
private fun initialDueCal(iso: String?): Calendar {
    if (iso != null && iso.length >= 19 && iso.substring(11, 19) == "00:00:00") {
        return Calendar.getInstance().apply {
            clear()
            set(iso.substring(0, 4).toInt(), iso.substring(5, 7).toInt() - 1, iso.substring(8, 10).toInt(), 0, 0, 0)
        }
    }
    val millis = parseInstantMillis(iso)
    return Calendar.getInstance().apply {
        if (millis != null) {
            timeInMillis = millis
        } else {
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
        }
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }
}

private fun localCalOf(year: Int, month: Int, day: Int, hour: Int, minute: Int): Calendar =
    Calendar.getInstance().apply {
        clear()
        set(year, month, day, hour, minute, 0)
    }

private fun daysInMonth(year: Int, month: Int): Int =
    Calendar.getInstance().apply {
        clear()
        set(year, month, 1)
    }.getActualMaximum(Calendar.DAY_OF_MONTH)

private fun monthGridStart(year: Int, month: Int, weekStart: Int): Calendar {
    val first = Calendar.getInstance().apply {
        clear()
        set(year, month, 1)
    }
    val firstDow = if (weekStart == 0) Calendar.SUNDAY else Calendar.MONDAY
    val leading = (first.get(Calendar.DAY_OF_WEEK) - firstDow + 7) % 7
    return (first.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, -leading) }
}
