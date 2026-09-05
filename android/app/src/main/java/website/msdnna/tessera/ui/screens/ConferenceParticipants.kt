package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
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
import website.msdnna.tessera.data.conference.ConfPerson
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.TesseraWarning
import website.msdnna.tessera.ui.viewmodels.ConferenceRoomUiState
import website.msdnna.tessera.util.CONF_VOLUME_MAX
import website.msdnna.tessera.util.ConfDeniedKind
import website.msdnna.tessera.util.ConfMicBadge
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.confMicBadge

/**
 * Who is in the call, and what can be done about them (#2896 §6, web
 * `ParticipantsPanel.vue`).
 *
 * The panel mixes two sources that look alike and are not. The roster comes from
 * the room socket — the server's truth about presence, roles, hands and
 * force-mutes. The volume controls come from the media engine and never leave
 * this phone. Keeping that line visible is the point of the layout: the slider
 * and the local mute sit under the name, while the two moderation actions are
 * pushed to a row of their own behind a confirmation. «Я их не слышу» and «их не
 * должен слышать никто» must not be two controls that look the same.
 *
 * A sheet rather than the web's side rail: a phone has no width to spare beside
 * a video call, and the rail's own collapse button exists on the web for exactly
 * that reason.
 */
@Composable
internal fun ConferenceParticipantsPanel(
    state: ConferenceRoomUiState,
    onClose: () -> Unit,
    onForceMute: (String, Boolean) -> Unit,
    onAskKick: (String) -> Unit,
    onCancelKick: () -> Unit,
    onConfirmKick: () -> Unit,
    onLocalMute: (String) -> Unit,
    onVolume: (String, Float) -> Unit,
    onDismissDenied: () -> Unit,
) {
    val c = Tessera.colors
    val roster = state.roster

    // Scrim and sheet are siblings rather than parent and child, and that is not
    // a layout preference. A `clickable` wrapped around the sheet — the obvious
    // way to stop a tap on a row reaching the toolbar underneath — makes it a
    // merging semantics node, and every tag inside it stops answering. Stacked
    // this way the scrim covers exactly what the sheet does not, so nothing has
    // to swallow anything.
    Column(Modifier.fillMaxSize()) {
        Box(
            Modifier.fillMaxWidth().weight(SCRIM_WEIGHT)
                .background(c.text1.copy(alpha = 0.4f))
                .clickableNoRipple(onClick = onClose),
        )
        Column(
            Modifier.fillMaxWidth()
                // A share of the height rather than a fixed one: the sheet has to
                // stay inside a short screen, and the roster scrolls anyway.
                .weight(PANEL_WEIGHT)
                .clip(RoundedCornerShape(topStart = RadiusMd, topEnd = RadiusMd))
                .background(c.surface)
                .testTag(TestTags.CONFERENCE_PANEL),
        ) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stringResource(R.string.conf_panel_title, roster.size),
                    color = c.text1,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                )
                IonIcon(
                    Ion.CLOSE,
                    size = 18.dp,
                    tint = c.text2,
                    description = stringResource(R.string.conf_panel_close),
                    modifier = Modifier.clickableNoRipple(onClick = onClose)
                        .testTag(TestTags.CONFERENCE_PANEL_CLOSE),
                )
            }

            // Our own force-mute, said out loud. The toolbar shows a microphone
            // that is merely off, which reads as our own last press.
            if (state.room.forceMuted) {
                PanelNotice(stringResource(R.string.conf_panel_you_force_muted), TestTags.CONFERENCE_BANNER)
            }

            val deniedText = when (state.denied) {
                ConfDeniedKind.KICK -> stringResource(R.string.conf_denied_kick)
                ConfDeniedKind.MUTE -> stringResource(R.string.conf_denied_mute)
                ConfDeniedKind.OTHER -> stringResource(R.string.conf_denied_other)
                ConfDeniedKind.NONE -> ""
            }
            if (deniedText.isNotEmpty()) {
                // Dismissed by tapping the notice itself, not a wrapper around
                // it: a `clickable` one node up merges the tag away, which is
                // the same trap the scrim above sidesteps.
                PanelNotice(deniedText, TestTags.CONFERENCE_DENIED, onDismissDenied)
            }

            if (roster.isEmpty()) {
                Text(
                    stringResource(R.string.conf_panel_empty),
                    color = c.text3,
                    fontSize = 13.sp,
                    modifier = Modifier.fillMaxWidth().padding(vertical = 20.dp)
                        .testTag(TestTags.CONFERENCE_PANEL_EMPTY),
                )
            } else {
                LazyColumn(Modifier.fillMaxWidth().weight(1f)) {
                    items(roster.size, key = { roster[it].userId }) { i ->
                        val person = roster[i]
                        PersonRow(
                            person = person,
                            me = person.userId == state.room.meId,
                            actions = state.rowActions(person),
                            hasLocalAudio = state.hasLocalAudio(person),
                            locallyMuted = person.userId in state.session.localMuted,
                            volume = state.session.volumes[person.userId] ?: 1f,
                            onForceMute = { onForceMute(person.userId, !person.forceMuted) },
                            onKick = { onAskKick(person.userId) },
                            onLocalMute = { onLocalMute(person.userId) },
                            onVolume = { onVolume(person.userId, it) },
                        )
                    }
                }
            }
        }
    }

    state.kickTarget?.let { target ->
        TConfirmDialog(
            title = stringResource(R.string.conf_panel_kick),
            message = stringResource(R.string.conf_panel_confirm_kick, target.name),
            confirmText = stringResource(R.string.conf_panel_kick),
            confirmTag = TestTags.CONFERENCE_KICK_CONFIRM,
            onConfirm = onConfirmKick,
            onDismiss = onCancelKick,
        )
    }
}

