<script setup>
import { computed, ref } from 'vue'
import { NButton, NText } from 'naive-ui'
import { headingLabel } from '@/utils/docToc'

// The outline of the open document (#2733). It is a navigation panel and
// nothing else: it draws what docToc derived from the live tree and reports
// which entry was clicked. Deriving the rows here instead would tie the outline
// to a mounted component, and the rules worth testing (nesting, which section
// the caret is in) are exactly the ones that would become untestable.
//
// Since #2728 it is a rail of ticks that opens on hover, not a 260px column.
// The column was a third panel competing for the same row as the discussion and
// the history, and it is what left the text 9px wide with three panels open
// (#2738). The rail costs 22px and the list of titles is a popover over the
// text, which is also how a reader uses an outline: glance, jump, gone.
const props = defineProps({
  // Rows from docOutline: {id, level, text, depth}.
  rows: { type: Array, default: () => [] },
  // The heading whose section the caret is in, highlighted so a long document
  // says where you are and not only where you can go.
  activeId: { type: String, default: '' },
  // On a phone the rail is not a control: there is no hover to open it with,
  // and the popover it opens is anchored `right: 100%` off a rail that sits at
  // the left edge of the screen — it lands outside the viewport (#2893). So the
  // whole hover mechanism is dropped there and the outline becomes a sheet.
  narrow: { type: Boolean, default: false },
})
const emit = defineEmits(['go', 'close'])

// Hover drives the popover, but focus has to open it too: the entries are
// buttons, and a Tab into a display:none list would be a keyboard trap where
// the focus ring lands on nothing.
//
// The two are tracked apart rather than folded into one flag on purpose. With a
// single flag, clicking an entry closes the outline under the pointer: the
// click focuses the button, the jump then pulls focus back into the editor, and
// the focusout that follows would clear the flag the pointer is still holding.
const hovered = ref(false)
const focused = ref(false)
// Narrow forces it open because there the outline is only on screen when the
// user asked for it: the toolbar toggle mounts this component, so "mounted" and
// "open" are the same statement. Keeping it hover-gated would render a sheet
// that never shows its contents.
const open = computed(() => props.narrow || hovered.value || focused.value)

// A tick is as wide as its heading is shallow, so the rail is a silhouette of
// the document rather than a row of identical dashes.
function tickWidth(depth) {
  return Math.max(16 - depth * 3, 7)
}
</script>

<template>
  <aside
    class="doc-toc"
    :class="{ narrow }"
    data-testid="doc-toc"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
    @focusin="focused = true"
    @focusout="focused = false"
  >
    <!-- The collapsed state. Ticks are decoration for a list that is already in
         the DOM below, so they are not focusable and not announced twice. -->
    <div class="rail" :class="{ faded: open }" aria-hidden="true">
      <span
        v-for="row in rows"
        :key="row.id"
        class="tick"
        :class="{ active: row.id === activeId }"
        :style="{ width: `${tickWidth(row.depth)}px` }"
        data-testid="doc-toc-tick"
      />
    </div>

    <!-- v-show, not v-if: the entries stay mounted so the outline can be read
         by assistive tech and reached by Tab without a hover first. -->
    <div v-show="open" class="flyout" data-testid="doc-toc-flyout">
      <div class="panel-head">
        <span class="panel-title">{{ $t('documents.toc.title') }}</span>
        <span class="grow" />
        <n-button quaternary size="tiny" @click="emit('close')">{{
          $t('common.action.close')
        }}</n-button>
      </div>

      <div class="panel-body">
        <p v-if="!rows.length" class="empty">
          <n-text depth="3">{{ $t('documents.toc.empty') }}</n-text>
        </p>
        <!-- Indent is a left padding on a flat list rather than nested lists: on
             a narrow popover a fourth-level heading inside four <ul>s has almost
             no width left for its own text. -->
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
    </div>
  </aside>
</template>

<style scoped>
/* The reference frame for the popover, which is positioned off this rail rather
   than off the work area: the rail is the thing the pointer is on. */
.doc-toc {
  position: relative;
  flex: none;
  width: 22px;
  min-height: 0;
  display: flex;
  justify-content: center;
  padding-top: 10px;
}
.rail {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  width: 16px;
  /* The rail can outgrow the viewport in a long document; it clips rather than
     scrolls, because it is a hint and the popover is the real list. */
  overflow: hidden;
  transition: opacity 0.12s ease;
}
/* Hidden by opacity, not by display: it must keep receiving the pointer, or the
   popover would close the instant it opened. */
.rail.faded {
  opacity: 0;
}
.tick {
  height: 2px;
  flex: none;
  border-radius: 1px;
  background: var(--t-text3);
}
.tick.active {
  background: var(--t-primary);
}
.flyout {
  position: absolute;
  top: 0;
  /* Opens to the left, over the text: to the right is the window edge. */
  right: 100%;
  z-index: 20;
  width: 260px;
  max-height: 60vh;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
}
@media (max-width: 900px) {
  .flyout {
    width: 220px;
  }
}
/* Phone layout: a sheet over the working area, the same shape the history and
   the links panel already take there. Absolute against `.work`, which is the
   positioned ancestor — the outline must not join the single column, or it
   lands between the text and the discussion as a strip of ticks nobody can
   read (задача 2893). */
.doc-toc.narrow {
  position: absolute;
  inset: 0;
  width: auto;
  padding: 0;
  /* Above the annotation layer (1) and level with the sidebar sheet (2). */
  z-index: 2;
}
/* The silhouette is a hover affordance; with the sheet always open there is
   nothing left for it to hint at. */
.doc-toc.narrow .rail {
  display: none;
}
.doc-toc.narrow .flyout {
  position: static;
  width: 100%;
  max-height: none;
  height: 100%;
  box-shadow: 0 0 20px var(--t-border);
}
/* Full-width rows: on a phone the outline is the only thing on screen, so a
   heading has the whole width instead of an ellipsis after four words. */
.doc-toc.narrow .entry {
  padding-top: 8px;
  padding-bottom: 8px;
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
