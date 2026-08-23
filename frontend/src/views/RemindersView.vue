<script setup>
import { ref, onMounted } from 'vue'
import {
  NButton,
  NInput,
  NDatePicker,
  NCheckbox,
  NCard,
  NPopconfirm,
  NIcon,
  useMessage,
} from 'naive-ui'
import { TrashOutline, AlarmOutline } from '@vicons/ionicons5'
import { reminders as remApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import { useFormat } from '@/composables/useFormat'

const { firstDayOfWeek, dateTimePattern: dateTimeFormat, formatDateTime } = useFormat()
const message = useMessage()
const list = ref([])
const newMessage = ref('')
const newAt = ref(null)

async function load() {
  try {
    const res = await remApi.list()
    list.value = res.data || []
  } catch (e) {
    message.error(e.message)
  }
}

async function add() {
  if (!newMessage.value.trim() || !newAt.value) {
    message.warning('Укажите текст и время')
    return
  }
  try {
    await remApi.create({
      message: newMessage.value.trim(),
      remind_at: new Date(newAt.value).toISOString(),
    })
    newMessage.value = ''
    newAt.value = null
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

async function toggle(r) {
  try {
    await remApi.update(r.id, { remind_at: r.remind_at, message: r.message, done: !r.done })
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

async function remove(r) {
  try {
    await remApi.remove(r.id)
    await load()
  } catch (e) {
    message.error(e.message)
  }
}

function fmt(ts) {
  return formatDateTime(ts, { day: '2-digit', month: 'short' })
}
function overdue(r) {
  return !r.done && Date.parse(r.remind_at) < Date.now()
}

onMounted(load)
</script>

<template>
  <div class="reminders">
    <n-card size="small" class="add-card">
      <div class="add-row">
        <n-input v-model:value="newMessage" placeholder="О чём напомнить?" @keyup.enter="add" />
        <n-date-picker
          v-model:value="newAt"
          type="datetime"
          clearable
          :first-day-of-week="firstDayOfWeek"
          :format="dateTimeFormat"
        />
        <n-button type="primary" @click="add">Добавить</n-button>
      </div>
    </n-card>

    <div class="list">
      <div v-for="r in list" :key="r.id" class="rem" :class="{ done: r.done, overdue: overdue(r) }">
        <n-checkbox :checked="r.done" @update:checked="toggle(r)" />
        <div class="rem-body">
          <div class="rem-msg">{{ r.message || 'Напоминание' }}</div>
          <div class="rem-at">
            {{ fmt(r.remind_at) }}<span v-if="overdue(r)"> · просрочено</span>
          </div>
        </div>
        <n-popconfirm
          :positive-button-props="{ type: 'error' }"
          positive-text="Удалить"
          @positive-click="remove(r)"
        >
          <template #trigger>
            <n-button text size="tiny" type="error">
              <n-icon :component="TrashOutline" />
            </n-button>
          </template>
          Удалить напоминание?
        </n-popconfirm>
      </div>
      <empty-state v-if="!list.length" :icon="AlarmOutline" text="Напоминаний пока нет" />
    </div>
  </div>
</template>

<style scoped>
.reminders {
  width: 100%;
}
.add-card {
  margin-bottom: 16px;
}
.add-row {
  display: flex;
  gap: 8px;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.rem {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
}
.rem.overdue {
  border-color: transparent;
  background:
    linear-gradient(var(--t-surface), var(--t-surface)) padding-box,
    linear-gradient(
        to top right,
        color-mix(in srgb, #e0533d 86%, #000),
        #e0533d 50%,
        color-mix(in srgb, #e0533d 86%, #fff)
      )
      border-box;
}
.rem.done .rem-msg {
  text-decoration: line-through;
  opacity: 0.6;
}
.rem-body {
  flex: 1;
  min-width: 0;
}
.rem-msg {
  color: var(--t-text1);
}
.rem-at {
  font-size: 12px;
  color: var(--t-text3);
}
</style>
