package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.UpdateState
import website.msdnna.tessera.util.Ion

/** Danger red shared with the rest of the app (web `--t-danger`). */
private val DangerRed = Color(0xFFE0533D)

/**
 * The in-app update prompt: a centred card over a scrim. Shows the available
 * version + notes, a download progress bar, then an install action. Tapping the
 * scrim dismisses it (except mid-download).
 */
@Composable
fun UpdateDialog(
    state: UpdateState,
    onUpdate: () -> Unit,
    onInstall: () -> Unit,
    onDismiss: () -> Unit,
) {
    val release = when (state) {
        is UpdateState.Available -> state.release
        is UpdateState.Downloading -> state.release
        is UpdateState.Ready -> state.release
        is UpdateState.Failed -> state.release
        UpdateState.Idle -> return
    }
    val c = Tessera.colors
    val downloading = state is UpdateState.Downloading

    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.5f))
            .clickableNoRipple { if (!downloading) onDismiss() },
        contentAlignment = Alignment.Center,
    ) {
        // Swallow taps on the card so they don't reach the dismiss scrim.
        Box(Modifier.popupAppear(TransformOrigin.Center).padding(28.dp).clickableNoRipple {}) {
            TCard(modifier = Modifier.widthIn(max = 360.dp)) {
                Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier.size(38.dp).clip(CircleShape).background(accentGradient(c.primary)),
                            contentAlignment = Alignment.Center,
                        ) {
                            IonIcon(Ion.DOWNLOAD, size = 20.dp, tint = c.onPrimary)
                        }
                        Spacer(Modifier.width(12.dp))
                        Column {
                            Text(
                                stringResource(R.string.update_available),
                                color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold,
                            )
                            Text(
                                stringResource(R.string.update_version, release.version),
                                color = c.text3, fontSize = 13.sp,
                            )
                        }
                    }

                    if (release.notes.isNotBlank()) {
                        Text(release.notes, color = c.text2, fontSize = 13.sp)
                    }

                    when (state) {
                        is UpdateState.Downloading -> {
                            LinearProgressIndicator(
                                progress = { state.progress },
                                modifier = Modifier.fillMaxWidth(),
                                color = c.primary,
                                trackColor = c.surfaceAlt,
                            )
                            Text(
                                stringResource(R.string.update_downloading, (state.progress * 100).toInt()),
                                color = c.text3,
                                fontSize = 12.sp,
                            )
                        }

                        is UpdateState.Ready -> {
                            TButton(
                                text = stringResource(R.string.update_install),
                                onClick = onInstall,
                                modifier = Modifier.fillMaxWidth(),
                            )
                        }

                        is UpdateState.Failed -> {
                            Text(state.message.resolve(), color = DangerRed, fontSize = 13.sp)
                            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                TButton(
                                    stringResource(R.string.update_later),
                                    onClick = onDismiss,
                                    kind = TButtonKind.Secondary,
                                    modifier = Modifier.weight(1f),
                                )
                                TButton(
                                    stringResource(R.string.common_retry),
                                    onClick = onUpdate,
                                    modifier = Modifier.weight(1f),
                                )
                            }
                        }

                        else -> {
                            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                TButton(
                                    stringResource(R.string.update_later),
                                    onClick = onDismiss,
                                    kind = TButtonKind.Secondary,
                                    modifier = Modifier.weight(1f),
                                )
                                TButton(
                                    stringResource(R.string.update_action),
                                    onClick = onUpdate,
                                    modifier = Modifier.weight(1f),
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}
