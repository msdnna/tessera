package website.msdnna.tessera.ui.screens

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.conference.ConfPeer
import website.msdnna.tessera.data.conference.ConfRoomEnd
import website.msdnna.tessera.data.conference.ConfVideo
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.TesseraWarning
import website.msdnna.tessera.ui.viewmodels.ConferenceChatViewModel
import website.msdnna.tessera.ui.viewmodels.ConferenceRoomUiState
import website.msdnna.tessera.ui.viewmodels.ConferenceRoomViewModel
import website.msdnna.tessera.util.ConfAudioRoute
import website.msdnna.tessera.util.ConfBanner
import website.msdnna.tessera.util.ConfMediaGrant
import website.msdnna.tessera.util.ConfQuality
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.confCanRetry
import website.msdnna.tessera.util.confHandCount

/**
 * The call itself (#2896 §5, web `ConferenceRoom.vue`): the stage, the strip of
 * everybody else and the toolbar.
 *
 * Shown over the lobby exactly while we hold a seat. Membership is the lobby's
 * to decide and media follows it, never the other way round — the same one-way
 * rule the web states, and the reason somebody who left the roster cannot still
 * be heard by the room.
 *
 * The three things the web has to explain and this screen does not: an insecure
 * page, a blocked autoplay and a device picker. Android has no such policies,
 * and its device list is one question — which speaker — answered by the routing
 * menu instead of three dropdowns of hardware ids.
 */
@Composable
fun ConferenceRoom(conferenceId: String, onHangup: () -> Unit) {
    val vm: ConferenceRoomViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()
    val chatVm: ConferenceChatViewModel = viewModel()
    val chat by chatVm.state.collectAsStateWithLifecycle()
    val ctx = LocalContext.current

    // Asked for from here rather than at startup: a microphone permission
    // requested by an app that is not in a call is a dialog with no explanation
    // attached, and the usual answer to that one is «нет».
    val micLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted -> vm.micGranted(granted) }
    val camLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted -> vm.setCam(granted, granted) }

    LaunchedEffect(conferenceId) {
        val mic = ctx.hasPermission(Manifest.permission.RECORD_AUDIO)
        // Connect either way. A call joined without a microphone is a call you
        // can still listen to, and the banner says why nobody can hear you —
        // far better than a lobby button that appears to do nothing.
        vm.enter(conferenceId, ConfMediaGrant(mic = mic, cam = false))
        if (!mic) micLauncher.launch(Manifest.permission.RECORD_AUDIO)
    }

    // Until §9's minimised bar exists there is nowhere for a call to live
    // without a screen, so leaving this one ends it. Tied to composition rather
    // than to the view model: the model is scoped to the activity and would
    // outlive the room by the whole session.
    DisposableEffect(conferenceId) { onDispose { vm.exit() } }

    val view = LocalView.current
    DisposableEffect(Unit) {
        view.keepScreenOn = true
        onDispose { view.keepScreenOn = false }
    }

    // The room threw us out, or it ended for everyone. Leaving is not a choice
    // at that point — the socket refuses to reconnect and the media is already
    // gone, so a screen that stayed would be a still photograph of a call.
    LaunchedEffect(state.room.ended) {
        if (state.room.ended != ConfRoomEnd.NONE) onHangup()
    }

    // Bound to the call, not to the sheet: the badge has to count while the chat
    // is closed, which it cannot do without the list behind it.
    LaunchedEffect(conferenceId, state.room.canModerate) {
        chatVm.bind(conferenceId, state.room.canModerate)
    }
    LaunchedEffect(state.room.chatNudge) { chatVm.onNudge(state.room.chatNudge) }

    Box(Modifier.fillMaxSize()) {
        ConferenceRoomBody(
            state = state,
            onToggleMic = {
                if (state.grant.mic) vm.toggleMic() else micLauncher.launch(Manifest.permission.RECORD_AUDIO)
            },
            onToggleCam = {
                when {
                    state.session.cam -> vm.setCam(false)
                    state.grant.cam -> vm.setCam(true)
                    else -> camLauncher.launch(Manifest.permission.CAMERA)
                }
            },
            onSwitchCamera = { vm.switchCamera() },
            onOpenRoutes = { vm.openRouteMenu() },
            onCloseRoutes = { vm.closeRouteMenu() },
            onSelectRoute = { vm.selectRoute(it) },
            onToggleStageOnly = { vm.toggleStageOnly() },
            onRetry = { vm.retry() },
            onHangup = {
                vm.exit()
                onHangup()
            },
            onToggleHand = { vm.toggleHand() },
            onOpenPanel = { vm.openPanel() },
            onClosePanel = { vm.closePanel() },
            onForceMute = { id, muted -> vm.forceMute(id, muted) },
            onAskKick = { vm.askKick(it) },
            onCancelKick = { vm.cancelKick() },
            onConfirmKick = { vm.confirmKick() },
            onLocalMute = { vm.toggleLocalMute(it) },
            onVolume = { id, v -> vm.setPeerVolume(id, v) },
            onDismissDenied = { vm.clearDenied() },
            chatUnread = chat.unread,
            onOpenChat = { chatVm.open() },
        )

        // Over the room rather than inside its body: the sheet needs a picker, a
        // resolver and a view model, none of which the body — which a spec mounts
        // against a call that never existed — is allowed to know about.
        if (chat.open) ConferenceChatHost(chatVm, chat)
    }
}

