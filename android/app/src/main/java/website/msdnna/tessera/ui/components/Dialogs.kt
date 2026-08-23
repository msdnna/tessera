package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import website.msdnna.tessera.data.model.EstimationConfig
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.Estimation

/** Themed modal shell — a centred surface card used by the dialogs below. */
@Composable
private fun DialogShell(onDismiss: () -> Unit, content: @Composable () -> Unit) {
    val c = Tessera.colors
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier
                .popupAppear(TransformOrigin.Center)
                .fillMaxWidth()
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface)
                .padding(20.dp),
        ) { content() }
    }
}

/** Single-line text-input dialog (create / rename). */
@Composable
fun TInputDialog(
    title: String,
    initial: String = "",
    confirmText: String = "Сохранить",
    placeholder: String = "",
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    var text by remember { mutableStateOf(initial) }
    DialogShell(onDismiss) {
        Text(title, color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(14.dp))
        TTextField(value = text, onValueChange = { text = it }, placeholder = placeholder)
        Spacer(Modifier.height(18.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            Spacer(Modifier.width(8.dp))
            TButton(
                confirmText,
                enabled = text.isNotBlank(),
                onClick = { if (text.isNotBlank()) onConfirm(text.trim()) },
            )
        }
    }
}

/**
 * Type-to-confirm dialog for high-risk deletes: the user must type [name]
 * exactly before the destructive button enables (GitHub-style). Used for
 * project / workspace deletion.
 */
@Composable
fun TConfirmByNameDialog(
    title: String,
    message: String,
    name: String,
    confirmText: String = "Удалить",
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    var typed by remember { mutableStateOf("") }
    val matches = name.isNotBlank() && typed.trim() == name.trim()
    DialogShell(onDismiss) {
        Text(title, color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(10.dp))
        Text(message, color = c.text2, fontSize = 14.sp)
        Spacer(Modifier.height(10.dp))
        Text("Введите «$name» для подтверждения:", color = c.text3, fontSize = 13.sp)
        Spacer(Modifier.height(6.dp))
        TTextField(value = typed, onValueChange = { typed = it }, placeholder = name)
        Spacer(Modifier.height(18.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            Spacer(Modifier.width(8.dp))
            TButton(confirmText, enabled = matches, onClick = { if (matches) onConfirm() })
        }
    }
}

/** Confirm / cancel dialog (destructive actions). */
@Composable
fun TConfirmDialog(
    title: String,
    message: String,
    confirmText: String = "Удалить",
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    DialogShell(onDismiss) {
        Text(title, color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(10.dp))
        Text(message, color = c.text2, fontSize = 14.sp)
        Spacer(Modifier.height(18.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            Spacer(Modifier.width(8.dp))
            TButton(confirmText, onClick = onConfirm)
        }
    }
}

/**
 * Editor for the two-level task-estimation config. [scope] is "workspace" (the
 * default every project inherits) or "project" (an override). The "Наследовать"
 * toggle stores null. Mirrors the web `EstimationModal`.
 */
@Composable
fun EstimationDialog(
    scope: String,
    name: String,
    current: EstimationConfig?,
    inherited: EstimationConfig,
    onSave: (EstimationConfig?) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    var inherit by remember { mutableStateOf(current == null) }
    var unit by remember { mutableStateOf(current?.unit?.ifEmpty { "time" } ?: "time") }
    var hours by remember { mutableStateOf(((current?.hoursPerDay ?: 8.0).toInt()).toString()) }
    var days by remember { mutableStateOf(((current?.daysPerWeek ?: 5.0).toInt()).toString()) }
    var scale by remember { mutableStateOf(current?.pointsScale ?: "fibonacci") }
    var label by remember { mutableStateOf(current?.customLabel.orEmpty()) }

    val title = if (scope == "workspace") "Оценка задач — по умолчанию" else "Оценка задач — $name"
    val res = LocalResources.current
    val inheritLabel = if (inherited.unit == "time") {
        "${Estimation.unitName(res, inherited)} · ${(inherited.hoursPerDay ?: 8.0).toInt()}ч/день · ${(inherited.daysPerWeek ?: 5.0).toInt()}дн/неделя"
    } else {
        Estimation.unitName(res, inherited)
    }

    fun build(): EstimationConfig? = if (inherit) {
        null
    } else when (unit) {
        "points" -> EstimationConfig(unit = "points", pointsScale = scale)

        "custom" -> EstimationConfig(unit = "custom", customLabel = label.trim())

        else -> EstimationConfig(
            unit = "time",
            hoursPerDay = (hours.toDoubleOrNull() ?: 8.0).coerceIn(1.0, 24.0),
            daysPerWeek = (days.toDoubleOrNull() ?: 5.0).coerceIn(1.0, 7.0),
        )
    }

    DialogShell(onDismiss) {
        Text(title, color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(14.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            TSwitch(checked = inherit, onCheckedChange = { inherit = it })
            Spacer(Modifier.width(10.dp))
            Text("Наследовать ($inheritLabel)", color = c.text2, fontSize = 13.sp)
        }
        if (!inherit) {
            Spacer(Modifier.height(14.dp))
            SegRow(
                options = listOf("time" to "Время", "points" to "Поинты", "custom" to "Свои"),
                selected = unit,
                onSelect = { unit = it },
            )
            Spacer(Modifier.height(12.dp))
            when (unit) {
                "time" -> {
                    TTextField(value = hours, onValueChange = { hours = it.filter(Char::isDigit) }, label = "Часов в рабочем дне")
                    Spacer(Modifier.height(10.dp))
                    TTextField(value = days, onValueChange = { days = it.filter(Char::isDigit) }, label = "Дней в рабочей неделе")
                }

                "points" -> SegRow(
                    options = listOf("fibonacci" to "Фибоначчи", "tshirt" to "Футболки", "linear" to "Линейная"),
                    selected = scale,
                    onSelect = { scale = it },
                )

                else -> TTextField(value = label, onValueChange = { label = it }, label = "Название единицы", placeholder = "напр. у.е.")
            }
        }
        Spacer(Modifier.height(18.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TButton("Отмена", kind = TButtonKind.Ghost, onClick = onDismiss)
            Spacer(Modifier.width(8.dp))
            TButton("Сохранить", onClick = {
                onSave(build())
                onDismiss()
            })
        }
    }
}

/** Compact segmented selector: equal-width labelled pills, the active one filled. */
@Composable
private fun SegRow(options: List<Pair<String, String>>, selected: String, onSelect: (String) -> Unit) {
    val c = Tessera.colors
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        options.forEach { (key, lbl) ->
            val active = key == selected
            Row(
                Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(RadiusSm))
                    .then(if (active) Modifier.background(c.surfaceAlt) else Modifier.border(1.dp, c.border, RoundedCornerShape(RadiusSm)))
                    .clickableNoRipple { onSelect(key) }
                    .padding(vertical = 8.dp),
                horizontalArrangement = Arrangement.Center,
            ) {
                Text(lbl, color = if (active) c.primary else c.text2, fontSize = 13.sp, fontWeight = if (active) FontWeight.SemiBold else FontWeight.Normal)
            }
        }
    }
}
