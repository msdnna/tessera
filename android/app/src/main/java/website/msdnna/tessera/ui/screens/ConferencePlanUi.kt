package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.WorkspaceMilestone
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.CONF_DURATIONS
import website.msdnna.tessera.util.CONF_RECURRENCES
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.conferenceDurationParts

// A meeting's plan on Android (#3034, web `utils/confPlan.js` + the schedule
// form's last line): its length, repeat and stage — read on a row and in the
// lobby, picked in the schedule dialog.

/** «45 мин» / «1 ч» / «1 ч 30 мин», or null when the meeting has no length. */
@Composable
internal fun conferenceDurationLabel(minutes: Int?): String? {
    val (h, m) = conferenceDurationParts(minutes) ?: return null
    return when {
        h == 0 -> stringResource(R.string.conf_plan_min, m)
        m == 0 -> stringResource(R.string.conf_plan_h, h)
        else -> stringResource(R.string.conf_plan_hm, h, m)
    }
}

@Composable
internal fun conferenceRecurrenceLabel(rule: String): String = stringResource(
    when (rule) {
        "daily" -> R.string.conf_repeat_daily
        "weekdays" -> R.string.conf_repeat_weekdays
        "weekly" -> R.string.conf_repeat_weekly
        "biweekly" -> R.string.conf_repeat_biweekly
        "monthly" -> R.string.conf_repeat_monthly
        else -> R.string.conf_repeat_none
    },
)

/**
 * The plan facts of a meeting on one line — length, repeat, stage. Draws
 * nothing for a meeting that has none of them.
 */
@Composable
internal fun ConferencePlanLine(conference: Conference, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    val length = if (conference.isEnded) null else conferenceDurationLabel(conference.durationMinutes)
    val repeat = conference.recurrence.takeIf { it.isNotBlank() }
    val stage = conference.milestoneTitle?.takeIf { it.isNotBlank() }
    if (length == null && repeat == null && stage == null) return
    Row(
        modifier.horizontalScroll(rememberScrollState()).testTag(TestTags.conferencePlan(conference.id)),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (length != null) {
            PlanFact(Ion.TIME, length)
        }
        if (repeat != null) {
            PlanFact(Ion.REPEAT, conferenceRecurrenceLabel(repeat))
        }
        if (stage != null) {
            StagePill(stage)
        }
    }
}

@Composable
private fun PlanFact(icon: String, text: String) {
    val c = Tessera.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        IonIcon(icon, size = 12.dp, tint = c.text3)
        Spacer(Modifier.width(4.dp))
        Text(text, color = c.text3, fontSize = 12.sp, maxLines = 1)
    }
}

