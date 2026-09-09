package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceStatus
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.ReminderDateTimePicker
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TabItem
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.UnderlineTabs
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.TesseraLive
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.ConferencesViewModel
import website.msdnna.tessera.util.ConfTimeKind
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.RECORDING_TTL_DEFAULT
import website.msdnna.tessera.util.TtlInput
import website.msdnna.tessera.util.conferenceDraft
import website.msdnna.tessera.util.conferenceTimeLine
import website.msdnna.tessera.util.localDateTimeLabel
import website.msdnna.tessera.util.parseTtlDays

/**
 * Conferences list (#2896 §2, web `ConferencesView`): the workspace's calls, the
 * status filter and the «Запланировать» dialog. Opening one — the lobby, the
 * roster and joining — is §3.
 */
@Composable
fun ConferencesScreen(
    workspaceId: String,
    /** A call to open the lobby of straight away — the minimised bar's return (§9). */
    preselectConferenceId: String? = null,
    onPreselectConsumed: () -> Unit = {},
) {
    val c = Tessera.colors
    val vm: ConferencesViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(workspaceId) {
        if (workspaceId.isNotBlank()) vm.load(workspaceId)
    }

    // Separate from the load above, and not conditional on it: the lobby fetches
    // the call by its own id, so returning to a meeting must not wait on a list
    // that may still be loading — or be lost because the filter excludes it.
    LaunchedEffect(preselectConferenceId) {
        val id = preselectConferenceId ?: return@LaunchedEffect
        vm.openById(id)
        onPreselectConsumed()
    }

    // The lobby is an inline overlay, not a Dialog, so Back would otherwise fall
    // through to the shell's nav back-stack and leave the section entirely.
    BackHandler(enabled = state.openId != null) { vm.closeLobby() }

    Box(Modifier.fillMaxSize().background(c.bg).testTag(TestTags.CONFERENCES_SCREEN)) {
        Column(Modifier.fillMaxSize()) {
            Row(
                Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                UnderlineTabs(
                    tabs = ConferenceStatus.FILTERS.map {
                        TabItem(label = filterLabel(it), testTag = TestTags.conferenceFilter(it))
                    },
                    selected = ConferenceStatus.FILTERS.indexOf(state.filter).coerceAtLeast(0),
                    onSelect = { vm.setFilter(ConferenceStatus.FILTERS[it]) },
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                TButton(
                    stringResource(R.string.conf_schedule),
                    enabled = workspaceId.isNotBlank(),
                    onClick = { vm.compose() },
                    modifier = Modifier.testTag(TestTags.CONFERENCE_SCHEDULE),
                )
            }

            when {
                state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                state.list.isEmpty() -> ConferencesEmpty(
                    text = when {
                        workspaceId.isBlank() -> stringResource(R.string.conf_no_workspace)
                        state.filter == null -> stringResource(R.string.conf_empty)
                        else -> stringResource(R.string.conf_empty_filtered)
                    },
                )

                else -> LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 12.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(state.list, key = { it.id }) { conf ->
                        ConferenceRow(
                            conference = conf,
                            confirmingDelete = state.pendingDelete?.id == conf.id,
                            onClick = { vm.open(conf) },
                            onAskDelete = { vm.askDelete(conf) },
                            onConfirmDelete = { vm.confirmDelete() },
                            onCancelDelete = { vm.cancelDelete() },
                        )
                    }
                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }

        state.error?.let { message ->
            Box(Modifier.fillMaxSize().padding(16.dp), contentAlignment = Alignment.BottomCenter) {
                Text(message.resolve(), color = c.text3, fontSize = 12.sp)
            }
        }

        state.openId?.let { id ->
            ConferenceLobby(conferenceId = id, onBack = { vm.closeLobby() })
        }

        if (state.composing) {
            ScheduleConferenceDialog(
                saving = state.saving,
                error = state.createError?.resolve(),
                onSubmit = { title, description, at, ttl -> vm.create(title, description, at, ttl) },
                onDismiss = { vm.cancelCompose() },
            )
        }
    }
}

@Composable
private fun filterLabel(status: String?): String = stringResource(
    when (status) {
        ConferenceStatus.LIVE -> R.string.conf_filter_live
        ConferenceStatus.SCHEDULED -> R.string.conf_filter_scheduled
        ConferenceStatus.ENDED -> R.string.conf_filter_ended
        else -> R.string.conf_filter_all
    },
)

@Composable
private fun ConferencesEmpty(text: String) {
    val c = Tessera.colors
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            IonIcon(Ion.VIDEOCAM, size = 40.dp, tint = c.text3)
            Spacer(Modifier.height(10.dp))
            Text(text, color = c.text3, fontSize = 14.sp)
        }
    }
}

