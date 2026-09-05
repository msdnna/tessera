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
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceParticipant
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.ConferenceLobbyUiState
import website.msdnna.tessera.ui.viewmodels.ConferenceLobbyViewModel
import website.msdnna.tessera.util.ConfTimeKind
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.conferenceTimeLine
import website.msdnna.tessera.util.label
import website.msdnna.tessera.util.localDateTimeLabel

/**
 * The lobby of one conference (#2896 §3, web `ConferencesView`'s detail half):
 * the call's plan, who is in the room, who is still being called, and the
 * controls to join, leave, end and invite.
 *
 * An inline overlay over the list, the same master/detail shape [DocumentsScreen]
 * uses — the section is one destination in the shell's nav, so a detail on the
 * back stack would need a second one.
 */
@Composable
fun ConferenceLobby(conferenceId: String, onBack: () -> Unit) {
    val c = Tessera.colors
    val vm: ConferenceLobbyViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(conferenceId) { vm.load(conferenceId) }

    Column(Modifier.fillMaxSize().background(c.surface).testTag(TestTags.CONFERENCE_LOBBY)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = onBack,
                boxSize = 40.dp,
                modifier = Modifier.graphicsLayer { scaleX = -1f }.testTag(TestTags.CONFERENCE_BACK),
            )
            Spacer(Modifier.width(4.dp))
            Text(
                state.conference?.title?.ifBlank { stringResource(R.string.conf_untitled) }
                    ?: stringResource(R.string.conf_back),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
        }
        HorizontalDivider(color = c.border)

        when {
            state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                TesseraLoader()
            }

            state.notFound || state.conference == null -> Box(
                Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    IonIcon(Ion.VIDEOCAM, size = 40.dp, tint = c.text3)
                    Spacer(Modifier.height(10.dp))
                    Text(stringResource(R.string.conf_detail_not_found), color = c.text3, fontSize = 14.sp)
                }
            }

            else -> ConferenceLobbyBody(
                state = state,
                onJoin = { vm.join() },
                onLeave = { vm.leave() },
                onAskEnd = { vm.askEnd() },
                onConfirmEnd = { vm.confirmEnd() },
                onCancelEnd = { vm.cancelEnd() },
                onInvite = { vm.invite(it) },
            )
        }
    }

    // The call itself, over the lobby, exactly while we hold a seat (§5). Media
    // follows membership and never leads it: hanging up drops the seat, and the
    // roster coming back without us is what closes this — so a room the server
    // no longer counts us in cannot stay on screen still publishing.
    if (state.inRoom) {
        ConferenceRoom(conferenceId = conferenceId, onHangup = { vm.leave() })
    }
}

/**
 * The lobby's content, driven purely by [state]. Stateless (and `internal`) so a
 * spec can mount it with a hand-built roster — the screen above it needs a
 * repository and a socket before it renders anything at all.
 */
@Composable
internal fun ConferenceLobbyBody(
    state: ConferenceLobbyUiState,
    onJoin: () -> Unit,
    onLeave: () -> Unit,
    onAskEnd: () -> Unit,
    onConfirmEnd: () -> Unit,
    onCancelEnd: () -> Unit,
    onInvite: (String) -> Unit,
) {
    val c = Tessera.colors
    val conference = state.conference ?: return

    LazyColumn(
        Modifier.fillMaxSize().padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            Spacer(Modifier.height(6.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                ConferenceStatusPill(conference.status)
                Spacer(Modifier.width(10.dp))
                Text(lobbyTimeLine(conference), color = c.text3, fontSize = 12.sp)
            }
        }

        if (conference.description.isNotBlank()) {
            item { Text(conference.description, color = c.text2, fontSize = 14.sp) }
        }

        item {
            LobbyActions(
                state = state,
                onJoin = onJoin,
                onLeave = onLeave,
                onAskEnd = onAskEnd,
                onConfirmEnd = onConfirmEnd,
                onCancelEnd = onCancelEnd,
                onInvite = onInvite,
            )
        }

        item { SectionLabel(stringResource(R.string.conf_section_in_room)) }
        if (state.roster.isEmpty()) {
            item { Text(stringResource(R.string.conf_room_empty), color = c.text3, fontSize = 13.sp) }
        } else {
            items(state.roster.size, key = { state.roster[it].userId }) { i ->
                SeatRow(state.roster[i], showHost = true)
            }
        }

        // Only rendered when somebody is actually being called: an «Приглашены»
        // header over nothing reads as a list that failed to load.
        if (state.invited.isNotEmpty()) {
            item { SectionLabel(stringResource(R.string.conf_section_invited)) }
            items(state.invited.size, key = { state.invited[it].userId }) { i ->
                SeatRow(state.invited[i], showHost = false)
            }
        }

        state.error?.let { message ->
            item { Text(message.resolve(), color = c.text3, fontSize = 12.sp) }
        }

        item { Spacer(Modifier.height(24.dp)) }
    }
}

