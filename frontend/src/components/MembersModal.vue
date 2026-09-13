<script setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NModal,
  NCard,
  NInput,
  NSelect,
  NButton,
  NSpace,
  NText,
  NTag,
  NPopconfirm,
  NIcon,
  useMessage,
} from 'naive-ui'
import { TrashOutline, CopyOutline, MailOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useFormat } from '@/composables/useFormat'
import { copyText } from '@/utils/clipboard'
import { LOCK_OWNER, LOCK_SELF, memberRowLock, addMemberRefusal } from '@/utils/members'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:show'])

const { t } = useI18n()
const { formatDate } = useFormat()
const message = useMessage()
const auth = useAuthStore()
const members = ref([])
const invitations = ref([])
const email = ref('')
const inviteError = ref('')
const role = ref('member')

// Who is looking. The dialog knew the owner (the list carries the role) but not
// the viewer, so an admin's own row offered the same role select and the same
// trash icon as anybody else's — see utils/members.js. `ownerId` is left to the
// row's own `role` field; there is no workspace record in scope here.
const who = computed(() => ({
  selfId: auth.user?.id || '',
  selfEmail: auth.user?.email || '',
}))
const rowLock = (m) => memberRowLock(m, who.value)
const lastLink = ref('') // link from the most recent invitation, for copying
// Computed: the role labels sit inside a select, and a frozen table would keep
// showing the language of the first render — including inside the open dropdown.
const roleOptions = computed(() => [
  { label: t('shell.members.member'), value: 'member' },
  { label: t('shell.members.admin'), value: 'admin' },
])

// An invitation lives a week (backend invitationTTL), but the list endpoint only
// drops the accepted ones — an expired invitation stays in the response forever.
// Every path that redeems one refuses it past `expires_at`, so an unmarked row
// tells the admin "invited, hasn't accepted yet" about a link that answers
// "invalid or expired invitation"; there is no unique key on (workspace, email)
// either, so re-invites pile up and only the newest of the identical-looking
// rows works. The date is what tells them apart.
const invitationRows = computed(() =>
  invitations.value.map((inv) => {
    const until = inv.expires_at ? new Date(inv.expires_at) : null
    const expired = !!until && until.getTime() <= Date.now()
    return { ...inv, expired, until: until && !expired ? formatDate(until) : '' }
  }),
)

async function load() {
  if (!props.wsId) return
  try {
    const [m, inv] = await Promise.all([wsApi.members(props.wsId), wsApi.invitations(props.wsId)])
    members.value = m.data || []
    invitations.value = inv.data || []
  } catch (e) {
    message.error(e.message)
  }
}

// One input: add an already-registered user instantly; otherwise fall back to an
// email invitation (link shared manually / by email when SMTP is on).
async function invite() {
  const e = email.value.trim()
  if (!e) return
  // The same operation by the other door: CreateMembership is an upsert, so an
  // address already on the list is a role change, and the caller's own address
  // with the select on its "member" default was a self-demotion answered with a
  // green "Участник добавлен". The server refuses it now; this only spares the
  // round trip and says which rule was hit.
  const refused = addMemberRefusal(e, members.value, who.value)
  if (refused) {
    inviteError.value =
      refused === LOCK_OWNER ? t('shell.members.cannotOwner') : t('shell.members.cannotSelf')
    return
  }
  inviteError.value = ''
  try {
    await wsApi.addMember(props.wsId, { email: e, role: role.value })
    email.value = ''
    lastLink.value = ''
    await load()
    message.success(t('shell.members.added'))
  } catch (err) {
    // `raw`, not `message`: the branch is on the server's English sentinel. The
    // humanized line is localized and would stop matching the moment that
    // sentinel got a catalogue entry — which it now has (#2932).
    if (err.raw === 'no user with that email') {
      try {
        const res = await wsApi.createInvitation(props.wsId, { email: e, role: role.value })
        lastLink.value = res.data.link || ''
        email.value = ''
        await load()
        message.success(t('shell.members.inviteCreated'))
      } catch (err2) {
        message.error(err2.message)
      }
    } else {
      message.error(err.message)
    }
  }
}

