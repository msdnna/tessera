package website.msdnna.tessera.ui.screens

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.LocalTextSelectionColors
import androidx.compose.foundation.text.selection.TextSelectionColors
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil.compose.AsyncImage
import coil.request.ImageRequest
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.MtLogo
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.AuthViewModel
import website.msdnna.tessera.util.Ion

/** The fixed brand purple of the launch splash — the auth screen shares it. */
private val BrandPurple = Color(0xFF7C6CFF)

/**
 * Login / register, presented on the brand purple gradient (matching the launch
 * splash): a large white monogram, then the form rendered directly on the
 * gradient as frosted inputs. Server settings hide behind a gear popover and
 * apply live — whatever is in the field is the server used, no save button.
 */
@Composable
fun AuthScreen(
    serverUrl: String,
    onServerUrlChange: (String) -> Unit,
    vm: AuthViewModel = viewModel(),
) {
    val c = Tessera.colors
    val state by vm.state.collectAsStateWithLifecycle()

    // Logo sized so its WIDTH is half the screen (MtLogo's width = height * 120/100).
    val logoSize = (LocalConfiguration.current.screenWidthDp.dp * 0.5f) / (120f / 100f)

    var register by rememberSaveable { mutableStateOf(false) }
    var email by rememberSaveable { mutableStateOf("") }
    var name by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var showServer by rememberSaveable { mutableStateOf(false) }
    var serverDraft by remember(serverUrl) { mutableStateOf(serverUrl) }

    Box(
        Modifier
            .fillMaxSize()
            .background(accentGradient(BrandPurple)),
    ) {
        // Airy drifting aurora over the brand gradient (mirrors the web login).
        AuthAurora(Modifier.fillMaxSize())

        // Server settings — gear in the top-right, popover with a live URL field.
        Box(
            Modifier
                .align(Alignment.TopEnd)
                .windowInsetsPadding(WindowInsets.statusBars)
                .padding(6.dp),
        ) {
            // Active state so it's clear the popover belongs to this gear.
            Box(
                Modifier
                    .clip(CircleShape)
                    .background(if (showServer) Color.White.copy(alpha = 0.18f) else Color.Transparent),
            ) {
                IonIconButton(
                    Ion.SETTINGS,
                    onClick = { showServer = !showServer },
                    boxSize = 40.dp,
                    tint = Color.White,
                )
            }
            TDropdown(expanded = showServer, onDismiss = { showServer = false }, bare = true) {
                // Caret + card, right-aligned so the caret apex sits under the gear.
                Column(horizontalAlignment = Alignment.End) {
                    PopoverCaret(Modifier.padding(end = 18.dp))
                    TCard(modifier = Modifier.widthIn(min = 264.dp, max = 300.dp)) {
                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            Text("Сервер", color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                            TTextField(
                                value = serverDraft,
                                onValueChange = {
                                    serverDraft = it
                                    onServerUrlChange(it)
                                },
                                placeholder = "https://tessera.website.msdnna",
                                modifier = Modifier.fillMaxWidth(),
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                            )
                            Text(
                                "Применяется сразу. Пусто — сервер по умолчанию.",
                                color = c.text3,
                                fontSize = 11.sp,
                            )
                        }
                    }
                }
            }
        }

        Column(
            modifier = Modifier
                .verticalScroll(rememberScrollState())
                .widthIn(max = 400.dp)
                .fillMaxWidth()
                .windowInsetsPadding(WindowInsets.safeDrawing)
                .imePadding()
                .padding(horizontal = 28.dp, vertical = 40.dp)
                .align(Alignment.Center),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            MtLogo(size = logoSize, tint = Color.White, gradient = false)
            Spacer(Modifier.height(20.dp))
            Text(
                if (register) "Создайте аккаунт" else "Войдите в аккаунт",
                color = Color.White.copy(alpha = 0.78f),
                fontSize = 14.sp,
            )
            Spacer(Modifier.height(28.dp))

            AuthField(
                value = email,
                onValueChange = { email = it },
                label = "Email",
                placeholder = "you@example.com",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
            )
            if (register) {
                Spacer(Modifier.height(14.dp))
                AuthField(
                    value = name,
                    onValueChange = { name = it },
                    label = "Имя",
                    placeholder = "Как вас зовут",
                )
            }
            Spacer(Modifier.height(14.dp))
            AuthField(
                value = password,
                onValueChange = { password = it },
                label = "Пароль",
                placeholder = "••••••••",
                isPassword = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            )

            if (!state.error.isNullOrBlank()) {
                Spacer(Modifier.height(12.dp))
                Text(
                    state.error!!,
                    color = Color(0xFFFFD9D2),
                    fontSize = 13.sp,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            Spacer(Modifier.height(20.dp))
            AuthSubmit(
                text = if (register) "Зарегистрироваться" else "Войти",
                loading = state.loading,
                onClick = {
                    if (register) vm.register(email, name, password) else vm.login(email, password)
                },
            )

            Spacer(Modifier.height(16.dp))
            Text(
                if (register) "Уже есть аккаунт? Войти" else "Нет аккаунта? Зарегистрироваться",
                color = Color.White,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                textAlign = TextAlign.Center,
                modifier = Modifier
                    .fillMaxWidth()
                    .clickableNoRipple {
                        register = !register
                        vm.clearError()
                    },
            )
        }
    }
}

/**
 * Three large, soft radial blobs of neighbouring brand hues that slowly drift and
 * breathe over the base brand gradient — the "airy" animated login background.
 * Each blob is a radial gradient with a soft transparent falloff (so it reads as a
 * blurred glow without an actual blur pass) and is composited with [BlendMode.Screen]
 * so it only ever lightens the purple, never muddies it.
 */
@Composable
private fun AuthAurora(modifier: Modifier = Modifier) {
    val t = rememberInfiniteTransition(label = "authAurora")
    val p1 by t.animateFloat(
        0f, 1f, infiniteRepeatable(tween(13000, easing = LinearEasing), RepeatMode.Reverse), label = "p1",
    )
    val p2 by t.animateFloat(
        0f, 1f, infiniteRepeatable(tween(17000, easing = LinearEasing), RepeatMode.Reverse), label = "p2",
    )
    val p3 by t.animateFloat(
        0f, 1f, infiniteRepeatable(tween(21000, easing = LinearEasing), RepeatMode.Reverse), label = "p3",
    )
    Canvas(modifier) {
        val w = size.width
        val h = size.height
        fun blob(color: Color, cx: Float, cy: Float, r: Float) {
            drawRect(
                brush = Brush.radialGradient(
                    colors = listOf(color, color.copy(alpha = 0f)),
                    center = Offset(cx, cy),
                    radius = r,
                ),
                blendMode = BlendMode.Screen,
            )
        }
        blob(Color(0xFFA99BFF).copy(alpha = 0.55f), w * (0.15f + 0.25f * p1), h * (0.10f + 0.16f * p1), w * (0.85f + 0.20f * p1))
        blob(Color(0xFF6A55E6).copy(alpha = 0.50f), w * (0.90f - 0.22f * p2), h * (0.88f - 0.16f * p2), w * (0.80f + 0.20f * p2))
        blob(Color(0xFFC3B8FF).copy(alpha = 0.45f), w * (0.55f - 0.18f * p3), h * (0.40f + 0.18f * p3), w * (0.70f + 0.15f * p3))
    }
}

/** Small upward caret drawn above the server popover, pointing at the gear. */
@Composable
private fun PopoverCaret(modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Canvas(modifier.size(width = 18.dp, height = 9.dp)) {
        val w = size.width
        val h = size.height
        val tri = Path().apply {
            moveTo(w / 2f, 0f)
            lineTo(w, h)
            lineTo(0f, h)
            close()
        }
        drawPath(tri, color = c.cardSurface)
        val stroke = 1.dp.toPx()
        drawLine(c.border, Offset(0f, h), Offset(w / 2f, 0f), strokeWidth = stroke)
        drawLine(c.border, Offset(w / 2f, 0f), Offset(w, h), strokeWidth = stroke)
    }
}

/** A frosted single-line field that reads cleanly on the purple gradient. */
@Composable
private fun AuthField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    placeholder: String,
    isPassword: Boolean = false,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
) {
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val border = if (focused) Color.White.copy(alpha = 0.85f) else Color.White.copy(alpha = 0.30f)
    val selectionColors = TextSelectionColors(
        handleColor = Color.White,
        backgroundColor = Color.White.copy(alpha = 0.35f),
    )

    Column(Modifier.fillMaxWidth()) {
        Text(
            label,
            color = Color.White.copy(alpha = 0.92f),
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.padding(bottom = 6.dp),
        )
        CompositionLocalProvider(LocalTextSelectionColors provides selectionColors) {
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = true,
                textStyle = TextStyle(color = Color.White, fontSize = 15.sp),
                cursorBrush = SolidColor(Color.White),
                interactionSource = interaction,
                visualTransformation = if (isPassword) PasswordVisualTransformation() else VisualTransformation.None,
                keyboardOptions = keyboardOptions,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 48.dp)
                    .background(Color.White.copy(alpha = 0.14f), RoundedCornerShape(RadiusMd))
                    .border(1.dp, border, RoundedCornerShape(RadiusMd))
                    .padding(horizontal = 14.dp, vertical = 13.dp),
                decorationBox = { inner ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty() && placeholder.isNotEmpty()) {
                            Text(placeholder, color = Color.White.copy(alpha = 0.55f), fontSize = 15.sp)
                        }
                        inner()
                    }
                },
            )
        }
    }
}

/** The high-contrast white CTA (purple label) — the primary action on purple. */
@Composable
private fun AuthSubmit(text: String, loading: Boolean, onClick: () -> Unit) {
    Box(
        Modifier
            .fillMaxWidth()
            .background(Color.White, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(enabled = !loading, onClick = onClick)
            .heightIn(min = 48.dp)
            .padding(horizontal = 18.dp, vertical = 12.dp),
        contentAlignment = Alignment.Center,
    ) {
        if (loading) {
            CircularProgressIndicator(modifier = Modifier.size(20.dp), strokeWidth = 2.dp, color = BrandPurple)
        } else {
            Text(text, color = BrandPurple, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}