private fun android.content.Context.hasPermission(name: String): Boolean =
    ContextCompat.checkSelfPermission(this, name) == PackageManager.PERMISSION_GRANTED

/**
 * The room, driven purely by [state]. Stateless (and `internal`) so a spec can
 * mount a call that never existed — the screen above it needs an SFU, a token
 * and a microphone before it renders a thing.
 */
@Composable
internal fun ConferenceRoomBody(
    state: ConferenceRoomUiState,
    onToggleMic: () -> Unit,
    onToggleCam: () -> Unit,
    onSwitchCamera: () -> Unit,
    onOpenRoutes: () -> Unit,
    onCloseRoutes: () -> Unit,
    onSelectRoute: (ConfAudioRoute) -> Unit,
    onToggleStageOnly: () -> Unit,
    onRetry: () -> Unit,
    onHangup: () -> Unit,
    onToggleHand: () -> Unit = {},
    onOpenPanel: () -> Unit = {},
    onClosePanel: () -> Unit = {},
    onForceMute: (String, Boolean) -> Unit = { _, _ -> },
    onAskKick: (String) -> Unit = {},
    onCancelKick: () -> Unit = {},
    onConfirmKick: () -> Unit = {},
    onLocalMute: (String) -> Unit = {},
    onVolume: (String, Float) -> Unit = { _, _ -> },
    onDismissDenied: () -> Unit = {},
    chatUnread: Int = 0,
    onOpenChat: () -> Unit = {},
) {
    val c = Tessera.colors
    val layout = state.stage

    Box(Modifier.fillMaxSize()) {
        Column(
            Modifier.fillMaxSize().background(c.bg).testTag(TestTags.CONFERENCE_ROOM),
        ) {
            if (!state.stageOnly) ConferenceBanner(state.banner, state.session.error, onRetry)

            // Folding is the button's job, not the stage's. A `clickable` here would
            // merge the semantics of everything under it, and the tiles' own tags —
            // the thing every spec selects by — would vanish into this node.
            Box(
                Modifier.fillMaxWidth().weight(1f).padding(horizontal = 10.dp),
                contentAlignment = Alignment.Center,
            ) {
                val stage = layout.stage
                if (stage == null) {
                    Text(
                        stringResource(R.string.conf_media_no_one),
                        color = c.text3,
                        fontSize = 13.sp,
                    )
                } else {
                    // The marker is a wrapper rather than a second `testTag` on the
                    // tile: two of them on one node keep the outer value, and the
                    // tile would stop answering to the identity every spec uses.
                    Box(Modifier.fillMaxSize().testTag(TestTags.CONFERENCE_STAGE)) {
                        ConferenceTile(peer = stage, screen = layout.screen, modifier = Modifier.fillMaxSize())
                    }
                }
                // Same affordance as the web's fullscreen button, doing the local
                // equivalent: there is no browser chrome to escape here, so it folds
                // away ours.
                IonIcon(
                    if (state.stageOnly) Ion.CONTRACT else Ion.EXPAND,
                    size = 18.dp,
                    tint = c.text2,
                    modifier = Modifier.align(Alignment.TopEnd).padding(6.dp)
                        .clip(CircleShape)
                        .background(c.surface.copy(alpha = 0.85f))
                        .clickableNoRipple(onClick = onToggleStageOnly)
                        .padding(7.dp)
                        .testTag(TestTags.CONFERENCE_FULLSCREEN),
                )
            }

            if (!state.stageOnly && layout.strip.isNotEmpty()) {
                LazyRow(
                    Modifier.fillMaxWidth().height(96.dp).padding(horizontal = 10.dp, vertical = 6.dp)
                        .testTag(TestTags.CONFERENCE_STRIP),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    items(layout.strip.size, key = { layout.strip[it].identity }) { i ->
                        ConferenceTile(
                            peer = layout.strip[i],
                            screen = false,
                            // Height-first: a `fillMaxSize` inside a row that scrolls
                            // horizontally has no width to fill, and the tile comes
                            // out zero-wide — present in the tree, invisible on screen.
                            modifier = Modifier.fillMaxHeight().aspectRatio(RATIO_16_9),
                        )
                    }
                }
            }

            if (!state.stageOnly) {
                ConferenceToolbar(
                    state = state,
                    onToggleMic = onToggleMic,
                    onToggleCam = onToggleCam,
                    onSwitchCamera = onSwitchCamera,
                    onOpenRoutes = onOpenRoutes,
                    onCloseRoutes = onCloseRoutes,
                    onSelectRoute = onSelectRoute,
                    onToggleHand = onToggleHand,
                    onOpenPanel = onOpenPanel,
                    onHangup = onHangup,
                    chatUnread = chatUnread,
                    onOpenChat = onOpenChat,
                )
            }
        }

        if (state.panelOpen) {
            ConferenceParticipantsPanel(
                state = state,
                onClose = onClosePanel,
                onForceMute = onForceMute,
                onAskKick = onAskKick,
                onCancelKick = onCancelKick,
                onConfirmKick = onConfirmKick,
                onLocalMute = onLocalMute,
                onVolume = onVolume,
                onDismissDenied = onDismissDenied,
            )
        }
    }
}