/**
 * One call. Stateless so the delete confirmation can be driven from a spec —
 * and `internal` for the same reason.
 */
@Composable
internal fun ConferenceRow(
    conference: Conference,
    confirmingDelete: Boolean,
    onClick: () -> Unit,
    onAskDelete: () -> Unit,
    onConfirmDelete: () -> Unit,
    onCancelDelete: () -> Unit,
) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick).padding(12.dp)
            .testTag(TestTags.conferenceRow(conference.id)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ConferenceStatusPill(conference.status)
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Text(
                conference.title.ifBlank { stringResource(R.string.conf_untitled) },
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
            )
            Spacer(Modifier.height(4.dp))
            Text(timeLineText(conference), color = c.text3, fontSize = 12.sp, maxLines = 1)
            Spacer(Modifier.height(2.dp))
            val invited = stringResource(R.string.conf_row_invited, conference.participantCount)
            // «В комнате: 0» is noise on a call nobody is in — the pill says so already.
            val inRoom = stringResource(R.string.conf_row_in_room, conference.activeCount)
                .takeIf { conference.activeCount > 0 }
            val author = conference.createdByName?.takeIf { it.isNotBlank() }
                ?.let { stringResource(R.string.conf_row_author, it) }
                ?: stringResource(R.string.conf_row_author_unknown)
            Text(
                listOfNotNull(invited, inRoom, author).joinToString(META_SEPARATOR),
                color = c.text3,
                fontSize = 12.sp,
                maxLines = 1,
            )
        }
        Box {
            IonIconButton(
                Ion.TRASH,
                onClick = onAskDelete,
                boxSize = 36.dp,
                tint = c.text3,
                modifier = Modifier.testTag(TestTags.conferenceDelete(conference.id)),
            )
            TConfirmPopover(
                expanded = confirmingDelete,
                message = stringResource(R.string.conf_confirm_delete),
                onConfirm = onConfirmDelete,
                onDismiss = onCancelDelete,
                confirmTag = TestTags.CONFERENCE_DELETE_CONFIRM,
            )
        }
    }
}

/** The one time line a row shows — see [conferenceTimeLine] for which one. */
@Composable
private fun timeLineText(conference: Conference): String {
    val line = conferenceTimeLine(conference)
    val at = line.iso?.let { localDateTimeLabel(LocalResources.current, it, LocalDateFormat.current) }.orEmpty()
    // An unparseable stamp renders as «Без времени начала» rather than as a
    // label with an empty tail («Начало: »).
    if (line.kind == ConfTimeKind.NONE || at.isBlank()) return stringResource(R.string.conf_row_no_time)
    return when (line.kind) {
        ConfTimeKind.ENDED -> stringResource(R.string.conf_row_ended_at, at)
        ConfTimeKind.STARTED -> stringResource(R.string.conf_row_started_at, at)
        else -> stringResource(R.string.conf_row_scheduled_at, at)
    }
}

/**
 * Status chip. Live and scheduled carry the same-hue gradient of the design
 * language; a finished call is neutral, and neutrals stay flat.
 *
 * Shared with the lobby (§3) rather than copied: the row and the detail must
 * never disagree about what «Идёт сейчас» looks like.
 */
@Composable
internal fun ConferenceStatusPill(status: String) {
    val c = Tessera.colors
    val hue = when (status) {
        ConferenceStatus.LIVE -> TesseraLive
        ConferenceStatus.SCHEDULED -> c.primary
        else -> null
    }
    val label = stringResource(
        when (status) {
            ConferenceStatus.LIVE -> R.string.conf_status_live
            ConferenceStatus.SCHEDULED -> R.string.conf_status_scheduled
            else -> R.string.conf_status_ended
        },
    )
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm))
            .border(1.dp, hue?.copy(alpha = 0.45f) ?: c.border, RoundedCornerShape(RadiusSm))
            .padding(horizontal = 8.dp, vertical = 4.dp),
    ) {
        Text(
            label,
            color = hue ?: c.text3,
            fontSize = 11.sp,
            fontWeight = FontWeight.Medium,
            style = if (hue != null) TextStyle(brush = accentGradient(hue)) else TextStyle.Default,
        )
    }
}

