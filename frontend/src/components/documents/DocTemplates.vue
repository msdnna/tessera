<script setup>
import { computed, ref } from 'vue'
import { NButton, NCard, NIcon, NInput, NModal, NPopconfirm, NSpin, NText } from 'naive-ui'
import { CloudUploadOutline, DocumentTextOutline, TrashOutline } from '@vicons/ionicons5'
import { IMPORT_EXTENSIONS } from '@/utils/docImport'

// The template gallery (#2734). Presentational, like DocHistory: it renders the
// cards and reports what the user picked, while the view owns the API calls.
// Saved templates and the built-in starters arrive as one list already shaped
// by the caller — a card does not need to know where it came from, only whether
// it can be deleted (built-ins cannot).
const props = defineProps({
  show: { type: Boolean, default: false },
  templates: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  // Set while a document is being created from a template, so the gallery can
  // disable itself instead of letting an impatient second click create a second
  // document.
  busy: { type: String, default: '' },
  error: { type: String, default: '' },
})
const emit = defineEmits(['update:show', 'use', 'upload', 'remove'])

const fileInput = ref(null)
const query = ref('')

const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.templates
  return props.templates.filter((t) =>
    `${t.title} ${t.description || ''}`.toLowerCase().includes(q),
  )
})

function pickFile() {
  fileInput.value?.click()
}

function onFile(e) {
  const file = e.target.files?.[0]
  // Reset first: picking the same file twice in a row fires no change event
  // otherwise, and a failed import is exactly when someone retries the same file.
  e.target.value = ''
  if (file) emit('upload', file)
}
</script>

<template>
  <n-modal :show="show" transform-origin="center" @update:show="(v) => emit('update:show', v)">
    <n-card class="tpl-card" :bordered="false" role="dialog" aria-modal="true">
      <div class="tpl-head">
        <span class="tpl-title">Шаблоны документов</span>
        <n-input
          v-model:value="query"
          size="small"
          clearable
          placeholder="Поиск"
          class="tpl-search"
        />
        <n-button size="small" data-testid="tpl-upload" @click="pickFile">
          <template #icon><n-icon :component="CloudUploadOutline" /></template>
          Загрузить
        </n-button>
      </div>
      <n-text v-if="error" type="error" class="tpl-error">{{ error }}</n-text>

      <n-spin v-if="loading" size="small" />
      <div v-else-if="shown.length" class="tpl-grid">
        <div
          v-for="t in shown"
          :key="t.id"
          class="tpl-tile"
          :class="{ builtin: t.builtin }"
          data-testid="tpl-tile"
        >
          <div class="tile-head">
            <span v-if="t.icon" class="tpl-emoji">{{ t.icon }}</span>
            <n-icon v-else :component="DocumentTextOutline" :size="16" />
            <span class="tile-title">{{ t.title }}</span>
          </div>
          <p class="tile-desc">{{ t.description || t.preview || 'Без описания' }}</p>
          <div class="tile-foot">
            <span class="tile-origin">{{ t.builtin ? 'Встроенный' : t.author_name || '' }}</span>
            <span class="tile-actions">
              <n-popconfirm v-if="!t.builtin" @positive-click="emit('remove', t)">
                <template #trigger>
                  <n-button quaternary size="tiny" title="Удалить шаблон">
                    <template #icon><n-icon :component="TrashOutline" /></template>
                  </n-button>
                </template>
                Удалить шаблон? Документы, созданные по нему, останутся.
              </n-popconfirm>
              <n-button
                size="tiny"
                type="primary"
                :loading="busy === t.id"
                :disabled="!!busy"
                data-testid="tpl-use"
                @click="emit('use', t)"
              >
                Создать
              </n-button>
            </span>
          </div>
        </div>
      </div>
      <p v-else class="tpl-empty">
        Ничего не нашлось. Сохраните документ как шаблон или загрузите файл ({{
          IMPORT_EXTENSIONS.join(', ')
        }}).
      </p>

      <input
        ref="fileInput"
        type="file"
        class="tpl-file"
        :accept="IMPORT_EXTENSIONS.join(',')"
        @change="onFile"
      />
    </n-card>
  </n-modal>
</template>

<style scoped>
.tpl-card {
  width: min(760px, 94vw);
  max-height: 84vh;
  overflow-y: auto;
}
.tpl-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.tpl-title {
  font-weight: 600;
  margin-right: auto;
}
.tpl-search {
  max-width: 220px;
}
.tpl-error {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
}
.tpl-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 10px;
}
.tpl-tile {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--t-border);
  border-radius: 10px;
  background: var(--t-surface);
}
/* Built-ins are marked by a flat neutral tint, not by an accent: they are not
   more important than the team's own templates, only differently sourced. */
.tpl-tile.builtin {
  background: var(--t-surface-alt);
}
.tile-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.tile-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tile-desc {
  margin: 0;
  font-size: 12px;
  line-height: 1.4;
  color: var(--t-text3);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  -webkit-box-orient: vertical;
}
.tile-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  margin-top: auto;
}
.tile-origin {
  font-size: 11px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tile-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
.tpl-empty {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--t-text3);
}
.tpl-file {
  display: none;
}
</style>
