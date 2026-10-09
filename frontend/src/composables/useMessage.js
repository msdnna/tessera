import { getCurrentInstance, inject } from 'vue'
import { useToastsStore } from '@/stores/toasts'

// The answer to my own action (v2, #3117 — PackageI «Ответ на моё действие»):
// what naive's `useMessage()` used to be, drawn as a toast of the bottom lane —
// an error with the server's details under «Подробнее» (and «Повторить» when
// the caller hands a retry), a warning, a success, an info. Same call shape as
// naive's, so the swap is an import line:
//
//   const message = useMessage()
//   message.error(e.message)                         // 8 s, or until closed with { retry }
//   message.error(text, { retry: () => save() })     // «Повторить ↵»
//   message.success(t('…'), { action: { label, run } })
//   message.warning(text) · message.info(text)
//   const m = message.loading(text); m.destroy()
//
// Durations are the toasts' own (PackageI «Тайминги»); an explicit `duration`
// still wins.
//
// A provider above the component wins over the store — under the key naive's
// provider used, which is where the specs hand in their spy bus
// (tests/setup.js); App.vue provides this very API under it as well, so a
// leftover naive `useMessage()` lands in the same lane.
export const MESSAGE_KEY = 'n-message-api'

export function messageApi(store = useToastsStore()) {
  const say = (kind) => (content, opts) => store.respond(kind, content, opts || {})
  return {
    error: say('error'),
    warning: say('warning'),
    success: say('success'),
    info: say('info'),
    loading: say('loading'),
    create: say('info'),
    destroyAll: () => store.dismissLane('bottom'),
  }
}

export function useMessage() {
  if (getCurrentInstance()) {
    const injected = inject(MESSAGE_KEY, null)
    if (injected) return injected
  }
  return messageApi()
}
