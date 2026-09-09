package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.TesseraLive
import website.msdnna.tessera.util.ConfMiniLine
import website.msdnna.tessera.util.Ion

/**
 * The call, minimised (#2896 §9, web `ConferenceMiniWindow`).
 *
 * Up to §8 the meeting *was* the room screen: navigating away hung up, because a
 * call had nowhere else to live. This bar is that somewhere — it rides over every
 * other screen while a conference is running off-stage, says which meeting it is
 * and carries the two controls that must never be more than one tap away: the
 * microphone and the way out.
 *
 * Stateless, and `internal` for the same reason the room's body is: a spec can
 * mount a call that never existed, where the screen above it needs an SFU.
 */
@Composable
internal fun ConferenceMiniBar(
    line: ConfMiniLine,
    title: String,
    others: Int,
    micOn: Boolean,
    micEnabled: Boolean,
    onReturn: () -> Unit,
    onToggleMic: () -> Unit,
    onHangup: () -> Unit,
) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth()
            .padding(horizontal = 8.dp, vertical = 6.dp)
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            // The whole bar is the way back, not a chevron on the end of it: this
            // is a one-handed tap on a phone that is doing something else.
            //
            // Labelled, because the bar's own text names the meeting and its state
            // — neither of which says that touching it goes back. Without the
            // label the one gesture that reopens a call announces itself as
            // «Двойное нажатие — активировать».
            .clickableNoRipple(
                onClickLabel = stringResource(R.string.conf_mini_return),
                onClick = onReturn,
            )
            .padding(horizontal = 10.dp, vertical = 8.dp)
            .testTag(TestTags.CONFERENCE_MINI),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // The dot is the call's pulse: red only while it is actually carrying
        // sound. Reconnecting and lost both drop it, so a bar in trouble does not
        // look like a bar that is fine.
        Box(
            Modifier.size(8.dp)
                .clip(CircleShape)
                .background(if (line == ConfMiniLine.LIVE) TesseraLive else c.text3),
        )
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
            Text(
                title.ifBlank { stringResource(R.string.conf_untitled) },
                color = c.text1,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
            )
            Text(
                stringResource(line.label),
                color = if (line == ConfMiniLine.LOST) TesseraDanger else c.text3,
                fontSize = 11.sp,
                maxLines = 1,
                modifier = Modifier.testTag(TestTags.CONFERENCE_MINI_LINE),
            )
        }
        if (others > 0) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.testTag(TestTags.CONFERENCE_MINI_COUNT),
            ) {
                IonIcon(
                    Ion.PEOPLE,
                    size = 14.dp,
                    tint = c.text3,
                    description = stringResource(R.string.conf_mini_others, others),
                )
                Spacer(Modifier.width(3.dp))
                Text("$others", color = c.text3, fontSize = 12.sp)
            }
            Spacer(Modifier.width(8.dp))
        }
        MiniControl(
            icon = if (micOn) Ion.MIC else Ion.MIC_OFF,
            active = micOn,
            enabled = micEnabled,
            tag = TestTags.CONFERENCE_MINI_MIC,
            description = stringResource(if (micOn) R.string.conf_mini_mic_on else R.string.conf_mini_mic_off),
            onClick = onToggleMic,
        )
        Spacer(Modifier.width(6.dp))
        MiniControl(
            icon = Ion.CALL,
            active = false,
            enabled = true,
            danger = true,
            tag = TestTags.CONFERENCE_MINI_HANGUP,
            description = stringResource(R.string.conf_mini_hangup),
            onClick = onHangup,
        )
    }
}

/** What the bar says, in the profile's language. */
private val ConfMiniLine.label: Int
    get() = when (this) {
        // The room's own «Подключаемся…», not a second copy of it: this is the
        // same wait, and two strings for one state drift the moment either moves.
        ConfMiniLine.CONNECTING -> R.string.conf_media_connecting

        ConfMiniLine.RECONNECTING -> R.string.conf_mini_reconnecting

        ConfMiniLine.LIVE -> R.string.conf_mini_live

        ConfMiniLine.LOST -> R.string.conf_mini_lost
    }

/**
 * A control on the bar: the room's round button at a third of the area, because
 * this one shares a 44dp strip with a title rather than owning a toolbar. Its own
 * composable rather than a parameter on the room's — the two differ in every
 * dimension they have, and one control that is sometimes 48dp and sometimes 32dp
 * is a worse thing to read than two that are each one size.
 */
@Composable
private fun MiniControl(
    icon: String,
    active: Boolean,
    enabled: Boolean,
    tag: String,
    description: String,
    onClick: () -> Unit,
    danger: Boolean = false,
) {
    val c = Tessera.colors
    val bg = when {
        danger -> TesseraDanger
        active -> c.primary
        else -> c.surface
    }
    val fg = if (danger || active) c.onPrimary else c.text2
    Box(
        Modifier.size(32.dp)
            .clip(CircleShape)
            .background(if (enabled) bg else bg.copy(alpha = 0.4f))
            .clickableNoRipple(enabled = enabled, onClick = onClick)
            .testTag(tag),
        contentAlignment = Alignment.Center,
    ) {
        IonIcon(
            icon,
            size = 16.dp,
            tint = if (enabled) fg else fg.copy(alpha = 0.5f),
            description = description,
        )
    }
}