/**
 * One participant.
 *
 * Built around the avatar with room made for a stream, not the other way round:
 * this team rarely turns a camera on, and a tile designed for video spends most
 * of every call showing a grey box.
 */
@Composable
private fun ConferenceTile(peer: ConfPeer, screen: Boolean, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    val track = if (screen) peer.screenTrack else peer.videoTrack
    // The speaking ring is a reserved transparent border rather than one that
    // appears: a border that materialises resizes the tile under it, and on the
    // strip that shifts every other face sideways once a second.
    val ring = if (!screen && peer.speaking) c.primary else Color.Transparent

    Box(
        modifier
            .clip(RoundedCornerShape(RadiusMd))
            .background(c.surfaceAlt)
            .border(2.dp, ring, RoundedCornerShape(RadiusMd))
            .testTag(TestTags.conferenceTile(peer.identity)),
        contentAlignment = Alignment.Center,
    ) {
        if (track != null) {
            ConfVideo(peer = peer, screen = screen, modifier = Modifier.fillMaxSize())
        } else if (screen) {
            // A presenter whose first frame is still in flight — not somebody
            // without a camera, so their avatar would be the wrong answer.
            IonIcon(Ion.VIDEOCAM, size = 30.dp, tint = c.text3)
        } else {
            MemberAvatar(if (peer.local) 44.dp else 36.dp, peer.name, userId = peer.identity)
        }

        if (peer.quality.weak) {
            IonIcon(
                Ion.CELLULAR,
                size = 13.dp,
                tint = if (peer.quality == ConfQuality.LOST) TesseraDanger else TesseraWarning,
                modifier = Modifier.align(Alignment.TopStart).padding(6.dp)
                    .testTag(TestTags.CONFERENCE_QUALITY),
            )
        }

        Row(
            Modifier.align(Alignment.BottomStart).padding(6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                peer.name,
                color = c.text1,
                fontSize = 11.sp,
                maxLines = 1,
                modifier = Modifier.background(c.surface.copy(alpha = 0.8f)).padding(horizontal = 5.dp, vertical = 2.dp),
            )
            if (!screen && !peer.micOn) {
                Spacer(Modifier.width(4.dp))
                IonIcon(
                    Ion.MIC_OFF,
                    size = 13.dp,
                    tint = c.text3,
                    modifier = Modifier.testTag(TestTags.conferenceTileMuted(peer.identity)),
                )
            }
        }
    }
}

