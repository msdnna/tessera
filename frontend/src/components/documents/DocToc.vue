<script setup>
import { NButton, NIcon, NText } from 'naive-ui'
import { ListOutline } from '@vicons/ionicons5'
import { headingLabel } from '@/utils/docToc'

// The outline of the open document (#2733). It is a navigation panel and
// nothing else: it draws what docToc derived from the live tree and reports
// which entry was clicked. Deriving the rows here instead would tie the outline
// to a mounted component, and the rules worth testing (nesting, which section
// the caret is in) are exactly the ones that would become untestable.
defineProps({
  // Rows from docOutline: {id, level, text, depth}.
  rows: { type: Array, default: () => [] },
  // The heading whose section the caret is in, highlighted so a long document
  // says where you are and not only where you can go.
  activeId: { type: String, default: '' },
})
const emit = defineEmits(['go', 'close'])
</script>

<template>
  <aside class="doc-toc" data-testid="doc-toc">
    <div class="panel-head">
      <n-icon :component="ListOutline" :size="16" />
      <span class="panel-title">Оглавление</span>
      <span class="grow" />
      <n-button quaternary size="tiny" @click="emit('close')">Закрыть</n-button>
    </div>

    <div class="panel-body">
      <p v-if="!rows.length" class="empty">
        <n-text depth="3">Добавьте заголовки — они соберутся в оглавление.</n-text>
      </p>
      <!-- Indent is a left padding on a flat list rather than nested lists: on a
           300px panel a fourth-level heading inside four <ul>s has almost no
           width left for its own text. -->
      <button
        v-for="row in rows"
        :key="row.id"
        type="button"
        class="entry"
        :class="{ active: row.id === activeId }"
        :style="{ paddingLeft: `${6 + row.depth * 12}px` }"
        :data-block-id="row.id"
        data-testid="doc-toc-entry"
        @click="emit('go', row.id)"
      >
        {{ headingLabel(row) }}
      </button>
    </div>
  </aside>
</template>

<style scoped>
.doc-toc {
  display: flex;
  flex-direction: column;
  width: 260px;
  flex: none;
  min-height: 0;
  border-left: 1px solid var(--t-border);
  padding-left: 12px;
  gap: 8px;
}
@media (max-width: 900px) {
  .doc-toc {
    width: auto;
    max-height: 40vh;
    border-left: none;
    border-top: 1px solid var(--t-border);
    padding-left: 0;
    padding-top: 12px;
  }
}
.panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--t-text2);
}
.panel-title {
  font-size: 13px;
  font-weight: 600;
}
.grow {
  flex: 1;
}
.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.empty {
  font-size: 12px;
  margin: 0;
}
/* Entries are neutral; only the section being read takes the accent, as in the
   history and discussion panels. */
.entry {
  display: block;
  width: 100%;
  padding: 4px 6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font-size: 13px;
  line-height: 1.35;
  text-align: left;
  cursor: pointer;
  /* A heading can be a sentence long; the outline stays one line per entry so
     the structure remains readable as a structure. */
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.entry:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.entry.active {
  color: var(--t-primary);
  background: var(--t-hover);
}
</style>
