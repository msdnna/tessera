<script setup>
// Branded catch-all for unknown routes (and a soft fallback for other
// non-critical navigation dead-ends), in the account screens' frame so it works
// signed in or out (v2, #3107 — PackageE «404»): the code in the accent
// gradient, a title, a line, and two ways on — home (↵) and back (Esc).
import { onMounted, onBeforeUnmount } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import AuthLayout from '@/components/AuthLayout.vue'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import '@/components/kit/kit.css'

// The overridable texts default to '' rather than to the Russian wording: a
// non-empty default is never falsy, so the `|| $t(…)` fallback in the template
// would never fire and the screen would stay Russian in every locale (#2799).
defineProps({
  code: { type: String, default: '404' },
  title: { type: String, default: '' },
  text: { type: String, default: '' },
})

const router = useRouter()

function back() {
  if (window.history.length > 1) router.back()
  else router.push('/')
}
// The page's own two keys: there is no app shell here to dispatch them.
function onKey(e) {
  if (e.defaultPrevented || e.target?.closest?.('input, textarea, [contenteditable]')) return
  if (e.key === 'Enter') {
    e.preventDefault()
    router.push('/')
  } else if (e.key === 'Escape') {
    e.preventDefault()
    back()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <auth-layout>
    <div class="nf" data-testid="not-found">
      <div class="nf-code accent-grad-text">{{ code }}</div>
      <div class="nf-title">{{ title || $t('shell.notFound.title') }}</div>
      <p class="nf-text">{{ text || $t('shell.notFound.text') }}</p>
      <div class="nf-actions">
        <router-link to="/" class="kit-btn primary" data-testid="not-found-home">
          {{ $t('common.auth.toHome') }}<Kbd>{{ CAPS.enter }}</Kbd>
        </router-link>
        <button type="button" class="kit-btn" @click="back">
          {{ $t('shell.notFound.back') }}<Kbd>{{ CAPS.escape }}</Kbd>
        </button>
      </div>
    </div>
  </auth-layout>
</template>

<style scoped>
.nf {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 8px 0 4px;
}
.nf-code {
  font-size: 76px;
  font-weight: 800;
  line-height: 1;
  letter-spacing: -0.03em;
}
.nf-title {
  margin-top: 14px;
  font-size: 18px;
  font-weight: 700;
  color: var(--t-text1);
}
.nf-text {
  margin: 8px 0 0;
  max-width: 320px;
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--t-text2);
}
.nf-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 20px;
}
</style>
