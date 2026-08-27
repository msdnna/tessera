<script setup>
import { ref, computed } from 'vue'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { accountFlows } from '@/api'
import AuthLayout from '@/components/AuthLayout.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const token = route.query.token || ''
const pw = ref('')
const confirm = ref('')
const loading = ref(false)
const done = ref(false)
const valid = computed(() => pw.value.length >= 8 && pw.value === confirm.value)

async function submit() {
  if (!valid.value || !token) return
  loading.value = true
  try {
    await accountFlows.resetPassword(token, pw.value)
    done.value = true
    setTimeout(() => router.push('/login'), 1500)
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <auth-layout :title="$t('common.auth.reset.title')">
    <p v-if="!token" class="note">{{ $t('common.auth.reset.noToken') }}</p>
    <p v-else-if="done" class="note">{{ $t('common.auth.reset.done') }}</p>
    <template v-else>
      <n-form @submit.prevent="submit">
        <n-form-item :label="$t('common.auth.reset.password')">
          <n-input v-model:value="pw" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item :label="$t('common.auth.reset.confirm')">
          <n-input
            v-model:value="confirm"
            type="password"
            show-password-on="click"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-button type="primary" block :disabled="!valid" :loading="loading" @click="submit">
          {{ $t('common.auth.reset.submit') }}
        </n-button>
      </n-form>
    </template>
    <div class="auth-foot">
      <router-link to="/login">{{ $t('common.auth.reset.toLogin') }}</router-link>
    </div>
  </auth-layout>
</template>

<style scoped>
.note {
  color: #fff;
  font-size: 14px;
  line-height: 1.5;
}
</style>
