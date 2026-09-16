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
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.ProjectIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.MembersUiState
import website.msdnna.tessera.ui.viewmodels.MembersViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.MemberBadge
import website.msdnna.tessera.util.MemberLock
import website.msdnna.tessera.util.memberRowBadge
import website.msdnna.tessera.util.memberRowLock

/**
 * Подписи ролей. Это функция, а не `val` уровня файла: карта с готовым текстом
 * вычислилась бы один раз при загрузке класса и застыла бы на языке первого
 * рендера — переключение языка бейджи ролей уже не тронуло бы.
 */
@Composable
private fun roleLabels(): Map<String, String> = mapOf(
    "owner" to stringResource(R.string.members_role_owner),
    "admin" to stringResource(R.string.members_role_admin),
    "member" to stringResource(R.string.members_role_member),
)

/** Workspace members modal (web `MembersModal`): list + invite-by-email + remove. */
@Composable
fun MembersModal(workspaceId: String, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val vm: MembersViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(workspaceId) { if (workspaceId.isNotBlank()) vm.load(workspaceId) }

    val clipboard = LocalClipboardManager.current
    var email by remember { mutableStateOf("") }
    var role by remember { mutableStateOf("member") }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg)).background(c.surface).padding(20.dp),
        ) {
            Text(stringResource(R.string.members_title), color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(14.dp))

            when {
                state.loading -> Box(Modifier.fillMaxWidth().height(120.dp), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                else -> Column(Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
                    state.members.forEach { m ->
                        MemberRow(
                            member = m,
                            meId = state.meId,
                            canManage = state.canManage,
                            onRemove = { vm.remove(m.userId) },
                            onChangeRole = { vm.changeRole(m.userId, it) },
                        )
                    }
                }
            }

            // Everything below writes, and every write here is `requireManager`:
            // the invitation list, its trash icons, and the invite form all answer
            // 403 to a plain member. They get the roster and a line saying why it
            // is all they get — a hidden control is an inconvenience, one that
            // 403s is a bug report.
            if (!state.canManage) {
                if (!state.loading) {
                    Spacer(Modifier.height(10.dp))
                    Text(stringResource(R.string.members_read_only), color = c.text3, fontSize = 12.sp)
                }
            } else {
                MembersManagement(
                    state = state,
                    email = email,
                    onEmailChange = { email = it },
                    role = role,
                    onRoleChange = { role = it },
                    onInvite = {
                        vm.invite(email, role)
                        email = ""
                    },
                    onRevokeInvite = { vm.revokeInvite(it) },
                    onCopyLink = { clipboard.setText(AnnotatedString(state.lastInviteLink)) },
                )
            }

            // Outside the manager half: a failed roster load is the one error a
            // plain member can still hit here.
            TFormError(state.error, modifier = Modifier.padding(top = 6.dp))

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton(stringResource(R.string.common_done), onClick = onDismiss)
            }
        }
    }
}

/**
 * The manager-only half of the dialog: pending invitations and the invite form.
 *
 * Split out rather than guarded field by field so the gate reads as one decision
 * — the same one `requireManager` makes on the server for every route this half
 * calls (#2932).
 */
