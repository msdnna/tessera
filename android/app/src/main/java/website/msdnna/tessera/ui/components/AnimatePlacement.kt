package website.msdnna.tessera.ui.components

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.AnimationVector2D
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.VectorConverter
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.offset
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.onPlaced
import androidx.compose.ui.layout.positionInParent
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.round
import kotlinx.coroutines.launch

/**
 * Animates a composable sliding to its new position whenever its placement in the
 * parent changes (e.g. when a sibling collapses or a placeholder is inserted
 * during a drag) — the smooth "cards make room" reorder feel. Standard recipe:
 * record the placed offset, and render at the difference between the animated and
 * the target offset.
 */
fun Modifier.animatePlacement(): Modifier = composed {
    val scope = rememberCoroutineScope()
    var target by remember { mutableStateOf(IntOffset.Zero) }
    var animatable by remember { mutableStateOf<Animatable<IntOffset, AnimationVector2D>?>(null) }
    this
        .onPlaced { target = it.positionInParent().round() }
        .offset {
            val anim = animatable ?: Animatable(target, IntOffset.VectorConverter).also { animatable = it }
            if (anim.targetValue != target) {
                scope.launch { anim.animateTo(target, spring(stiffness = Spring.StiffnessMediumLow)) }
            }
            anim.value - target
        }
}

/**
 * Fades the composable in (alpha 0→1) when it first appears — so a card that
 * lands in a NEW column (a fresh composable in that column's list) gently
 * materialises instead of snapping in. Runs once per composition, so it doesn't
 * re-fire on plain data updates (the same slot persists).
 */
fun Modifier.fadeInOnAppear(durationMs: Int = 220): Modifier = composed {
    val alpha = remember { Animatable(0f) }
    LaunchedEffect(Unit) { alpha.animateTo(1f, tween(durationMs)) }
    this.graphicsLayer { this.alpha = alpha.value }
}