/** «Подключаемся…», «Переподключаемся», a refusal — one line, most final first. */
@Composable
private fun ConferenceBanner(banner: ConfBanner, error: String, onRetry: () -> Unit) {
    if (banner == ConfBanner.NONE) return
    val c = Tessera.colors
    val text = when (banner) {
        ConfBanner.UNAVAILABLE -> stringResource(R.string.conf_media_unavailable)

        ConfBanner.ENDED -> stringResource(R.string.conf_media_ended)

        ConfBanner.KICKED -> stringResource(R.string.conf_media_kicked)

        // The server's own sentence when it sent one: it knows why better than
        // a generic line does.
        ConfBanner.ERROR -> error.ifBlank { stringResource(R.string.conf_media_failed) }

        ConfBanner.RECONNECTING -> stringResource(R.string.conf_media_reconnecting)

        ConfBanner.CONNECTING -> stringResource(R.string.conf_media_connecting)

        ConfBanner.NO_MIC -> stringResource(R.string.conf_media_no_mic)

        ConfBanner.NONE -> ""
    }
    Row(
        Modifier.fillMaxWidth().background(c.surfaceAlt).padding(horizontal = 12.dp, vertical = 8.dp)
            .testTag(TestTags.CONFERENCE_BANNER),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.WARNING, size = 15.dp, tint = c.text3)
        Spacer(Modifier.width(8.dp))
        Text(text, color = c.text2, fontSize = 12.sp, modifier = Modifier.weight(1f))
        if (confCanRetry(banner)) {
            Spacer(Modifier.width(8.dp))
            Text(
                stringResource(R.string.conf_media_retry),
                color = c.primary,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.clickableNoRipple(onClick = onRetry).testTag(TestTags.CONFERENCE_RETRY),
            )
        }
    }
}