async function remove(userId) {
  try {
    await wsApi.removeMember(props.wsId, userId)
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

async function changeRole(userId, newRole) {
  try {
    await wsApi.updateMemberRole(props.wsId, userId, newRole)
    await load()
    message.success(t('shell.members.roleUpdated'))
  } catch (e) {
    message.error(e.message)
    await load() // revert the select to the server's state
  }
}

async function revokeInvite(invId) {
  try {
    await wsApi.deleteInvitation(props.wsId, invId)
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

async function copyLink() {
  if (await copyText(lastLink.value)) message.success(t('shell.members.linkCopied'))
  else message.info(lastLink.value)
}

watch(
  () => [props.show, props.wsId],
  ([show]) => show && load(),
)
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card :title="t('shell.members.title')" style="width: 460px; max-width: 92vw" role="dialog">
      <div class="members">
        <div v-for="m in members" :key="m.user_id" class="member">
          <div class="info">
            <span class="name">{{ m.name }}</span>
            <span class="mail">{{ m.email }}</span>
          </div>
          <n-tag v-if="rowLock(m) === LOCK_OWNER" size="small" :bordered="false">
            {{ t('shell.members.owner') }}
          </n-tag>
          <!-- A control that vanishes with no word reads as a broken dialog, so
               the locked row says which rule locked it. -->
          <n-tag v-else-if="rowLock(m) === LOCK_SELF" size="small" :bordered="false">
            {{ t('shell.members.you') }}
          </n-tag>
          <n-select
            v-else
            :value="m.role"
            :options="roleOptions"
            size="small"
            style="width: 120px"
            @update:value="(v) => changeRole(m.user_id, v)"
          />
          <n-popconfirm
            v-if="!rowLock(m)"
            :positive-button-props="{ type: 'error' }"
            :positive-text="t('common.action.delete')"
            @positive-click="remove(m.user_id)"
          >
            <template #trigger>
              <n-button text size="tiny" type="error"
                ><n-icon :component="TrashOutline"
              /></n-button>
            </template>
            {{ t('shell.members.removeConfirm') }}
          </n-popconfirm>
        </div>
      </div>

      <!-- Pending invitations -->
      <div v-if="invitations.length" class="invites">
        <n-text depth="3" class="lbl">{{ t('shell.members.invitations') }}</n-text>
        <div
          v-for="inv in invitationRows"
          :key="inv.id"
          class="invite-row"
          :class="{ 'is-expired': inv.expired }"
        >
          <n-icon :component="MailOutline" class="inv-icon" />
          <span class="inv-mail">{{ inv.email }}</span>
          <n-tag size="small" :bordered="false">{{
            inv.role === 'admin' ? t('shell.members.admin') : t('shell.members.member')
          }}</n-tag>
          <!-- Revoke stays on an expired row: clearing it out of the list is the
               only thing left to do with it. -->
          <n-tag v-if="inv.expired" class="inv-expired" size="small" type="error" :bordered="false">
            {{ t('shell.members.inviteExpired') }}
          </n-tag>
          <n-text v-else-if="inv.until" depth="3" class="inv-until">
            {{ t('shell.members.inviteUntil', { date: inv.until }) }}
          </n-text>
          <n-button text size="tiny" type="error" @click="revokeInvite(inv.id)">
            <n-icon :component="TrashOutline" />
          </n-button>
        </div>
      </div>

      <div class="invite">
        <n-text depth="3" class="lbl">{{ t('shell.members.inviteLabel') }}</n-text>
        <n-space>
          <n-input
            v-model:value="email"
            size="small"
            :placeholder="t('shell.members.emailPlaceholder')"
            @update:value="inviteError = ''"
            @keyup.enter="invite"
          />
          <n-select v-model:value="role" :options="roleOptions" size="small" style="width: 130px" />
          <n-button type="primary" size="small" @click="invite">
            {{ t('shell.members.invite') }}
          </n-button>
        </n-space>
        <n-text v-if="inviteError" type="error" class="invite-err">{{ inviteError }}</n-text>
        <n-text depth="3" class="hint">{{ t('shell.members.inviteHint') }}</n-text>
        <div v-if="lastLink" class="link-box">
          <n-input :value="lastLink" size="small" readonly />
          <n-button size="small" @click="copyLink">
            <template #icon><n-icon :component="CopyOutline" /></template>
          </n-button>
        </div>
      </div>
    </n-card>
  </n-modal>
</template>

<style scoped>
.members {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 16px;
  max-height: 240px;
  overflow-y: auto;
}
.member {
  display: flex;
  align-items: center;
  gap: 10px;
}
.info {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.name {
  color: var(--t-text1);
}
.mail {
  font-size: 12px;
  color: var(--t-text3);
}
.invites {
  margin-bottom: 16px;
}
.invite-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}
.inv-icon {
  color: var(--t-text3);
}
.inv-mail {
  flex: 1;
  font-size: 13px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
}
.inv-until {
  font-size: 11px;
  white-space: nowrap;
}
/* A dead row still belongs in the list (it is the only place to revoke it), it
   just must not read as one that is still waiting for an answer. */
.invite-row.is-expired .inv-mail,
.invite-row.is-expired .inv-icon {
  opacity: 0.6;
}
.invite-err {
  display: block;
  font-size: 11px;
  margin-top: 6px;
}
.lbl {
  display: block;
  font-size: 12px;
  margin-bottom: 6px;
}
.hint {
  display: block;
  font-size: 11px;
  margin-top: 6px;
}
.link-box {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}
</style>
