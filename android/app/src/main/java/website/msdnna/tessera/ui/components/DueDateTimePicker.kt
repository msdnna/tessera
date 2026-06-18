package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import java.util.Calendar
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
import website.msdnna.tessera.util.millisDayKey
import website.msdnna.tessera.util.millisToUtcIso
import website.msdnna.tessera.util.occurrenceKeys
import website.msdnna.tessera.util.parseInstantMillis

private val MonthsFull = listOf(
    "январь", "февраль", "март", "апрель", "май", "июнь",
    "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь",
)
private val Weekdays = listOf("пн", "вт", "ср", "чт", "пт", "сб", "вс")

private fun weekdayLabels(weekStart: Int): List<String> =
    if (weekStart == 0) listOf(Weekdays.last()) + Weekdays.dropLast(1) else Weekdays

// Frequency chips (empty = no repeat).
private val FreqChips = listOf(
    "" to "Нет", "daily" to "День", "weekly" to "Неделя",
    "monthly" to "Месяц", "yearly" to "Год", "custom" to "Выборочно",
)
private val TriggerOptions = listOf(
    "complete" to "При завершении", "column" to "При переходе в колонку", "schedule" to "По расписанию",
)
private val UnitForms = mapOf(
    "daily" to listOf("день", "дня", "дней"),
    "weekly" to listOf("неделю", "недели", "недель"),
    "monthly" to listOf("месяц", "месяца", "месяцев"),
    "yearly" to listOf("год", "года", "лет"),
)

private fun ruPlural(n: Int, forms: List<String>): String {
    val m10 = n % 10
    val m100 = n % 100
    return when {
        m10 == 1 && m100 != 11 -> forms[0]
        m10 in 2..4 && (m100 < 10 || m100 >= 20) -> forms[1]
        else -> forms[2]
    }
}

// weekday index for the chips, 0=Sun..6=Sat, ordered by week-start.
private fun weekdayOrder(weekStart: Int): List<Int> =
    if (weekStart == 0) listOf(0, 1, 2, 3, 4, 5, 6) else listOf(1, 2, 3, 4, 5, 6, 0)

