<script setup>
import { ref } from 'vue'
import { NIcon } from 'naive-ui'
import {
  AddCircleOutline,
  CheckmarkCircleOutline,
  ArrowForwardCircleOutline,
  CreateOutline,
  OpenOutline,
  LinkOutline,
  CloseOutline,
} from '@vicons/ionicons5'
import { copyText } from '@/utils/clipboard'
import UserAvatar from './UserAvatar.vue'

// A transient, bottom-left stack of board-activity toasts: who
// did what on the board you're looking at, with quick Open / Copy-link actions.
// Deliberately NOT the bell notification centre — this only mirrors live board
// activity and never persists. KanbanBoard feeds it via the exposed push().
const emit = defineEmits(['open'])

const DISMISS_MS = 6500
const CAP = 3
let seq = 0

const toasts = ref([])

const VERBS = {
  created: { text: 'создал(а) задачу', icon: AddCircleOutline, color: '#7c5cff' },
  completed: { text: 'завершил(а) задачу', icon: CheckmarkCircleOutline, color: '#18a058' },
  reopened: { text: 'вернул(а) в работу', icon: ArrowForwardCircleOutline, color: '#e0922f' },
  moved: { text: 'переместил(а) задачу', icon: ArrowForwardCircleOutline, color: '#2f80ed' },
  updated: { text: 'изменил(а) задачу', icon: CreateOutline, color: '#2f80ed' },
}

// activity: { id, number, title, verb, actorId, actorName, avatar, self }
function push(activity) {
  const meta = VERBS[activity.verb] || VERBS.updated
  const id = ++seq
  const entry = { ...activity, key: id, meta, copied: false }
  toasts.value.push(entry)
  if (toasts.value.length > CAP) toasts.value.splice(0, toasts.value.length - CAP)
  entry.timer = setTimeout(() => dismiss(id), DISMISS_MS)
}
function dismiss(key) {
  const i = toasts.value.findIndex((t) => t.key === key)
  if (i < 0) return
  clearTimeout(toasts.value[i].timer)
  toasts.value.splice(i, 1)
}
function open(t) {
  dismiss(t.key)
  emit('open', t.id)
}
async function copyLink(t) {
  const num = t.number ?? t.id
  const url = `${location.origin}${location.pathname}?task=${num}`
  await copyText(url)
  t.copied = true
  setTimeout(() => (t.copied = false), 1600)
}

defineExpose({ push })
</script>

