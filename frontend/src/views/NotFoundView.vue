<script setup>
// Branded catch-all for unknown routes (and a soft fallback for other
// non-critical navigation dead-ends). Sits on the brand gradient like the auth
// screens; works signed in or out. Without it, an unmatched path renders a blank
// (black) screen.
import { RouterLink } from 'vue-router'
import AuthLayout from '@/components/AuthLayout.vue'

defineProps({
  code: { type: String, default: '404' },
  title: { type: String, default: 'Страница не найдена' },
  text: {
    type: String,
    default: 'Такой страницы нет или она была перемещена.',
  },
})
</script>

<template>
  <auth-layout>
    <div class="nf">
      <div class="nf-code">{{ code }}</div>
      <div class="nf-title">{{ title }}</div>
      <p class="nf-text">{{ text }}</p>
    </div>
    <div class="auth-foot"><router-link to="/">На главную</router-link></div>
  </auth-layout>
</template>

<style scoped>
.nf {
  text-align: center;
  color: #fff;
}
.nf-code {
  font-size: 56px;
  font-weight: 800;
  letter-spacing: 1px;
  line-height: 1;
  text-shadow: 0 6px 18px rgba(24, 11, 70, 0.35);
}
.nf-title {
  margin-top: 12px;
  font-size: 18px;
  font-weight: 600;
}
.nf-text {
  margin-top: 8px;
  font-size: 14px;
  line-height: 1.5;
  color: rgba(255, 255, 255, 0.85);
}
</style>