/**
 * «Запланировать» (web `ConferencesView`'s dialog). Stateless apart from the
 * form's own text, and `internal` so a spec can drive it without a socket.
 */
@Composable
internal fun ScheduleConferenceDialog(
    saving: Boolean,
    error: String?,
    onSubmit: (title: String, description: String, scheduledAtIso: String?, ttlDays: Int) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    var title by remember { mutableStateOf("") }
    var description by remember { mutableStateOf("") }
    var at by remember { mutableStateOf<String?>(null) }
    var ttlText by remember { mutableStateOf(RECORDING_TTL_DEFAULT.toString()) }
    var showPicker by remember { mutableStateOf(false) }

    val ttl = parseTtlDays(ttlText)
    // The same read decides both whether the button is live and what it sends,
    // so the two cannot drift apart — see [conferenceDraft].
    val draft = conferenceDraft(title, description, at, ttlText)
    val canSubmit = draft != null && !saving

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surface)
                .heightIn(max = 560.dp).verticalScroll(rememberScrollState()).padding(20.dp)
                .testTag(TestTags.CONFERENCE_SCHEDULE_DIALOG),
        ) {
            Text(
                stringResource(R.string.conf_create_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(14.dp))
            TTextField(
                value = title,
                onValueChange = { title = it },
                label = stringResource(R.string.conf_create_name),
                placeholder = stringResource(R.string.conf_create_name_placeholder),
                fieldTag = TestTags.CONFERENCE_CREATE_NAME,
            )
            Spacer(Modifier.height(12.dp))
            TTextField(
                value = description,
                onValueChange = { description = it },
                label = stringResource(R.string.conf_create_description),
                placeholder = stringResource(R.string.conf_create_description_placeholder),
                singleLine = false,
            )

            Spacer(Modifier.height(12.dp))
            Text(
                stringResource(R.string.conf_create_at),
                color = c.text2,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
            )
            Spacer(Modifier.height(6.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Row(
                    Modifier.weight(1f).clip(RoundedCornerShape(RadiusMd))
                        .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                        .clickableNoRipple { showPicker = true }
                        .padding(horizontal = 12.dp, vertical = 11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    IonIcon(Ion.TIME, size = 16.dp, tint = c.text3)
                    Spacer(Modifier.width(8.dp))
                    Text(
                        at?.let { localDateTimeLabel(LocalResources.current, it, LocalDateFormat.current) }
                            ?: stringResource(R.string.conf_create_at_empty),
                        color = if (at != null) c.text1 else c.placeholder,
                        fontSize = 14.sp,
                    )
                }
                if (at != null) {
                    Spacer(Modifier.width(6.dp))
                    IonIconButton(Ion.CLOSE, onClick = { at = null }, boxSize = 36.dp, tint = c.text3)
                }
            }
            Spacer(Modifier.height(4.dp))
            Text(stringResource(R.string.conf_create_at_hint), color = c.text3, fontSize = 12.sp)

            Spacer(Modifier.height(12.dp))
            TTextField(
                value = ttlText,
                onValueChange = { ttlText = it },
                label = stringResource(R.string.conf_create_ttl),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                fieldTag = TestTags.CONFERENCE_CREATE_TTL,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                if (ttl is TtlInput.Days) {
                    stringResource(R.string.conf_create_ttl_hint)
                } else {
                    stringResource(R.string.conf_create_ttl_invalid)
                },
                color = if (ttl is TtlInput.Days) c.text3 else TesseraDanger,
                fontSize = 12.sp,
            )

            TFormError(error, Modifier.padding(top = 10.dp))

            Spacer(Modifier.height(18.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton(stringResource(R.string.common_cancel), kind = TButtonKind.Ghost, onClick = onDismiss)
                Spacer(Modifier.width(8.dp))
                TButton(
                    stringResource(R.string.conf_create_submit),
                    enabled = canSubmit,
                    loading = saving,
                    onClick = {
                        val d = draft ?: return@TButton
                        onSubmit(d.title, d.description, d.scheduledAtIso, d.ttlDays)
                    },
                    modifier = Modifier.testTag(TestTags.CONFERENCE_CREATE_SUBMIT),
                )
            }
        }
    }

    if (showPicker) {
        ReminderDateTimePicker(
            initialIso = at,
            onPick = {
                at = it
                showPicker = false
            },
            onDismiss = { showPicker = false },
        )
    }
}

/** «·» between the row's meta facts, matching the web's inline list. */
private const val META_SEPARATOR = " · "