@Composable
private fun MembersManagement(
    state: MembersUiState,
    email: String,
    onEmailChange: (String) -> Unit,
    role: String,
    onRoleChange: (String) -> Unit,
    onInvite: () -> Unit,
    onRevokeInvite: (String) -> Unit,
    onCopyLink: () -> Unit,
) {
    val c = Tessera.colors
    Column {
        // Pending invitations (by email; invitee may not have an account yet).
        if (state.invitations.isNotEmpty()) {
            Spacer(Modifier.height(14.dp))
            Text(stringResource(R.string.members_invitations), color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
            Spacer(Modifier.height(4.dp))
            state.invitations.forEach { inv ->
                Row(Modifier.fillMaxWidth().padding(vertical = 5.dp), verticalAlignment = Alignment.CenterVertically) {
                    IonIcon(Ion.LINK, size = 16.dp, tint = c.text3)
                    Spacer(Modifier.width(8.dp))
                    Text(inv.email, color = c.text2, fontSize = 13.sp, maxLines = 1, modifier = Modifier.weight(1f))
                    Text(
                        stringResource(if (inv.role == "admin") R.string.members_role_admin else R.string.members_role_member),
                        color = c.text3,
                        fontSize = 11.sp,
                    )
                    Spacer(Modifier.width(6.dp))
                    IonIconButton(Ion.TRASH, onClick = { onRevokeInvite(inv.id) }, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
                }
            }
        }

        Spacer(Modifier.height(16.dp))
        Text(stringResource(R.string.members_invite_title), color = c.text3, fontSize = 12.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.height(6.dp))
        TTextField(
            value = email,
            onValueChange = onEmailChange,
            placeholder = "email@example.com",
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
        )
        Spacer(Modifier.height(8.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            RoleChip(stringResource(R.string.members_role_member), role == "member") { onRoleChange("member") }
            Spacer(Modifier.width(8.dp))
            RoleChip(stringResource(R.string.members_role_admin), role == "admin") { onRoleChange("admin") }
            Spacer(Modifier.weight(1f))
            TButton(
                stringResource(R.string.members_invite_action),
                enabled = email.isNotBlank() && !state.busy,
                loading = state.busy,
                onClick = onInvite,
            )
        }
        Text(
            stringResource(R.string.members_invite_hint),
            color = c.text3,
            fontSize = 11.sp,
            modifier = Modifier.padding(top = 6.dp),
        )
        // The most recent invitation's link, for copying (also emailed when SMTP on).
        if (state.lastInviteLink.isNotBlank()) {
            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    state.lastInviteLink,
                    color = c.text2,
                    fontSize = 12.sp,
                    maxLines = 1,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                TButton(stringResource(R.string.members_copy_link), kind = TButtonKind.Secondary, onClick = onCopyLink)
            }
        }
    }
}

@Composable
private fun MemberRow(
    member: Member,
    meId: String,
    canManage: Boolean,
    onRemove: () -> Unit,
    onChangeRole: (String) -> Unit,
) {
    val c = Tessera.colors
    var confirm by remember { mutableStateOf(false) }
    var roleMenu by remember { mutableStateOf(false) }
    val roles = roleLabels()
    val lock = memberRowLock(member, meId, canManage)
    val badge = when (memberRowBadge(member, meId)) {
        MemberBadge.OWNER -> roles.getValue("owner")
        MemberBadge.YOU -> stringResource(R.string.members_you)
        MemberBadge.ROLE -> roles[member.role] ?: member.role
    }
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
        // A locked row is a plain badge: the workspace owner's role is immutable,
        // the viewer's own row would be a self-demotion, and for someone who
        // passes no `requireManager` at all every row is locked alike.
        if (lock != MemberLock.NONE) {
            Box(
                Modifier.clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt).padding(horizontal = 8.dp, vertical = 3.dp),
            ) {
                Text(badge, color = c.text2, fontSize = 11.sp, fontWeight = FontWeight.Medium)
            }
        } else {
            // Tap the role badge to switch member ↔ admin.
            Box {
                Row(
                    Modifier.clip(RoundedCornerShape(RadiusSm)).background(c.surfaceAlt)
                        .clickableNoRipple { roleMenu = true }.padding(horizontal = 8.dp, vertical = 3.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(badge, color = c.text2, fontSize = 11.sp, fontWeight = FontWeight.Medium)
                    Spacer(Modifier.width(3.dp))
                    IonIcon(Ion.CHEVRON_DOWN, size = 12.dp, tint = c.text3)
                }
                TDropdown(expanded = roleMenu, onDismiss = { roleMenu = false }) {
                    TMenuItem(roles.getValue("member"), onClick = {
                        roleMenu = false
                        if (member.role != "member") onChangeRole("member")
                    })
                    TMenuItem(roles.getValue("admin"), onClick = {
                        roleMenu = false
                        if (member.role != "admin") onChangeRole("admin")
                    })
                }
            }
        }
        if (lock == MemberLock.NONE) {
            Spacer(Modifier.width(6.dp))
            Box {
                IonIconButton(Ion.TRASH, onClick = { confirm = true }, boxSize = 30.dp, iconSize = 16.dp, tint = c.text3)
                TConfirmPopover(
                    expanded = confirm,
                    message = stringResource(R.string.members_remove_confirm),
                    confirmText = stringResource(R.string.members_remove),
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
