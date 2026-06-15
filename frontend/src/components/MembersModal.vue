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
import { TrashOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:show'])

const message = useMessage()
const members = ref([])
const email = ref('')
const role = ref('member')
const roleOptions = [
  { label: 'Участник', value: 'member' },
  { label: 'Админ', value: 'admin' },
]

async function load() {
  if (!props.wsId) return
  try {
    members.value = (await wsApi.members(props.wsId)).data || []
  } catch (e) {
    message.error(e.message)
  }
}

async function invite() {
  const e = email.value.trim()
  if (!e) return
  try {
    await wsApi.addMember(props.wsId, { email: e, role: role.value })
    email.value = ''
    await load()
    message.success('Участник добавлен')
  } catch (err) {
    message.error(err.message)
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
          <n-button type="primary" size="small" @click="invite">Добавить</n-button>
        </n-space>
        <n-text depth="3" class="hint">Пользователь должен быть уже зарегистрирован.</n-text>
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
  max-height: 280px;
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
</style>