@Composable
private fun ConferenceToolbar(
    state: ConferenceRoomUiState,
    onToggleMic: () -> Unit,
    onToggleCam: () -> Unit,
    onSwitchCamera: () -> Unit,
    onOpenRoutes: () -> Unit,
    onCloseRoutes: () -> Unit,
    onSelectRoute: (ConfAudioRoute) -> Unit,
    onToggleHand: () -> Unit,
    onOpenPanel: () -> Unit,
    onHangup: () -> Unit,
    chatUnread: Int,
    onOpenChat: () -> Unit,
) {
    val c = Tessera.colors
    val controls = state.controls
    // A flow, not a row: eight 48dp controls with gaps between them are wider
    // than a 411dp phone, and a Row overflowing does not shrink or scroll — it
    // clips, and the button it clips is the one on the end. Wrapping costs a
    // second line on a narrow screen and keeps «Завершить» reachable on every one.
    FlowRow(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp, Alignment.CenterHorizontally),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        RoundControl(
            icon = if (controls.micOn) Ion.MIC else Ion.MIC_OFF,
            active = controls.micOn,
            enabled = controls.micEnabled,
            tag = TestTags.CONFERENCE_MIC,
            onClick = onToggleMic,
        )
        RoundControl(
            icon = if (controls.camOn) Ion.VIDEOCAM else Ion.VIDEOCAM_OFF,
            active = controls.camOn,
            enabled = controls.camEnabled,
            tag = TestTags.CONFERENCE_CAM,
            onClick = onToggleCam,
        )
        RoundControl(
            icon = Ion.CAMERA_REVERSE,
            active = false,
            enabled = controls.switchCamEnabled,
            tag = TestTags.CONFERENCE_SWITCH_CAM,
            onClick = onSwitchCamera,
        )
        Box {
            RoundControl(
                icon = Ion.VOLUME_HIGH,
                active = false,
                enabled = controls.routeEnabled,
                tag = TestTags.CONFERENCE_ROUTE,
                onClick = onOpenRoutes,
            )
            TDropdown(expanded = state.routeMenu, onDismiss = onCloseRoutes) {
                state.session.routes.forEach { route ->
                    Row(
                        Modifier.fillMaxWidth()
                            .clickableNoRipple { onSelectRoute(route) }
                            .padding(horizontal = 12.dp, vertical = 9.dp)
                            .testTag(TestTags.conferenceRoute(route)),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(routeLabel(route), color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                        if (route == state.session.route) {
                            Spacer(Modifier.width(10.dp))
                            IonIcon(Ion.CHECK, size = 15.dp, tint = c.primary, gradient = true)
                        }
                    }
                }
            }
        }
        // Raising a hand needs no microphone and no camera, so it stays enabled
        // wherever the toolbar is: a force-muted participant asking to speak is
        // exactly who this control is for.
        RoundControl(
            icon = Ion.HAND_RIGHT,
            active = state.room.handUp,
            enabled = state.room.connected,
            tag = TestTags.CONFERENCE_HAND,
            // Named by what the press will do, not by what is on screen: a
            // toolbar of icons says nothing about which of them is already on.
            description = stringResource(
                if (state.room.handUp) R.string.conf_panel_lower_hand else R.string.conf_panel_raise_hand,
            ),
            onClick = onToggleHand,
        )
        Box {
            RoundControl(
                icon = Ion.PEOPLE,
                active = false,
                // The roster comes from the room socket, not the SFU: the panel
                // opens on a call whose media never connected, which is when
                // «кто вообще здесь» is asked most.
                enabled = state.room.connected,
                tag = TestTags.CONFERENCE_PEOPLE,
                onClick = onOpenPanel,
            )
            // Hands are the one thing in the roster that is waiting on somebody,
            // so the count on the closed panel is the hands and not the heads —
            // a badge reading «5» on every call is a badge nobody looks at.
            val hands = confHandCount(state.room.people)
            if (hands > 0) {
                Text(
                    hands.toString(),
                    color = c.onPrimary,
                    fontSize = 10.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.align(Alignment.TopEnd)
                        .clip(CircleShape)
                        .background(TesseraWarning)
                        .padding(horizontal = 5.dp, vertical = 1.dp)
                        .testTag(TestTags.CONFERENCE_HANDS),
                )
            }
        }
        Box {
            RoundControl(
                icon = Ion.CHATBUBBLES,
                active = false,
                // Same reasoning as the roster: the chat is HTTP, and a call
                // whose media never came up can still be typed in.
                enabled = state.room.connected,
                tag = TestTags.CONFERENCE_CHAT_OPEN,
                description = stringResource(R.string.conf_chat_open),
                onClick = onOpenChat,
            )
            if (chatUnread > 0) {
                Text(
                    chatUnread.toString(),
                    color = c.onPrimary,
                    fontSize = 10.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.align(Alignment.TopEnd)
                        .clip(CircleShape)
                        .background(c.primary)
                        .padding(horizontal = 5.dp, vertical = 1.dp)
                        .testTag(TestTags.CONFERENCE_CHAT_UNREAD),
                )
            }
        }
        RoundControl(
            icon = Ion.CALL,
            active = false,
            enabled = true,
            danger = true,
            tag = TestTags.CONFERENCE_HANGUP,
            onClick = onHangup,
        )
    }
}

/** A circular toolbar button, the shape the web's `n-button circle` gives. */
@Composable
private fun RoundControl(
    icon: String,
    active: Boolean,
    enabled: Boolean,
    tag: String,
    onClick: () -> Unit,
    danger: Boolean = false,
    description: String? = null,
) {
    val c = Tessera.colors
    val bg = when {
        danger -> TesseraDanger
        active -> c.primary
        else -> c.surfaceAlt
    }
    val fg = when {
        danger || active -> c.onPrimary
        else -> c.text2
    }
    Box(
        Modifier.size(48.dp)
            .clip(CircleShape)
            .background(if (enabled) bg else bg.copy(alpha = 0.4f))
            .clickableNoRipple(enabled = enabled, onClick = onClick)
            .testTag(tag),
        contentAlignment = Alignment.Center,
    ) {
        IonIcon(
            icon,
            size = 21.dp,
            tint = if (enabled) fg else fg.copy(alpha = 0.5f),
            description = description,
        )
    }
}

@Composable
private fun routeLabel(route: ConfAudioRoute): String = stringResource(
    when (route) {
        ConfAudioRoute.BLUETOOTH -> R.string.conf_route_bluetooth
        ConfAudioRoute.WIRED_HEADSET -> R.string.conf_route_wired
        ConfAudioRoute.SPEAKER -> R.string.conf_route_speaker
        ConfAudioRoute.EARPIECE -> R.string.conf_route_earpiece
    },
)

private const val RATIO_16_9 = 16f / 9f
