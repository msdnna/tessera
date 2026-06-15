<script setup>
import { ref, computed, onMounted } from 'vue'
import { NButton, NTag, NPopconfirm, NIcon, NSpin, NInput, useMessage } from 'naive-ui'
import { ShieldCheckmarkOutline, KeyOutline, SearchOutline } from '@vicons/ionicons5'
import { admin } from '@/api'
import { useAuthStore } from '@/stores/auth'
import UserAvatar from '@/components/UserAvatar.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'

const auth = useAuthStore()
const message = useMessage()

const loading = ref(false)
const users = ref([])
const query = ref('')
const busy = ref('') // id currently mutating, to disable its row buttons

const meId = computed(() => auth.user?.id)
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return users.value
  return users.value.filter(
    (u) => u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
  )
})

async function load() {
  loading.value = true
  try {
    const res = await admin.listUsers()
    users.value = res.data || []
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function toggleActive(u) {
  busy.value = u.id
  try {
    await admin.setActive(u.id, !u.active)
    u.active = !u.active
    message.success(u.active ? 'Аккаунт активирован' : 'Аккаунт деактивирован')
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

async function toggleAdmin(u) {
  busy.value = u.id
  try {
    await admin.setAdmin(u.id, !u.is_admin)
    u.is_admin = !u.is_admin
    message.success(u.is_admin ? 'Назначен администратором' : 'Права администратора сняты')
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

async function copyResetLink(u) {
  busy.value = u.id
  try {
    const res = await admin.resetLink(u.id)
    const link = res.data?.link || ''
    // The backend returns a path-only link when PUBLIC_URL is unset — qualify it
    // against the current origin so the copied value is always clickable.
    const full = link.startsWith('http') ? link : `${window.location.origin}${link}`
    await navigator.clipboard.writeText(full)
    message.success('Ссылка для сброса пароля скопирована')
  } catch (e) {
    message.error(e.message)
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <n-spin :show="loading" :rotate="false">
    <template #icon><TesseraSpinner /></template>
    <div class="admin">
      <div class="head">
        <h2 class="title">
          <n-icon :component="ShieldCheckmarkOutline" class="title-ic" />
          Администрирование
        </h2>
        <span class="sub">Пользователи экземпляра — {{ users.length }}</span>
      </div>

      <n-input v-model:value="query" placeholder="Поиск по имени или почте" clearable class="search">
        <template #prefix><n-icon :component="SearchOutline" /></template>
      </n-input>

      <div class="list">
        <div v-for="u in filtered" :key="u.id" class="urow" :class="{ off: !u.active }">
          <UserAvatar class="ava" :user-id="u.id" :name="u.name" :src="u.avatar_url" />
          <div class="who">
            <div class="line1">
              <span class="uname">{{ u.name }}</span>
              <n-tag v-if="u.is_admin" size="small" type="warning" round>admin</n-tag>
              <n-tag v-if="u.id === meId" size="small" round>вы</n-tag>
              <n-tag v-if="!u.active" size="small" type="error" round>деактивирован</n-tag>
              <n-tag v-if="!u.email_verified" size="small" round :bordered="false">не подтверждён</n-tag>
            </div>
            <div class="umail">{{ u.email }}</div>
          </div>

          <div class="actions">
            <n-button
              size="small"
              quaternary
              :disabled="busy === u.id"
              title="Скопировать ссылку для сброса пароля"
              @click="copyResetLink(u)"
            >
              <template #icon><n-icon :component="KeyOutline" /></template>
            </n-button>

            <!-- Admin toggle: granting is one click; revoking confirms. Never self. -->
            <template v-if="u.id !== meId">
              <n-button
                v-if="!u.is_admin"
                size="small"
                :disabled="busy === u.id"
                @click="toggleAdmin(u)"
              >
                Сделать админом
              </n-button>
              <n-popconfirm v-else @positive-click="toggleAdmin(u)">
                <template #trigger>
                  <n-button size="small" :disabled="busy === u.id">Снять админа</n-button>
                </template>
                Снять права администратора у {{ u.name }}?
              </n-popconfirm>

              <!-- Active toggle: deactivating confirms (blocks login). -->
              <n-button
                v-if="!u.active"
                size="small"
                type="primary"
                ghost
                :disabled="busy === u.id"
                @click="toggleActive(u)"
              >
                Активировать
              </n-button>
              <n-popconfirm v-else @positive-click="toggleActive(u)">
                <template #trigger>
                  <n-button size="small" type="error" ghost :disabled="busy === u.id">
                    Деактивировать
                  </n-button>
                </template>
                Деактивировать {{ u.name }}? Пользователь не сможет войти.
              </n-popconfirm>
            </template>
          </div>
        </div>

        <div v-if="!filtered.length && !loading" class="empty">Никого не найдено</div>
      </div>
    </div>
  </n-spin>
</template>

<style scoped>
.admin {
  max-width: 900px;
  margin: 0 auto;
  padding: 8px 4px 40px;
}
.head {
  margin: 4px 0 16px;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 22px;
  font-weight: 700;
  color: var(--t-text1);
  margin: 0;
}
.title-ic {
  color: var(--t-primary);
  font-size: 22px;
}
.sub {
  font-size: 13px;
  color: var(--t-text3);
}
.search {
  margin-bottom: 14px;
  max-width: 360px;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.urow {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
}
.urow.off {
  opacity: 0.62;
}
.ava {
  width: 36px;
  height: 36px;
  flex: none;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 13px;
  font-weight: 600;
}
.who {
  flex: 1;
  min-width: 0;
}
.line1 {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.uname {
  font-weight: 600;
  color: var(--t-text1);
}
.umail {
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
}
.empty {
  text-align: center;
  color: var(--t-text3);
  margin-top: 40px;
}
@media (max-width: 768px) {
  .urow {
    flex-wrap: wrap;
  }
  .actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