<template>
  <div class="activity-stack">
    <transition-group name="toast">
      <div v-for="t in toasts" :key="t.key" class="toast">
        <div class="t-ava">
          <UserAvatar
            v-if="t.actorId || t.avatar"
            class="ava"
            :user-id="t.actorId"
            :src="t.avatar"
            :name="t.actorName"
          />
          <n-icon v-else :component="t.meta.icon" :size="22" :style="{ color: t.meta.color }" />
        </div>
        <div class="t-body">
          <div class="t-head">
            <n-icon :component="t.meta.icon" :size="14" :style="{ color: t.meta.color }" />
            <span class="t-who">{{ t.self ? 'Вы' : t.actorName || 'Кто-то' }}</span>
            <span class="t-verb">{{ t.meta.text }}</span>
          </div>
          <div class="t-title">{{ t.title }}</div>
          <div class="t-actions">
            <button class="t-btn primary" @click="open(t)">
              <n-icon :component="OpenOutline" :size="13" />Открыть
            </button>
            <button class="t-btn" @click="copyLink(t)">
              <n-icon :component="LinkOutline" :size="13" />{{
                t.copied ? 'Скопировано' : 'Ссылка'
              }}
            </button>
          </div>
        </div>
        <button class="t-close" title="Скрыть" @click="dismiss(t.key)">
          <n-icon :component="CloseOutline" :size="15" />
        </button>
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.activity-stack {
  position: fixed;
  left: 16px;
  bottom: 16px;
  /* НЕ выбирать значение в диапазоне 2000-4500: Naive раздаёт z-index оверлеям
     счётчиком от 2000 с инкрементом на каждый монтируемый поповер/дропдаун/модалку
     (vdirs/z-index-manager), так что «вроде выше модалки» перестаёт быть правдой
     после сотни оверлеев за сессию. Выше полосы Naive, но ниже блокирующих
     оверлеев приложения (лоадер 9000, оверлей связи 9001-9002). */
  z-index: 8000;
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: none;
  /* Один постоянный слой композитора на ВСЮ стопку, а не по слою на тост.
     Стопка смонтирована всё время жизни доски, поэтому слой создаётся один раз и
     не пересобирается. Промотировать сами тосты нельзя (см. комментарий у .toast):
     три перекрывающихся промотированных соседа Chromium сливает в один squashing
     layer и пересобирает его на каждую смену состава — это и давало мерцание. */
  will-change: transform;
}
.toast {
  pointer-events: auto;
  position: relative;
  display: flex;
  gap: 10px;
  width: 320px;
  max-width: calc(100vw - 32px);
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.16);
  /* НЕ вешать сюда will-change / translateZ. Пробовали (задача #2695, первый заход):
     тосты перекрываются, и Chromium сливает их промотированные слои в один
     squashing layer, который пересобирается на каждый вход/уход тоста. Кадр
     презентуется до того, как дорастеризованы все тайлы — на видео из задачи это
     ровно один кадр (f523 при 30 fps), где у ВСЕХ трёх тостов пропадает содержимое
     левее общей границы тайла x≈260, а сквозь них виден сайдбар. Это и есть
     «лёгкий фликер». Под нагрузкой главного потока (перетаскивание карточек: и
     SortableJS, и перезагрузка доски по WS) кадр держится дольше — поэтому на
     drag'е заметно, а на создании задачи почти нет. Слой держим на .activity-stack. */
}
.t-ava {
  flex: none;
  display: inline-flex;
  align-items: flex-start;
  padding-top: 1px;
}
.t-ava .ava {
  width: 30px;
  height: 30px;
  border-radius: 50%;
}
.t-body {
  flex: 1;
  min-width: 0;
}
.t-head {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  line-height: 1.3;
}
.t-who {
  font-weight: 600;
  color: var(--t-text2);
}
.t-verb {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.t-title {
  margin: 2px 0 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--t-text1);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: anywhere;
}
.t-actions {
  display: flex;
  gap: 6px;
}
.t-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  padding: 3px 9px;
  border-radius: 7px;
  border: 1px solid var(--t-border);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.t-btn:hover {
  background: var(--t-hover);
}
.t-btn.primary {
  border-color: transparent;
  background: var(--t-accent-grad, var(--t-primary));
  color: var(--t-on-primary, #fff);
}
.t-close {
  position: absolute;
  top: 6px;
  right: 6px;
  display: inline-flex;
  border: none;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  border-radius: 6px;
  padding: 1px;
}
.t-close:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
/* Анимация ТОЛЬКО по transform, без opacity. Затухание меняет отрисовку тоста
   покадрово и требует пересборки растра — а это тот самый пустой кадр. Чистый
   выезд не меняет содержимое вообще: слой .activity-stack растеризован один раз,
   меняется только смещение. Заодно уходит и переключение сглаживания текста
   (grayscale ↔ субпиксельное), из-за которого мерцало в конце анимации.
   -370px = 16 (отступ стопки) + 320 (ширина) + запас на тень: тост уезжает за
   левый край вьюпорта целиком, поэтому «пропадания» на месте не видно. */
.toast-enter-active,
.toast-leave-active {
  transition: transform 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}
.toast-enter-from,
.toast-leave-to {
  transform: translate3d(-370px, 0, 0);
}
/* transition-group вешает этот класс на оставшиеся тосты, когда из стопки
   вытесняется самый старый (push() → splice по CAP). Без правила они прыгают. */
.toast-move {
  transition: transform 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}
@media (prefers-reduced-motion: reduce) {
  .toast-enter-active,
  .toast-leave-active,
  .toast-move {
    transition: none;
  }
  .activity-stack {
    will-change: auto;
  }
}
</style>
