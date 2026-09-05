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
import androidx.compose.ui.platform.LocalContext
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
import website.msdnna.tessera.data.model.ConferenceRecording
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraWarning
import website.msdnna.tessera.ui.viewmodels.ConferenceLobbyUiState
import website.msdnna.tessera.ui.viewmodels.ConferenceLobbyViewModel
import website.msdnna.tessera.util.ConfTimeKind
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.confRecordingDownloadable
import website.msdnna.tessera.util.confRecordingLength
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
    val ctx = LocalContext.current
    // Resolved in composition so the system chooser speaks the profile's
    // language rather than the phone's, as the chat's downloads do.
    val chooserTitle = stringResource(R.string.conf_rec_download)

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
                onDownloadRecording = { recording ->
                    vm.downloadRecording(ctx.cacheDir, recording) { file ->
                        openDownloadedFile(ctx, file, MP4_MIME, chooserTitle)
                    }
                },
                onAskDeleteRecording = { vm.askDeleteRecording(it) },
                onCancelDeleteRecording = { vm.cancelDeleteRecording() },
                onConfirmDeleteRecording = { vm.confirmDeleteRecording() },
            )
        }
    }

    // The call itself, over the lobby, exactly while we hold a seat (§5). Media
    // follows membership and never leads it: hanging up drops the seat, and the
    // roster coming back without us is what closes this — so a room the server
    // no longer counts us in cannot stay on screen still publishing.
    if (state.inRoom) {
        ConferenceRoom(
            conferenceId = conferenceId,
            // Carried down rather than fetched again: this is where the call's
            // name is already known, and the minimised bar (§9) names the meeting
            // from screens that have never heard of it.
            title = state.conference?.title.orEmpty(),
            onHangup = { vm.leave() },
        )
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
    onDownloadRecording: (ConferenceRecording) -> Unit = {},
    onAskDeleteRecording: (String) -> Unit = {},
    onCancelDeleteRecording: () -> Unit = {},
    onConfirmDeleteRecording: () -> Unit = {},
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

        // Only when there is something to show. A team that never records must
        // not carry a permanent «Записей пока нет» under every conference.
        if (state.recordingRows.isNotEmpty()) {
            item {
                Box(Modifier.testTag(TestTags.CONFERENCE_RECORDINGS)) {
                    SectionLabel(stringResource(R.string.conf_rec_title))
                }
            }
            items(state.recordingRows.size, key = { state.recordingRows[it].id }) { i ->
                val recording = state.recordingRows[i]
                RecordingRow(
                    recording = recording,
                    canModerate = state.canModerate,
                    busy = state.recordingBusyId == recording.id,
                    confirming = state.confirmingDeleteId == recording.id,
                    onDownload = { onDownloadRecording(recording) },
                    onAskDelete = { onAskDeleteRecording(recording.id) },
                    onCancelDelete = onCancelDeleteRecording,
                    onConfirmDelete = onConfirmDeleteRecording,
                )
            }
            state.recordingsError?.let { message ->
                item {
                    Text(
                        message.resolve(),
                        color = c.text3,
                        fontSize = 12.sp,
                        modifier = Modifier.testTag(TestTags.CONFERENCE_RECORDINGS_ERROR),
                    )
                }
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

/**
 * One recording (#2896 §8).
 *
 * A failed row is shown rather than filtered out — «запись не удалась» is what
 * people came to this list to find out, and a row that silently disappeared
 * would have them asking the server admin instead. It carries no download for
 * the same reason it is shown: there are no bytes behind it.
 *
 * Delete sits behind a confirmation and only for a moderator, mirroring exactly
 * who may start one. Unlike a deleted message, which at least existed in
 * somebody's scroll, the file is gone from the disk.
 */
@Composable
private fun RecordingRow(
    recording: ConferenceRecording,
    canModerate: Boolean,
    busy: Boolean,
    confirming: Boolean,
    onDownload: () -> Unit,
    onAskDelete: () -> Unit,
    onCancelDelete: () -> Unit,
    onConfirmDelete: () -> Unit,
) {
    val c = Tessera.colors
    val length = confRecordingLength(recording)
    Row(
        Modifier.fillMaxWidth().padding(vertical = 3.dp)
            .testTag(TestTags.conferenceRecordingRow(recording.id)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(
            if (recording.isFailed) Ion.WARNING else Ion.VIDEOCAM,
            size = 15.dp,
            tint = if (recording.isFailed) TesseraWarning else c.text3,
        )
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(
                localDateTimeLabel(LocalResources.current, recording.startedAt, LocalDateFormat.current),
                color = c.text1,
                fontSize = 13.sp,
                maxLines = 1,
            )
            val detail = when {
                recording.isRunning -> stringResource(R.string.conf_rec_running)

                // The server's own sentence when it sent one: it knows why the
                // egress gave up better than a generic line does.
                recording.isFailed ->
                    recording.error.ifBlank { stringResource(R.string.conf_rec_failed) }

                length != null && length.hours > 0 -> stringResource(
                    R.string.conf_rec_length_hours,
                    length.hours,
                    length.minutes,
                    length.seconds,
                )

                length != null -> stringResource(R.string.conf_rec_length, length.minutes, length.seconds)

                else -> ""
            }
            if (detail.isNotBlank()) {
                Text(detail, color = c.text3, fontSize = 11.sp, maxLines = 1)
            }
        }
        if (confRecordingDownloadable(recording)) {
            IonIconButton(
                Ion.DOWNLOAD,
                onClick = onDownload,
                enabled = !busy,
                boxSize = 34.dp,
                description = stringResource(R.string.conf_rec_download),
                modifier = Modifier.testTag(TestTags.conferenceRecordingDownload(recording.id)),
            )
        }
        if (canModerate) {
            IonIconButton(
                Ion.TRASH,
                onClick = onAskDelete,
                enabled = !busy,
                boxSize = 34.dp,
                description = stringResource(R.string.conf_rec_delete),
                modifier = Modifier.testTag(TestTags.conferenceRecordingDelete(recording.id)),
            )
            // A dialog rather than the popover «Завершить» uses: this one erases
            // a file for good, and the confirm has to be reachable by a spec —
            // which is what the tagged button in `TConfirmDialog` is for.
            if (confirming) {
                TConfirmDialog(
                    title = stringResource(R.string.conf_rec_delete),
                    message = stringResource(R.string.conf_rec_delete_confirm),
                    confirmText = stringResource(R.string.conf_rec_delete),
                    confirmTag = TestTags.CONFERENCE_RECORDING_DELETE_CONFIRM,
                    onConfirm = onConfirmDelete,
                    onDismiss = onCancelDelete,
                )
            }
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

/**
 * What a recording is handed to the system viewer as (#2896 §8).
 *
 * Stated rather than read off the row: the egress worker writes mp4 and the API
 * sends no content type with the row, so guessing from the file name would leave
 * a recording the server named oddly with no player willing to open it.
 */
private const val MP4_MIME = "video/mp4"
