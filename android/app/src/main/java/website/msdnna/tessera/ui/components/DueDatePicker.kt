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
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Locale
import java.util.TimeZone
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient

private val MonthsFull = listOf(
    "январь", "февраль", "март", "апрель", "май", "июнь",
    "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь",
)
private val Weekdays = listOf("пн", "вт", "ср", "чт", "пт", "сб", "вс")

/** Weekday header reordered for the user's week-start (0 = Sunday, else Monday). */
private fun weekdayLabels(weekStart: Int): List<String> =
    if (weekStart == 0) listOf(Weekdays.last()) + Weekdays.dropLast(1) else Weekdays

/**
 * A due-date picker — our own grid (not M3), styled like the web frontend:
 * Monday-first weeks, prev/next month + year arrows, a rounded-square selected
 * day carrying the accent gradient, and a today dot. Emits an ISO-8601
 * UTC-midnight string (matching what the backend stores) or null when cleared.
 */
@Composable
fun DueDatePicker(initialIso: String?, onPick: (String?) -> Unit, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val weekStart by AppContainer.prefs.preferences.collectAsStateWithLifecycle(initialValue = Preferences())
    val ws = weekStart.weekStart
    val todayMillis = remember { utcMidnightToday() }
    val selected = remember { isoToMillis(initialIso) }
    // Open on the selected month, else the current month.
    val anchor = remember {
        utcCal().apply { timeInMillis = selected ?: todayMillis }
    }
    var year by remember { mutableStateOf(anchor.get(Calendar.YEAR)) }
    var month by remember { mutableStateOf(anchor.get(Calendar.MONTH)) } // 0-based

    fun stepMonth(delta: Int) {
        val m = month + delta
        year += Math.floorDiv(m, 12)
        month = Math.floorMod(m, 12)
    }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier
                .popupAppear(TransformOrigin.Center)
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface)
                .padding(16.dp),
        ) {
            // ── header: «  ‹  month year  ›  » ──
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                NavBtn(double = true, forward = false) { year-- }
                NavBtn(double = false, forward = false) { stepMonth(-1) }
                Text(
                    "${MonthsFull[month]} $year",
                    color = c.text1,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                    modifier = Modifier.weight(1f),
                )
                NavBtn(double = false, forward = true) { stepMonth(1) }
                NavBtn(double = true, forward = true) { year++ }
            }
            Spacer(Modifier.height(8.dp))

            // ── weekday header ──
            Row(Modifier.fillMaxWidth()) {
                weekdayLabels(ws).forEach { w ->
                    Text(
                        w,
                        color = c.text3,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                        textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                        modifier = Modifier.weight(1f),
                    )
                }
            }
            Spacer(Modifier.height(4.dp))

            // ── day grid (6 weeks) ──
            val gridStart = monthGridStart(year, month, ws)
            for (week in 0 until 6) {
                Row(Modifier.fillMaxWidth()) {
                    for (dow in 0 until 7) {
                        val cell = (gridStart.clone() as Calendar).apply {
                            add(Calendar.DAY_OF_MONTH, week * 7 + dow)
                        }
                        val cellMillis = cell.timeInMillis
                        DayCell(
                            day = cell.get(Calendar.DAY_OF_MONTH),
                            inMonth = cell.get(Calendar.MONTH) == month,
                            isSelected = selected != null && selected == cellMillis,
                            isToday = cellMillis == todayMillis,
                            modifier = Modifier.weight(1f),
                            onClick = {
                                onPick(millisToIso(cellMillis))
                                onDismiss()
                            },
                        )
                    }
                }
            }

            Spacer(Modifier.height(10.dp))
            // ── footer: Очистить … Сегодня ──
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "Очистить",
                    color = c.text3,
                    fontSize = 14.sp,
                    modifier = Modifier.clickableNoRipple {
                        onPick(null)
                        onDismiss()
                    },
                )
                Spacer(Modifier.weight(1f))
                Text(
                    "Сегодня",
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    style = TextStyle(brush = accentGradient(c.primary)),
                    modifier = Modifier.clickableNoRipple {
                        onPick(millisToIso(todayMillis))
                        onDismiss()
                    },
                )
            }
        }
    }
}

