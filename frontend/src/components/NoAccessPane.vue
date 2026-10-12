<script setup>
// «Нет доступа · Запросить доступ» (#3077, CatalogE «Нет доступа»): what a link
// to a board or a workspace shows someone who is not a member of it. One button
// asks the workspace's owner and admins to let them in; the answer comes as a
// notification.
//
// Deliberately says less than the mockup: the server answers a non-member with
// nothing about the space — not its name, not its owner — and this screen does
// not guess. Whoever lets them in is the one who names it.
//
// `target` is what the request names: { workspace_id } for the space's own link,
// { project_slug, board_slug } or { board_id } for a board's.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import { LockClosedOutline, CheckmarkCircleOutline } from '@vicons/ionicons5'
import { accessRequests } from '@/api'
import { humanizeError } from '@/utils/errors'
import { isTypingTarget } from '@/utils/keys'
import { CAPS } from '@/utils/keyCaps'
import Kbd from './Kbd.vue'
import './kit/kit.css'

const props = defineProps({
  target: { type: Object, required: true },
})
const { t } = useI18n()
const router = useRouter()

const state = ref('idle') // idle | busy | sent | error
const error = ref('')
const message = ref('')

async function request() {
  if (state.value === 'busy' || state.value === 'sent') return
  state.value = 'busy'
  error.value = ''
  try {
    await accessRequests.create({ ...props.target, message: message.value.trim() })
    state.value = 'sent'
  } catch (e) {
    // Already a member (a stale link of one's own space): just go in.
    if (e.status === 409) {
      router.replace('/')
      return
    }
    error.value = humanizeError(e)
    state.value = 'error'
  }
}
function back() {
  if (window.history.length > 1) router.back()
  else router.push('/')
}
function onKey(e) {
  if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return
  if (isTypingTarget(e.target) && e.key !== 'Enter') return
  if (e.key === 'Enter') {
    e.preventDefault()
    request()
  } else if (e.key === 'Escape' && !isTypingTarget(e.target)) {
    e.preventDefault()
    back()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="na" data-testid="no-access">
    <span class="na-tile">
      <n-icon
        :component="state === 'sent' ? CheckmarkCircleOutline : LockClosedOutline"
        :size="26"
      />
    </span>
    <h2 class="na-title">
      {{ state === 'sent' ? t('shell.noAccess.sentTitle') : t('shell.noAccess.title') }}
    </h2>
    <p class="na-text">
      {{ state === 'sent' ? t('shell.noAccess.sentText') : t('shell.noAccess.text') }}
    </p>
    <template v-if="state !== 'sent'">
      <input
        v-model="message"
        class="na-msg kf-input"
        maxlength="500"
        :placeholder="t('shell.noAccess.messagePlaceholder')"
        data-testid="no-access-message"
      />
      <p v-if="error" class="na-err" data-testid="no-access-error">{{ error }}</p>
    </template>
    <div class="na-actions">
      <button
        v-if="state !== 'sent'"
        type="button"
        class="kit-btn primary"
        :disabled="state === 'busy'"
        data-testid="no-access-request"
        @click="request"
      >
        {{ t('shell.noAccess.request') }}<Kbd>{{ CAPS.enter }}</Kbd>
      </button>
      <button type="button" class="kit-btn" data-testid="no-access-back" @click="back">
        {{ t('shell.notFound.back') }}<Kbd>{{ CAPS.escape }}</Kbd>
      </button>
    </div>
  </div>
</template>

<style scoped>
.na {
  height: 100%;
  min-height: 340px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 0 40px;
  text-align: center;
}
.na-tile {
  width: 56px;
  height: 56px;
  border-radius: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #f0a020;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 16%, var(--t-surface)),
    color-mix(in srgb, #f0a020 28%, var(--t-surface))
  );
}
.na-title {
  margin: 4px 0 0;
  font-size: 17px;
  font-weight: 700;
  color: var(--t-text1);
}
.na-text {
  margin: 0;
  max-width: 340px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
  text-wrap: pretty;
}
.na-msg {
  width: 320px;
  max-width: 100%;
  margin-top: 6px;
}
.na-err {
  margin: 0;
  font-size: 12.5px;
  color: var(--t-error);
}
.na-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
  justify-content: center;
}
</style>