/**
 * Join/leave, end and invite.
 *
 * Join is offered whatever the status: a conference is a reusable room (#2879),
 * so an ended one is entered rather than being a dead end. «Завершить» is the
 * opposite — it only means anything while somebody is in the call, and on a
 * paused room it would ask the server to end what already ended.
 */
@Composable
private fun LobbyActions(
    state: ConferenceLobbyUiState,
    onJoin: () -> Unit,
    onLeave: () -> Unit,
    onAskEnd: () -> Unit,
    onConfirmEnd: () -> Unit,
    onCancelEnd: () -> Unit,
    onInvite: (String) -> Unit,
) {
    val conference = state.conference ?: return
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        if (state.inRoom) {
            TButton(
                stringResource(R.string.conf_action_leave),
                kind = TButtonKind.Secondary,
                loading = state.busy,
                onClick = onLeave,
                modifier = Modifier.testTag(TestTags.CONFERENCE_LEAVE),
            )
        } else {
            TButton(
                stringResource(R.string.conf_action_join),
                loading = state.busy,
                onClick = onJoin,
                modifier = Modifier.testTag(TestTags.CONFERENCE_JOIN),
            )
        }

        if (state.canModerate && conference.isLive) {
            Spacer(Modifier.width(8.dp))
            Box {
                TButton(
                    stringResource(R.string.conf_action_end),
                    kind = TButtonKind.Ghost,
                    onClick = onAskEnd,
                    modifier = Modifier.testTag(TestTags.CONFERENCE_END),
                )
                TConfirmPopover(
                    expanded = state.confirmingEnd,
                    message = stringResource(R.string.conf_confirm_end),
                    confirmText = stringResource(R.string.conf_action_end),
                    onConfirm = onConfirmEnd,
                    onDismiss = onCancelEnd,
                )
            }
        }

        if (state.canInvite) {
            Spacer(Modifier.weight(1f))
            InviteButton(invitable = state.invitable, onInvite = onInvite)
        }
    }
}

/**
 * The invite picker. One tap invites that person immediately — there is no batch
 * «Отправить», matching the web popover: an invitation is a ring, and a list you
 * confirm at the end is a list you forget to confirm.
 */
@Composable
private fun InviteButton(invitable: List<Member>, onInvite: (String) -> Unit) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    Box {
        TButton(
            stringResource(R.string.conf_invite),
            kind = TButtonKind.Ghost,
            onClick = { open = true },
            modifier = Modifier.testTag(TestTags.CONFERENCE_INVITE),
        )
        TDropdown(expanded = open, onDismiss = { open = false }, scrollable = true) {
            if (invitable.isEmpty()) {
                Text(
                    stringResource(R.string.conf_invite_empty),
                    color = c.text3,
                    fontSize = 13.sp,
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
                )
            }
            invitable.forEach { m ->
                Row(
                    Modifier.fillMaxWidth()
                        .clickableNoRipple {
                            open = false
                            onInvite(m.userId)
                        }
                        .padding(horizontal = 12.dp, vertical = 8.dp)
                        .testTag(TestTags.conferenceInvitee(m.userId)),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    MemberAvatar(22.dp, m.name, userId = m.userId)
                    Spacer(Modifier.width(8.dp))
                    Text(m.name.ifBlank { m.email }, color = c.text1, fontSize = 14.sp)
                }
            }
        }
    }
}

/** One person in the roster. [showHost] marks who may moderate the call. */
@Composable
private fun SeatRow(participant: ConferenceParticipant, showHost: Boolean) {
    val c = Tessera.colors
    val name = participant.label()
    Row(
        Modifier.fillMaxWidth().padding(vertical = 2.dp).testTag(TestTags.conferenceSeat(participant.userId)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        MemberAvatar(26.dp, name, userId = participant.userId)
        Spacer(Modifier.width(8.dp))
        Text(name, color = c.text1, fontSize = 14.sp, maxLines = 1, modifier = Modifier.weight(1f))
        if (showHost && participant.isHost) {
            Text(stringResource(R.string.conf_seat_host), color = c.text3, fontSize = 11.sp)
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    val c = Tessera.colors
    Text(
        text,
        color = c.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.Medium,
        modifier = Modifier.padding(top = 6.dp),
    )
}

/** Same one-line rule as the list row — see `conferenceTimeLine`. */
@Composable
private fun lobbyTimeLine(conference: Conference): String {
    val line = conferenceTimeLine(conference)
    val at = line.iso?.let { localDateTimeLabel(LocalResources.current, it, LocalDateFormat.current) }.orEmpty()
    if (line.kind == ConfTimeKind.NONE || at.isBlank()) return stringResource(R.string.conf_row_no_time)
    return when (line.kind) {
        ConfTimeKind.ENDED -> stringResource(R.string.conf_row_ended_at, at)
        ConfTimeKind.STARTED -> stringResource(R.string.conf_row_started_at, at)
        else -> stringResource(R.string.conf_row_scheduled_at, at)
    }
}
