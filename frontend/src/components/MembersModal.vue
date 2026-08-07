<script setup>
import { ref, watch } from 'vue'
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
import { copyText } from '@/utils/clipboard'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:show'])

const message = useMessage()
const members = ref([])
const invitations = ref([])
const email = ref('')
const role = ref('member')
const lastLink = ref('') // link from the most recent invitation, for copying
const roleOptions = [
  { label: 'Участник', value: 'member' },
  { label: 'Админ', value: 'admin' },
]

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
  try {
    await wsApi.addMember(props.wsId, { email: e, role: role.value })
    email.value = ''
    lastLink.value = ''
    await load()
    message.success('Участник добавлен')
  } catch (err) {
    if (err.message === 'no user with that email') {
      try {
        const res = await wsApi.createInvitation(props.wsId, { email: e, role: role.value })
        lastLink.value = res.data.link || ''
        email.value = ''
        await load()
        message.success('Приглашение создано — скопируйте ссылку')
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
    message.success('Роль обновлена')
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
  if (await copyText(lastLink.value)) message.success('Ссылка скопирована')
  else message.info(lastLink.value)
}

watch(
  () => [props.show, props.wsId],
  ([show]) => show && load(),
)
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card title="Участники пространства" style="width: 460px; max-width: 92vw" role="dialog">
      <div class="members">
        <div v-for="m in members" :key="m.user_id" class="member">
          <div class="info">
            <span class="name">{{ m.name }}</span>
            <span class="mail">{{ m.email }}</span>
          </div>
          <n-tag v-if="m.role === 'owner'" size="small" :bordered="false">Владелец</n-tag>
          <n-select
            v-else
            :value="m.role"
            :options="roleOptions"
            size="small"
            style="width: 120px"
            @update:value="(v) => changeRole(m.user_id, v)"
          />
          <n-popconfirm
            v-if="m.role !== 'owner'"
            :positive-button-props="{ type: 'error' }"
            positive-text="Удалить"
            @positive-click="remove(m.user_id)"
          >
            <template #trigger>
              <n-button text size="tiny" type="error"
                ><n-icon :component="TrashOutline"
              /></n-button>
            </template>
            Удалить участника из пространства?
          </n-popconfirm>
        </div>
      </div>

      <!-- Pending invitations -->
      <div v-if="invitations.length" class="invites">
        <n-text depth="3" class="lbl">Приглашения</n-text>
        <div v-for="inv in invitations" :key="inv.id" class="invite-row">
          <n-icon :component="MailOutline" class="inv-icon" />
          <span class="inv-mail">{{ inv.email }}</span>
          <n-tag size="small" :bordered="false">{{
            inv.role === 'admin' ? 'Админ' : 'Участник'
          }}</n-tag>
          <n-button text size="tiny" type="error" @click="revokeInvite(inv.id)">
            <n-icon :component="TrashOutline" />
          </n-button>
        </div>
      </div>

      <div class="invite">
        <n-text depth="3" class="lbl">Пригласить по email</n-text>
        <n-space>
          <n-input
            v-model:value="email"
            size="small"
            placeholder="user@example.com"
            @keyup.enter="invite"
          />
          <n-select v-model:value="role" :options="roleOptions" size="small" style="width: 130px" />
          <n-button type="primary" size="small" @click="invite">Пригласить</n-button>
        </n-space>
        <n-text depth="3" class="hint">
          Зарегистрированного — добавим сразу; нового — создадим приглашение по ссылке.
        </n-text>
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
