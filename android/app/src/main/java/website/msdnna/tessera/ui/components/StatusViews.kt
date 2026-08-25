package website.msdnna.tessera.ui.components

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.Ion

/**
 * Reassuring captions shown under a loader when a load drags on.
 *
 * A `val` list would freeze on the language of the first class load and survive a
 * language switch in the profile — the captions read the resources of the
 * composition that asks for them instead.
 */
@Composable
fun defaultLoadingMessages(): List<String> = listOf(
    stringResource(R.string.loading_caption_connecting),
    stringResource(R.string.loading_caption_slow),
    stringResource(R.string.loading_caption_still_trying),
)

/**
 * Soft, fading captions that appear under a loader only after [startDelayMs] of
 * waiting, then cross-fade through [messages] every [intervalMs] and settle on
 * the last one. Nothing shows for a fast load (the common case). [color] should
 * suit the background (white on the purple splash, a muted theme tone in-app).
 */
@Composable
fun LoadingCaptions(
    color: Color,
    modifier: Modifier = Modifier,
    startDelayMs: Long = 5_000,
    intervalMs: Long = 3_500,
    messages: List<String> = defaultLoadingMessages(),
) {
    var index by remember { mutableIntStateOf(-1) }
    LaunchedEffect(messages, startDelayMs, intervalMs) {
        delay(startDelayMs)
        index = 0
        while (index < messages.size - 1) {
            delay(intervalMs)
            index += 1
        }
    }

    AnimatedContent(
        targetState = index,
        transitionSpec = { fadeIn(tween(600)) togetherWith fadeOut(tween(600)) },
        label = "loading-caption",
        modifier = modifier,
    ) { i ->
        if (i < 0) {
            Spacer(Modifier.height(18.dp))
        } else {
            Text(
                messages[i],
                color = color,
                fontSize = 13.sp,
                textAlign = TextAlign.Center,
                modifier = Modifier.widthIn(max = 280.dp).padding(horizontal = 24.dp),
            )
        }
    }
}

/**
 * Centered in-app loading state: the brand loader plus the delayed captions,
 * themed for the app (used while a board/screen fetches).
 */
@Composable
fun LoadingState(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            TesseraLoader()
            Spacer(Modifier.height(18.dp))
            LoadingCaptions(color = Tessera.colors.text3)
        }
    }
}

/**
 * Centered in-app error state: a concrete message (e.g. «500: context deadline
 * exceeded») and a single «Попробовать ещё раз» action, themed to the app.
 */
@Composable
fun ErrorState(message: String, onRetry: () -> Unit, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Box(modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(
                message,
                color = c.text2,
                fontSize = 14.sp,
                textAlign = TextAlign.Center,
                modifier = Modifier.widthIn(max = 320.dp),
            )
            TButton(stringResource(R.string.error_retry_action), onClick = onRetry, icon = Ion.REFRESH)
        }
    }
}