private val WeekdayShort = listOf("Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб")

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
    initialRecurrence: Recurrence?,
    columns: List<BoardColumn>,
    onApply: (iso: String?, recurrence: Recurrence?) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    val ws = AppContainer.prefs.preferences.collectAsStateWithLifecycle(initialValue = Preferences()).value.weekStart
    val initial = remember { localCal(parseInstantMillis(initialIso)) }
    var year by remember { mutableIntStateOf(initial.get(Calendar.YEAR)) }
    var month by remember { mutableIntStateOf(initial.get(Calendar.MONTH)) }
    var day by remember { mutableIntStateOf(initial.get(Calendar.DAY_OF_MONTH)) }
    var hour by remember { mutableIntStateOf(initial.get(Calendar.HOUR_OF_DAY)) }
    var minute by remember { mutableIntStateOf(initial.get(Calendar.MINUTE)) }

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

    fun stepMonth(delta: Int) {
        val m = month + delta
        year += Math.floorDiv(m, 12)
        month = Math.floorMod(m, 12)
        val maxDay = daysInMonth(year, month)
        if (day > maxDay) day = maxDay
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
            return localCalOf(yy, mm - 1, dd, hour, minute).timeInMillis
        }
        return localCalOf(year, month, day, hour, minute).timeInMillis
    }

    val occKeys = occurrenceKeys(buildRule(), dueMillis(), 24)
    val selectedKey = if (freq != "custom") millisDayKey(localCalOf(year, month, day, hour, minute).timeInMillis) else ""

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
            // ── calendar header ──
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                NavBtn(double = true, forward = false) { year-- }
                NavBtn(double = false, forward = false) { stepMonth(-1) }
                Text(
                    "${MonthsFull[month]} $year",
                    color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.Center, modifier = Modifier.weight(1f),
                )
                NavBtn(double = false, forward = true) { stepMonth(1) }
                NavBtn(double = true, forward = true) { year++ }
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

            val gridStart = monthGridStart(year, month, ws)
            for (week in 0 until 6) {
                Row(Modifier.fillMaxWidth()) {
                    for (dow in 0 until 7) {
                        val cell = (gridStart.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, week * 7 + dow) }
                        val inMonth = cell.get(Calendar.MONTH) == month
                        val cellDay = cell.get(Calendar.DAY_OF_MONTH)
                        val key = millisDayKey(cell.timeInMillis)
                        val isSel = key == selectedKey
                        DayCell(
                            day = cellDay,
                            inMonth = inMonth,
                            isSelected = isSel,
                            isOccurrence = !isSel && occKeys.contains(key),
                            modifier = Modifier.weight(1f),
                            onClick = {
                                if (freq == "custom") {
                                    if (customDates.contains(key)) customDates.remove(key) else customDates.add(key)
                                } else {
                                    year = cell.get(Calendar.YEAR)
                                    month = cell.get(Calendar.MONTH)
                                    day = cellDay
                                }
                            },
                        )
                    }
                }
            }

            Spacer(Modifier.height(12.dp))
            // ── time steppers ──
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically) {
                Stepper(value = hour, onChange = { hour = Math.floorMod(it, 24) })
                Text(":", color = c.text1, fontSize = 26.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 8.dp))
                Stepper(value = minute, onChange = { minute = Math.floorMod(it, 60) }, step = 5)
            }

            Spacer(Modifier.height(14.dp))
            // ── recurrence ──
            Row(verticalAlignment = Alignment.CenterVertically) {
                IonIcon(Ion.REPEAT, size = 14.dp, tint = c.text3)
                Spacer(Modifier.width(6.dp))
                Text("Повтор", color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
            }
            Spacer(Modifier.height(8.dp))
            // Frequency chips wrap onto two rows.
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                FreqChips.chunked(3).forEach { rowChips ->
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        rowChips.forEach { (value, label) ->
                            RecurChip(label = label, selected = freq == value, modifier = Modifier.weight(1f)) { freq = value }
                        }
                    }
                }
            }

            if (freq.isNotBlank()) {
                if (freq != "custom") {
                    Spacer(Modifier.height(10.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("каждые", color = c.text3, fontSize = 13.sp)
                        Spacer(Modifier.width(10.dp))
                        IntervalStepper(value = interval, onChange = { interval = it.coerceIn(1, 99) })
                        Spacer(Modifier.width(10.dp))
                        Text(ruPlural(interval, UnitForms.getValue(freq)), color = c.text3, fontSize = 13.sp)
                    }
                }
                if (freq == "weekly") {
                    Spacer(Modifier.height(10.dp))
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        weekdayOrder(ws).forEach { d ->
                            RecurChip(
                                label = WeekdayShort[d],
                                selected = weekdaySet.contains(d),
                                modifier = Modifier.weight(1f),
                            ) { if (weekdaySet.contains(d)) weekdaySet.remove(d) else weekdaySet.add(d) }
                        }
                    }
                }
                if (freq == "custom") {
                    Spacer(Modifier.height(8.dp))
                    Text("Отметьте даты повтора в календаре выше.", color = c.text3, fontSize = 12.sp)
                }

                Spacer(Modifier.height(12.dp))
                SelectField("Событие", TriggerOptions, trigger) { trigger = it ?: "complete" }

                if (trigger == "column") {
                    Spacer(Modifier.height(8.dp))
                    SelectField(
                        "Колонка-триггер",
                        columns.map { it.id as String? to it.name },
                        triggerColumn,
                        placeholder = "Выберите колонку",
                    ) { triggerColumn = it }
                }

                Spacer(Modifier.height(8.dp))
                SelectField(
                    "Переносить в",
                    listOf<Pair<String?, String>>(null to "Первая колонка") + columns.map { it.id as String? to it.name },
                    targetColumn,
                ) { targetColumn = it }

                Spacer(Modifier.height(10.dp))
                ToggleRow("Создавать дубликат", createNew) { createNew = it }
                ToggleRow("Повторять всегда", forever) { forever = it }
                if (freq == "daily" || freq == "weekly") {
                    ToggleRow("Пропускать выходные", skipWeekends) { skipWeekends = it }
                }
            }

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "Очистить", color = c.text3, fontSize = 14.sp,
                    modifier = Modifier.clickableNoRipple {
                        onApply(null, null)
                        onDismiss()
                    },
                )
                Spacer(Modifier.weight(1f))
                Text("Отмена", color = c.text3, fontSize = 14.sp, modifier = Modifier.clickableNoRipple { onDismiss() })
                Spacer(Modifier.width(18.dp))
                TButton("Готово", onClick = {
                    val rule = buildRule()
                    val iso = dueMillis()?.let { millisToUtcIso(it) }
                    onApply(iso, rule)
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
private fun DayCell(day: Int, inMonth: Boolean, isSelected: Boolean, isOccurrence: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(modifier.aspectRatio(1f).clickableNoRipple(onClick = onClick), contentAlignment = Alignment.Center) {
        Box(
            Modifier.size(36.dp).clip(RoundedCornerShape(RadiusMd)).then(
                when {
                    isSelected -> Modifier.background(accentGradient(c.primary))
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
                    isOccurrence -> c.primary
                    !inMonth -> c.text3
                    else -> c.text1
                },
                fontSize = 14.sp,
                fontWeight = if (isSelected || isOccurrence) FontWeight.SemiBold else FontWeight.Normal,
            )
        }
    }
}

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

private fun localCal(millis: Long?): Calendar = Calendar.getInstance().apply {
    if (millis != null) timeInMillis = millis else add(Calendar.MINUTE, 30)
    set(Calendar.SECOND, 0)
    set(Calendar.MILLISECOND, 0)
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
