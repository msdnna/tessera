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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
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
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.millisToUtcIso
import website.msdnna.tessera.util.parseInstantMillis

private val MonthsFull = listOf(
    "январь", "февраль", "март", "апрель", "май", "июнь",
    "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь",
)
private val Weekdays = listOf("пн", "вт", "ср", "чт", "пт", "сб", "вс")

private fun weekdayLabels(weekStart: Int): List<String> =
    if (weekStart == 0) listOf(Weekdays.last()) + Weekdays.dropLast(1) else Weekdays

/**
 * Date + time picker for reminders. Unlike [DueDatePicker] (UTC-midnight, date
 * only), a reminder is a real instant: the user picks a *local* day and time,
 * and we emit a UTC ISO-8601 string for that wall-clock moment. Same visual
 * language as the due-date grid, plus hour/minute steppers.
 */
@Composable
fun ReminderDateTimePicker(initialIso: String?, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val ws = AppContainer.prefs.preferences.collectAsStateWithLifecycle(initialValue = Preferences()).value.weekStart
    val initial = remember { localCal(parseInstantMillis(initialIso)) }
    var year by remember { mutableIntStateOf(initial.get(Calendar.YEAR)) }
    var month by remember { mutableIntStateOf(initial.get(Calendar.MONTH)) }
    var day by remember { mutableIntStateOf(initial.get(Calendar.DAY_OF_MONTH)) }
    var hour by remember { mutableIntStateOf(initial.get(Calendar.HOUR_OF_DAY)) }
    var minute by remember { mutableIntStateOf(initial.get(Calendar.MINUTE)) }

    fun stepMonth(delta: Int) {
        val m = month + delta
        year += Math.floorDiv(m, 12)
        month = Math.floorMod(m, 12)
        val maxDay = daysInMonth(year, month)
        if (day > maxDay) day = maxDay
    }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(16.dp),
        ) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                NavBtn(double = true, forward = false) { year-- }
                NavBtn(double = false, forward = false) { stepMonth(-1) }
                Text(
                    "${MonthsFull[month]} $year",
                    color = c.text1,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.weight(1f),
                )
                NavBtn(double = false, forward = true) { stepMonth(1) }
                NavBtn(double = true, forward = true) { year++ }
            }
            Spacer(Modifier.height(8.dp))

            Row(Modifier.fillMaxWidth()) {
                weekdayLabels(ws).forEach { w ->
                    Text(
                        w,
                        color = c.text3,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.weight(1f),
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
                        DayCell(
                            day = cellDay,
                            inMonth = inMonth,
                            isSelected = inMonth && cellDay == day,
                            modifier = Modifier.weight(1f),
                            onClick = {
                                year = cell.get(Calendar.YEAR)
                                month = cell.get(Calendar.MONTH)
                                day = cellDay
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

            Spacer(Modifier.height(12.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
                Text("Отмена", color = c.text3, fontSize = 14.sp, modifier = Modifier.clickableNoRipple { onDismiss() })
                Spacer(Modifier.width(18.dp))
                TButton("Готово", onClick = {
                    val millis = localCalOf(year, month, day, hour, minute).timeInMillis
                    onPick(millisToUtcIso(millis))
                    onDismiss()
                })
            }
        }
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
        IonIcon(
            Ion.CHEVRON_FORWARD,
            size = 16.dp,
            tint = c.text2,
            modifier = Modifier.graphicsLayer { rotationZ = if (up) -90f else 90f },
        )
    }
}

@Composable
private fun DayCell(day: Int, inMonth: Boolean, isSelected: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(modifier.aspectRatio(1f).clickableNoRipple(onClick = onClick), contentAlignment = Alignment.Center) {
        Box(
            Modifier.size(36.dp).clip(RoundedCornerShape(RadiusMd))
                .then(if (isSelected) Modifier.background(accentGradient(c.primary)) else Modifier),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                day.toString(),
                color = when {
                    isSelected -> c.onPrimary
                    !inMonth -> c.text3
                    else -> c.text1
                },
                fontSize = 14.sp,
                fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal,
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
                    Ion.CHEVRON_FORWARD,
                    size = 16.dp,
                    tint = c.text2,
                    modifier = if (forward) Modifier else Modifier.graphicsLayer { scaleX = -1f },
                )
            }
        }
    }
}

// ── local-zone Calendar helpers (real instants, no java.time on minSdk 24) ──

private fun localCal(millis: Long?): Calendar = Calendar.getInstance().apply {
    if (millis != null) {
        timeInMillis = millis
    } else {
        // Default: next round half-hour from now.
        add(Calendar.MINUTE, 30)
    }
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

/** First grid cell (week-start on/before the 1st) for the given month, local zone. */
private fun monthGridStart(year: Int, month: Int, weekStart: Int): Calendar {
    val first = Calendar.getInstance().apply {
        clear()
        set(year, month, 1)
    }
    val firstDow = if (weekStart == 0) Calendar.SUNDAY else Calendar.MONDAY
    val leading = (first.get(Calendar.DAY_OF_WEEK) - firstDow + 7) % 7
    return (first.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, -leading) }
}
