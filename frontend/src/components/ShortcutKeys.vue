<script setup>
// Key caps of a catalog shortcut (v2, #2956): «G затем B», «⌘ ⇧ C». Rendered
// from the same catalog the dispatcher reads, so a hint can't drift from the
// key that actually works.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Kbd from './Kbd.vue'
import { chordsOf } from '@/shortcuts/labels'

const props = defineProps({
  id: { type: String, required: true },
  // One cap per chord («⌘K») instead of one per key — for tight spots like the
  // hint inside the search field.
  joined: { type: Boolean, default: false },
  soft: { type: Boolean, default: false },
  // Sequences without «затем» between the chords («N M») — the mockups' tight
  // spots: a panel's header button, a footer hint (#2997).
  bare: { type: Boolean, default: false },
})

const { t } = useI18n()
const chords = computed(() => chordsOf(props.id))
const capsText = (caps) => (caps[0] === 'Ctrl' ? caps.join(' ') : caps.join(''))
</script>

<template>
  <span class="sk">
    <template v-for="(caps, i) in chords" :key="i">
      <span v-if="i > 0 && !bare" class="sk-then">{{ t('shell.keys.then') }}</span>
      <Kbd v-if="joined" :soft="soft">{{ capsText(caps) }}</Kbd>
      <template v-else>
        <Kbd v-for="(cap, j) in caps" :key="j" :soft="soft">{{ cap }}</Kbd>
      </template>
    </template>
  </span>
</template>

<style scoped>
.sk {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: none;
}
.sk-then {
  margin: 0 3px;
  font-size: 11px;
  color: var(--t-text3);
  white-space: nowrap;
}
</style>
