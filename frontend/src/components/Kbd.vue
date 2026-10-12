<script setup>
// One key cap of a shortcut hint (v2, #2951): «⌘K», «Esc», «C». Pass the
// label through the default slot; `soft` is the flatter cap used inside fields.
defineProps({
  soft: { type: Boolean, default: false },
})
</script>

<template>
  <kbd class="kbd" :class="{ soft }"><slot /></kbd>
</template>

<style scoped>
.kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: var(--t-r-kbd);
  background: var(--t-kbd);
  color: var(--t-text2);
  font: inherit;
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  box-shadow: 0 1px 1px rgba(0, 0, 0, 0.22);
}
.kbd.soft {
  height: 22px;
  padding: 0 7px;
  border-radius: 6px;
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 11.5px;
  box-shadow: none;
}
</style>