@Composable
private fun PanelNotice(text: String, tag: String, onClick: (() -> Unit)? = null) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().background(c.surfaceAlt)
            .then(if (onClick != null) Modifier.clickableNoRipple(onClick = onClick) else Modifier)
            .padding(horizontal = 14.dp, vertical = 8.dp)
            .testTag(tag),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.WARNING, size = 14.dp, tint = TesseraWarning)
        Spacer(Modifier.width(8.dp))
        Text(text, color = c.text2, fontSize = 12.sp)
    }
}

@Composable
private fun PersonRow(
    person: ConfPerson,
    me: Boolean,
    actions: website.msdnna.tessera.util.ConfRowActions,
    hasLocalAudio: Boolean,
    locallyMuted: Boolean,
    volume: Float,
    onForceMute: () -> Unit,
    onKick: () -> Unit,
    onLocalMute: () -> Unit,
    onVolume: (Float) -> Unit,
) {
    val c = Tessera.colors
    val badge = confMicBadge(person)
    Column(
        Modifier.fillMaxWidth()
            // Our own row carries no controls, so the tint is what explains the
            // empty space rather than it reading as something that failed.
            .background(if (me) c.surfaceAlt else c.surface)
            .padding(horizontal = 14.dp, vertical = 8.dp)
            .testTag(TestTags.conferencePerson(person.userId)),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            MemberAvatar(28.dp, person.name, userId = person.userId)
            Spacer(Modifier.width(9.dp))
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(person.name, color = c.text1, fontSize = 13.sp, maxLines = 1)
                    if (person.handAt != null) {
                        Spacer(Modifier.width(5.dp))
                        // The one warm colour here: a raised hand is the only
                        // thing in this list waiting on a person rather than
                        // describing one.
                        IonIcon(
                            Ion.HAND_RIGHT,
                            size = 13.dp,
                            tint = TesseraWarning,
                            // The marker has no text beside it, so without a
                            // label a raised hand is silent to a screen reader —
                            // which is the one thing it must not be.
                            description = stringResource(R.string.conf_panel_hand_up),
                            modifier = Modifier.testTag(TestTags.conferenceHandUp(person.userId)),
                        )
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        stringResource(if (person.host) R.string.conf_role_host else R.string.conf_role_member),
                        color = c.text3,
                        fontSize = 11.sp,
                    )
                    Text(" · ", color = c.text3, fontSize = 11.sp)
                    when (badge) {
                        ConfMicBadge.FORCE_MUTED -> Text(
                            stringResource(R.string.conf_panel_force_muted),
                            color = TesseraDanger,
                            fontSize = 11.sp,
                            modifier = Modifier.testTag(TestTags.conferenceForced(person.userId)),
                        )

                        ConfMicBadge.OFF -> Text(
                            stringResource(R.string.conf_panel_mic_off),
                            color = c.text3,
                            fontSize = 11.sp,
                        )

                        ConfMicBadge.ON -> Text(
                            stringResource(R.string.conf_panel_speaking),
                            color = c.primary,
                            fontSize = 11.sp,
                        )
                    }
                    if (me) {
                        Text(
                            " · " + stringResource(R.string.conf_panel_you),
                            color = c.text3,
                            fontSize = 11.sp,
                        )
                    }
                }
            }
            IonIcon(
                if (badge == ConfMicBadge.ON) Ion.MIC else Ion.MIC_OFF,
                size = 15.dp,
                tint = if (badge == ConfMicBadge.ON) c.primary else c.text3,
            )
        }

        // Local playback. Nothing below reaches the server or the other person.
        if (hasLocalAudio) {
            Row(
                Modifier.fillMaxWidth().padding(start = 37.dp, top = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIcon(
                    if (locallyMuted) Ion.VOLUME_MUTE else Ion.VOLUME_HIGH,
                    size = 15.dp,
                    tint = if (locallyMuted) TesseraDanger else c.text2,
                    description = stringResource(
                        if (locallyMuted) {
                            R.string.conf_panel_unmute_for_me
                        } else {
                            R.string.conf_panel_mute_for_me
                        },
                    ),
                    modifier = Modifier.clip(CircleShape)
                        .clickableNoRipple(onClick = onLocalMute)
                        .testTag(TestTags.conferenceLocalMute(person.userId)),
                )
                Spacer(Modifier.width(10.dp))
                Slider(
                    value = volume,
                    onValueChange = onVolume,
                    valueRange = 0f..CONF_VOLUME_MAX,
                    enabled = !locallyMuted,
                    colors = SliderDefaults.colors(
                        thumbColor = c.primary,
                        activeTrackColor = c.primary,
                        inactiveTrackColor = c.surfaceAlt,
                    ),
                    modifier = Modifier.weight(1f).height(24.dp)
                        .testTag(TestTags.conferenceVolume(person.userId)),
                )
            }
        }

        // Moderation. Both are visible to the whole room and neither can be
        // taken back by whoever did it.
        if (actions.forceMute || actions.kick) {
            Row(
                Modifier.fillMaxWidth().padding(top = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (actions.forceMute) {
                    TButton(
                        stringResource(
                            if (person.forceMuted) {
                                R.string.conf_panel_unforce_mute
                            } else {
                                R.string.conf_panel_force_mute
                            },
                        ),
                        kind = TButtonKind.Ghost,
                        onClick = onForceMute,
                        modifier = Modifier.testTag(TestTags.conferenceForceMute(person.userId)),
                    )
                }
                if (actions.kick) {
                    TButton(
                        stringResource(R.string.conf_panel_kick),
                        kind = TButtonKind.Danger,
                        onClick = onKick,
                        modifier = Modifier.testTag(TestTags.conferenceKick(person.userId)),
                    )
                }
            }
        }
    }
}

private const val SCRIM_WEIGHT = 3f
private const val PANEL_WEIGHT = 7f