/** The stage pill (#3053): the web's pill chip with the trail sign. */
@Composable
internal fun StagePill(title: String, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Row(
        modifier.clip(RoundedCornerShape(999.dp)).background(c.border.copy(alpha = 0.5f))
            .padding(horizontal = 8.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.TRAIL_SIGN, size = 12.dp, tint = c.text2)
        Spacer(Modifier.width(4.dp))
        Text(title, color = c.text2, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/**
 * The schedule dialog's plan pickers: length chips, then «К этапу» and the
 * repeat as two menus — the web form's last line. Stateless; the dialog owns
 * the values.
 */
@Composable
internal fun ConferencePlanPickers(
    duration: Int?,
    onDuration: (Int?) -> Unit,
    recurrence: String,
    onRecurrence: (String) -> Unit,
    repeatEnabled: Boolean,
    milestoneId: String?,
    onMilestone: (String?) -> Unit,
    milestones: List<WorkspaceMilestone>,
) {
    val c = Tessera.colors
    Column(Modifier.fillMaxWidth()) {
        Text(
            stringResource(R.string.conf_plan_duration),
            color = c.text2,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
        )
        Spacer(Modifier.height(6.dp))
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            CONF_DURATIONS.forEach { n ->
                val on = duration == n
                Box(
                    Modifier.clip(RoundedCornerShape(RadiusMd))
                        .border(1.dp, if (on) c.primary else c.border, RoundedCornerShape(RadiusMd))
                        .background(if (on) c.primary.copy(alpha = 0.12f) else c.surface)
                        .clickableNoRipple { onDuration(if (on) null else n) }
                        .padding(horizontal = 10.dp, vertical = 6.dp)
                        .testTag(TestTags.conferenceDuration(n)),
                ) {
                    Text(
                        conferenceDurationLabel(n).orEmpty(),
                        color = if (on) c.primary else c.text2,
                        fontSize = 13.sp,
                    )
                }
            }
        }
        Spacer(Modifier.height(12.dp))
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            // «К этапу [этап]»
            var stageOpen by remember { mutableStateOf(false) }
            Box(Modifier.weight(1f)) {
                Row(
                    Modifier.clickableNoRipple { stageOpen = true }.padding(vertical = 6.dp)
                        .testTag(TestTags.CONFERENCE_PLAN_STAGE),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    IonIcon(Ion.LINK, size = 14.dp, tint = c.text3)
                    Spacer(Modifier.width(6.dp))
                    Text(stringResource(R.string.conf_plan_stage), color = c.text2, fontSize = 13.sp)
                    Spacer(Modifier.width(6.dp))
                    val current = milestones.firstOrNull { it.id == milestoneId }?.title
                    if (current != null) {
                        StagePill(current)
                    } else {
                        Text(stringResource(R.string.conf_plan_no_stage), color = c.text3, fontSize = 13.sp)
                    }
                }
                TDropdown(expanded = stageOpen, onDismiss = { stageOpen = false }, scrollable = true) {
                    TMenuItem(
                        stringResource(R.string.conf_plan_no_stage),
                        onClick = {
                            onMilestone(null)
                            stageOpen = false
                        },
                    )
                    val open = milestones.filter { !it.isClosed || it.id == milestoneId }
                    if (open.isEmpty()) {
                        Text(
                            stringResource(R.string.conf_plan_no_stages),
                            color = c.text3,
                            fontSize = 12.sp,
                            modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
                        )
                    }
                    open.forEach { m ->
                        TMenuItem(
                            m.title,
                            icon = Ion.TRAIL_SIGN,
                            onClick = {
                                onMilestone(m.id)
                                stageOpen = false
                            },
                            trailing = if (open.map { it.projectName }.distinct().size > 1) {
                                { Text(m.projectName, color = c.text3, fontSize = 12.sp) }
                            } else {
                                null
                            },
                            modifier = Modifier.testTag(TestTags.conferenceStageOption(m.id)),
                        )
                    }
                }
            }
            // The repeat.
            var repeatOpen by remember { mutableStateOf(false) }
            Box {
                Row(
                    Modifier.clickableNoRipple(enabled = repeatEnabled) { repeatOpen = true }
                        .padding(vertical = 6.dp).testTag(TestTags.CONFERENCE_PLAN_REPEAT),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    IonIcon(Ion.REPEAT, size = 14.dp, tint = c.text3)
                    Spacer(Modifier.width(6.dp))
                    Text(
                        conferenceRecurrenceLabel(if (repeatEnabled) recurrence else ""),
                        color = if (repeatEnabled) c.text2 else c.text3,
                        fontSize = 13.sp,
                    )
                }
                TDropdown(expanded = repeatOpen, onDismiss = { repeatOpen = false }) {
                    CONF_RECURRENCES.forEach { rule ->
                        TMenuItem(
                            conferenceRecurrenceLabel(rule),
                            onClick = {
                                onRecurrence(rule)
                                repeatOpen = false
                            },
                            trailing = if (rule == recurrence) {
                                { IonIcon(Ion.CHECK, size = 14.dp, tint = c.primary) }
                            } else {
                                null
                            },
                            modifier = Modifier.testTag(TestTags.conferenceRepeatOption(rule)),
                        )
                    }
                }
            }
        }
        if (!repeatEnabled) {
            Text(stringResource(R.string.conf_plan_repeat_needs_time), color = c.text3, fontSize = 12.sp)
        }
    }
}
