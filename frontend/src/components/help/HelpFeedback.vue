<script setup>
// «Статья помогла? Да / Нет» under a help article (#3071, PackageE «HelpWin»).
//
// No table of its own: a vote is a per-user acknowledgement keyed
// `helpful:<slug>:<lang>:yes|no` (user_acknowledgements, 0064) — one vote per
// reader per article and language, the first one kept. The administration
// reads the totals back (GET /admin/help-feedback).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ThumbsUpOutline, ThumbsDownOutline, CheckmarkOutline } from '@vicons/ionicons5'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { helpVoteKey } from '@/utils/helpFeedback'

const props = defineProps({
  slug: { type: String, required: true },
  lang: { type: String, default: 'ru' },
})

const { t } = useI18n()
const acks = useWhatsNewStore()

const voted = computed(
  () =>
    acks.has(helpVoteKey(props.slug, props.lang, 'yes')) ||
    acks.has(helpVoteKey(props.slug, props.lang, 'no')),
)
function vote(v) {
  if (voted.value) return
  acks.ack(helpVoteKey(props.slug, props.lang, v))
}
</script>

<template>
  <div class="hf" data-testid="help-feedback">
    <template v-if="!voted">
      <span class="hf-q">{{ t('help.feedback.question') }}</span>
      <button type="button" class="hf-btn" data-testid="help-feedback-yes" @click="vote('yes')">
        <n-icon :component="ThumbsUpOutline" :size="13" />{{ t('help.feedback.yes') }}
      </button>
      <button type="button" class="hf-btn" data-testid="help-feedback-no" @click="vote('no')">
        <n-icon :component="ThumbsDownOutline" :size="13" />{{ t('help.feedback.no') }}
      </button>
    </template>
    <span v-else class="hf-thanks" data-testid="help-feedback-thanks">
      <n-icon :component="CheckmarkOutline" :size="14" />{{ t('help.feedback.thanks') }}
    </span>
  </div>
</template>

<style scoped>
.hf {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--t-text3);
}
.hf-q {
  margin-right: 4px;
}
.hf-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 28px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.hf-btn:hover {
  background: var(--t-hover);
}
.hf-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.hf-thanks {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #18a058;
}
</style>
