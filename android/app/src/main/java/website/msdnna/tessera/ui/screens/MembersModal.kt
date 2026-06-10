package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
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
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.ProjectIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.MembersViewModel
import website.msdnna.tessera.util.Ion

private val RoleLabels = mapOf("owner" to "Владелец", "admin" to "Админ", "member" to "Участник")

/** Workspace members modal (web `MembersModal`): list + invite-by-email + remove. */
@Composable
fun MembersModal(workspaceId: String, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val vm: MembersViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(workspaceId) { if (workspaceId.isNotBlank()) vm.load(workspaceId) }

    var email by remember { mutableStateOf("") }
    var role by remember { mutableStateOf("member") }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(20.dp),
        ) {
            Text("Участники", color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(14.dp))

            when {
                state.loading -> Box(Modifier.fillMaxWidth().height(120.dp), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                else -> Column(Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
                    state.members.forEach { m ->
                        MemberRow(m, onRemove = { vm.remove(m.userId) })
                    }
                }
            }

            Spacer(Modifier.height(16.dp))
            Text("Добавить участника", color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
            Spacer(Modifier.height(6.dp))
            TTextField(
                value = email,
                onValueChange = { email = it },
                placeholder = "email@example.com",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
            )
            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                RoleChip("Участник", role == "member") { role = "member" }
                Spacer(Modifier.width(8.dp))
                RoleChip("Админ", role == "admin") { role = "admin" }
                Spacer(Modifier.weight(1f))
                TButton(
                    "Добавить",
                    enabled = email.isNotBlank() && !state.busy,
                    loading = state.busy,
                    onClick = {
                        vm.invite(email, role)
                        email = ""
                    },
                )
            }
            Text(
                "Пользователь должен быть уже зарегистрирован.",
                color = c.text3,
                fontSize = 11.sp,
                modifier = Modifier.padding(top = 6.dp),
            )
            TFormError(state.error, modifier = Modifier.padding(top = 6.dp))

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton("Готово", onClick = onDismiss)
            }
        }
    }
}

@Composable
private fun MemberRow(member: Member, onRemove: () -> Unit) {
    val c = Tessera.colors
    var confirm by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().padding(vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ProjectIcon(name = member.name.ifBlank { member.email }.ifBlank { "?" }, icon = "", color = "", size = 32.dp)
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Text(member.name.ifBlank { "—" }, color = c.text1, fontSize = 14.sp, maxLines = 1)
            if (member.email.isNotBlank()) Text(member.email, color = c.text3, fontSize = 12.sp, maxLines = 1)
        }
        Box(
            Modifier.clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt).padding(horizontal = 8.dp, vertical = 3.dp),
        ) {
            Text(RoleLabels[member.role] ?: member.role, color = c.text2, fontSize = 11.sp, fontWeight = FontWeight.Medium)
        }
        if (member.role != "owner") {
            Spacer(Modifier.width(6.dp))
            Box {
                IonIconButton(Ion.TRASH, onClick = { confirm = true }, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
                TConfirmPopover(
                    expanded = confirm,
                    message = "Убрать участника?",
                    confirmText = "Убрать",
                    onConfirm = {
                        confirm = false
                        onRemove()
                    },
                    onDismiss = { confirm = false },
                )
            }
        }
    }
}

@Composable
private fun RoleChip(label: String, active: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm))
            .background(if (active) accentGradient(c.primary) else SolidColor(c.surfaceAlt))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 7.dp),
    ) {
        Text(label, color = if (active) c.onPrimary else c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
    }
}