@Composable
private fun DayCell(
    day: Int,
    inMonth: Boolean,
    isSelected: Boolean,
    isToday: Boolean,
    modifier: Modifier,
    onClick: () -> Unit,
) {
    val c = Tessera.colors
    Box(modifier.aspectRatio(1f).clickableNoRipple(onClick = onClick), contentAlignment = Alignment.Center) {
        Box(
            Modifier
                .size(36.dp)
                .clip(RoundedCornerShape(RadiusMd))
                .then(if (isSelected) Modifier.background(accentGradient(c.primary)) else Modifier),
        ) {
            Text(
                day.toString(),
                color = when {
                    isSelected -> c.onPrimary
                    !inMonth -> c.text3
                    else -> c.text1
                },
                fontSize = 14.sp,
                fontWeight = if (isSelected || isToday) FontWeight.SemiBold else FontWeight.Normal,
                modifier = Modifier.align(Alignment.Center),
            )
            if (isToday) {
                Box(
                    Modifier
                        .align(Alignment.TopEnd)
                        .padding(top = 5.dp, end = 6.dp)
                        .size(4.dp)
                        .clip(CircleShape)
                        .background(if (isSelected) c.onPrimary else c.primary),
                )
            }
        }
    }
}

/** A 34dp icon-only nav button; the "back" glyph is the forward chevron mirrored,
 *  and a "double" arrow (year jump) draws two chevrons. */
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
                    website.msdnna.tessera.util.Ion.CHEVRON_FORWARD,
                    size = 16.dp,
                    tint = c.text2,
                    modifier = if (forward) Modifier else Modifier.graphicsLayer { scaleX = -1f },
                )
            }
        }
    }
}

// ── date helpers (Calendar / UTC, no java.time on minSdk 24) ──

private fun utcCal(): Calendar = Calendar.getInstance(TimeZone.getTimeZone("UTC"), Locale.US)

private fun utcMidnightToday(): Long = utcCal().apply {
    set(Calendar.HOUR_OF_DAY, 0)
    set(Calendar.MINUTE, 0)
    set(Calendar.SECOND, 0)
    set(Calendar.MILLISECOND, 0)
}.timeInMillis

/** First grid cell (Monday on/before the 1st) for the given month, at UTC midnight. */
private fun monthGridStart(year: Int, month: Int, weekStart: Int): Calendar {
    val first = utcCal().apply {
        clear()
        set(year, month, 1)
    }
    // Calendar.DAY_OF_WEEK: Sunday=1 … Saturday=7. weekStart 0 = Sunday-first,
    // else Monday-first; offset the leading blanks accordingly.
    val firstDow = if (weekStart == 0) Calendar.SUNDAY else Calendar.MONDAY
    val leading = (first.get(Calendar.DAY_OF_WEEK) - firstDow + 7) % 7
    return (first.clone() as Calendar).apply { add(Calendar.DAY_OF_MONTH, -leading) }
}

private fun isoFmt(): SimpleDateFormat =
    SimpleDateFormat("yyyy-MM-dd'T'00:00:00'Z'", Locale.US).apply { timeZone = TimeZone.getTimeZone("UTC") }

private fun isoToMillis(iso: String?): Long? {
    if (iso.isNullOrBlank() || iso.length < 10) return null
    return runCatching {
        SimpleDateFormat("yyyy-MM-dd", Locale.US).apply { timeZone = TimeZone.getTimeZone("UTC") }
            .parse(iso.substring(0, 10))?.time
    }.getOrNull()
}

private fun millisToIso(millis: Long?): String? {
    if (millis == null) return null
    return isoFmt().format(java.util.Date(millis))
}
