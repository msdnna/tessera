<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NButton, NInput, NText, useMessage } from 'naive-ui'
import { LogoGitlab, CheckmarkOutline } from '@vicons/ionicons5'
import { gitlab as glApi } from '@/api'
import { diffSegments } from '@/utils/linediff'
import { priorityLabel } from '@/utils/priority'
import EmptyState from '@/components/EmptyState.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'

// Embeddable body of the GitLab write-back conflict resolver — rendered inside
// the GitLab integration modal's right pane, and inside a thin modal wrapper
// (ConflictResolverModal) for the app-level entry point.
const props = defineProps({
  wsId: { type: String, default: null },
  // When opened from a specific task, pre-select that task's conflict.
  focusTaskId: { type: String, default: null },
})
// 'empty' fires when the last conflict is resolved so a modal wrapper can close.
const emit = defineEmits(['resolved', 'empty'])

const message = useMessage()
const { t } = useI18n()

const conflicts = ref([])
const loading = ref(false)
const selectedId = ref(null)
const resolving = ref(false)
const manualField = ref(null) // field key currently being merged, or null
const manualValues = ref({}) // field → string

// Field and change-kind names come from the catalog per call rather than from a
// module-level map: a frozen map would keep the language of the first render
// (pitfall 1 of the #2799 plan). An unknown key from the server is shown raw.
const FIELD_KEYS = ['title', 'description', 'due', 'estimate', 'state', 'priority']
const KIND_KEYS = ['title_desc', 'due', 'estimate', 'state', 'priority']

const selected = computed(() => conflicts.value.find((c) => c.id === selectedId.value) || null)
// Manual merge makes sense only for free-text/numeric fields; state and priority
// are discrete (ours/theirs is enough), so hide the manual option for them.
const manualAllowed = computed(() =>
  (selected.value?.fields || []).every((f) => !['state', 'priority'].includes(f.field)),
)

function fieldLabel(f) {
  return FIELD_KEYS.includes(f) ? t(`gitlab.conflicts.field.${f}`) : f
}
function kindLabel(k) {
  return KIND_KEYS.includes(k) ? t(`gitlab.conflicts.kind.${k}`) : k
}
function emptyVal(v) {
  return v === '' || v == null
}
// Multi-line text fields (title/description) get a line-level diff vs the base so
// the diverged lines are highlighted; short scalar values render plain.
function isTextField(field) {
  return field === 'title' || field === 'description'
}
// Human-readable value for discrete fields (state/priority); others pass through.
function displayVal(field, v) {
  if (emptyVal(v)) return t('gitlab.conflicts.emptyValue')
  if (field === 'state') {
    return t(v === 'closed' ? 'gitlab.conflicts.state.closed' : 'gitlab.conflicts.state.open')
  }
  if (field === 'priority') return priorityLabel(v) || v
  return v
}
const theirsDiff = (f) => diffSegments(f.base, f.theirs)
const oursDiff = (f) => diffSegments(f.base, f.ours)

async function load() {
  if (!props.wsId) return
  loading.value = true
  try {
    const { data } = await glApi.conflicts(props.wsId)
    conflicts.value = data || []
    // Pre-select the focused task's conflict (when opened from a card), else keep
    // the current selection if still present, else the first.
    const focused =
      props.focusTaskId && conflicts.value.find((c) => c.task_id === props.focusTaskId)
    if (focused) {
      selectedId.value = focused.id
      manualField.value = null
    } else if (!conflicts.value.find((c) => c.id === selectedId.value)) {
      selectedId.value = conflicts.value[0]?.id || null
    }
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

function pick(c) {
  selectedId.value = c.id
  manualField.value = null
  manualValues.value = {}
}

function startManual() {
  // Seed the editable values with GitLab's side (theirs) as a starting point.
  const seed = {}
  for (const f of selected.value?.fields || []) seed[f.field] = f.theirs
  manualValues.value = seed
  manualField.value = 'all'
}

async function resolve(resolution) {
  const c = selected.value
  if (!c) return
  resolving.value = true
  try {
    const body = { resolution }
    if (resolution === 'manual') body.value = { ...manualValues.value }
    await glApi.resolveConflict(c.task_id, c.id, body)
    message.success(t('gitlab.conflicts.resolved'))
    conflicts.value = conflicts.value.filter((x) => x.id !== c.id)
    selectedId.value = conflicts.value[0]?.id || null
    manualField.value = null
    emit('resolved', c)
    if (!conflicts.value.length) emit('empty')
  } catch (e) {
    message.error(e.message)
  } finally {
    resolving.value = false
  }
}

onMounted(load)
watch(() => [props.wsId, props.focusTaskId], load)

defineExpose({ reload: load })
</script>

<template>
  <div class="cp-wrap">
    <div class="c-body">
      <!-- LEFT: open conflicts -->
      <div class="c-left">
        <empty-state
          v-if="!loading && !conflicts.length"
          size="small"
          :icon="LogoGitlab"
          :text="$t('gitlab.conflicts.empty')"
        />
        <button
          v-for="c in conflicts"
          :key="c.id"
          class="c-item"
          :class="{ on: c.id === selectedId }"
          @click="pick(c)"
        >
          <span class="c-item-main">
            <span class="c-item-title">{{
              c.task_title || $t('gitlab.conflicts.taskFallback')
            }}</span>
            <span class="c-item-meta">
              <span v-if="c.task_number" class="c-num">#{{ c.task_number }}</span>
              {{ kindLabel(c.change_kind) }}
            </span>
          </span>
          <span class="c-badge">{{ (c.fields || []).length }}</span>
        </button>
      </div>

      <!-- RIGHT: three-way detail + actions -->
      <div class="c-right">
        <empty-state v-if="!selected" size="small" :text="$t('gitlab.conflicts.select')" />
        <template v-else>
          <p class="c-hint">
            <!-- Two whole sentences, not one plus an optional tail: where the
                 "or merge them" clause attaches is not the same in every
                 language, so it cannot be glued on at render time. -->
            <n-text depth="3">
              {{ manualAllowed ? $t('gitlab.conflicts.hintManual') : $t('gitlab.conflicts.hint') }}
            </n-text>
          </p>

          <div v-for="f in selected.fields" :key="f.field" class="c-field">
            <div class="c-field-name">{{ fieldLabel(f.field) }}</div>
            <div class="c-three">
              <div class="c-col">
                <div class="c-col-lbl">{{ $t('gitlab.conflicts.colBase') }}</div>
                <div class="c-val base" :class="{ empty: emptyVal(f.base) }">
                  {{ displayVal(f.field, f.base) }}
                </div>
              </div>
              <div class="c-col">
                <div class="c-col-lbl gl">GitLab</div>
                <div class="c-val theirs" :class="{ empty: emptyVal(f.theirs) }">
                  <template v-if="emptyVal(f.theirs)">{{
                    $t('gitlab.conflicts.emptyValue')
                  }}</template>
                  <template v-else-if="isTextField(f.field)"
                    ><span
                      v-for="(seg, si) in theirsDiff(f)"
                      :key="si"
                      :class="{ 'ch-theirs': seg.changed }"
                      >{{ seg.text }}</span
                    ></template
                  >
                  <template v-else>{{ displayVal(f.field, f.theirs) }}</template>
                </div>
              </div>
              <div class="c-col">
                <div class="c-col-lbl mine">{{ $t('gitlab.conflicts.colOurs') }}</div>
                <div class="c-val ours" :class="{ empty: emptyVal(f.ours) }">
                  <template v-if="emptyVal(f.ours)">{{
                    $t('gitlab.conflicts.emptyValue')
                  }}</template>
                  <template v-else-if="isTextField(f.field)"
                    ><span
                      v-for="(seg, si) in oursDiff(f)"
                      :key="si"
                      :class="{ 'ch-ours': seg.changed }"
                      >{{ seg.text }}</span
                    ></template
                  >
                  <template v-else>{{ displayVal(f.field, f.ours) }}</template>
                </div>
              </div>
            </div>
            <div v-if="manualField" class="c-manual">
              <div class="c-col-lbl">{{ $t('gitlab.conflicts.merged') }}</div>
              <n-input
                v-if="f.field === 'description'"
                v-model:value="manualValues[f.field]"
                type="textarea"
                :autosize="{ minRows: 3, maxRows: 10 }"
                :placeholder="$t('gitlab.conflicts.mergedPlaceholder')"
              />
              <n-input
                v-else
                v-model:value="manualValues[f.field]"
                :placeholder="$t('gitlab.conflicts.mergedPlaceholder')"
              />
            </div>
          </div>

          <div class="c-actions">
            <n-button :disabled="resolving" @click="resolve('ours')">
              {{ $t('gitlab.conflicts.acceptOurs') }}
            </n-button>
            <n-button :disabled="resolving" @click="resolve('theirs')">
              {{ $t('gitlab.conflicts.acceptTheirs') }}
            </n-button>
            <n-button
              v-if="manualAllowed && !manualField"
              :disabled="resolving"
              @click="startManual"
            >
              {{ $t('gitlab.conflicts.mergeManual') }}
            </n-button>
            <n-button
              v-else-if="manualAllowed"
              type="primary"
              :loading="resolving"
              @click="resolve('manual')"
            >
              <template #icon><n-icon :component="CheckmarkOutline" /></template>
              {{ $t('gitlab.conflicts.saveMerge') }}
            </n-button>
          </div>
        </template>
      </div>
    </div>

    <loader-overlay :show="loading" contained :messages="[$t('gitlab.conflicts.loading')]" />
  </div>
</template>

<style scoped>
.cp-wrap {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
}
.c-body {
  display: flex;
  gap: 16px;
  flex: 1 1 auto;
  min-height: 320px;
}
.c-left {
  width: 280px;
  flex: none;
  border-right: 1px solid var(--t-border);
  padding-right: 12px;
  overflow-y: auto;
  max-height: 60vh;
}
.c-item {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
  text-align: left;
}
.c-item:hover {
  background: var(--t-hover);
}
.c-item.on {
  background: var(--t-hover);
  border-color: var(--t-border);
}
.c-item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.c-item-title {
  font-size: 13px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.c-item-meta {
  font-size: 11px;
  color: var(--t-text3);
}
.c-num {
  color: var(--t-text2);
  margin-right: 4px;
}
.c-badge {
  flex: none;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: color-mix(in srgb, #e0922f 22%, transparent);
  color: #b96a08;
  font-weight: 600;
  font-size: 11px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.c-right {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  max-height: 60vh;
}
.c-hint {
  margin: 0 0 12px;
}
.c-field {
  margin-bottom: 16px;
}
.c-field-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  margin-bottom: 6px;
}
.c-three {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 8px;
}
.c-col-lbl {
  font-size: 11px;
  color: var(--t-text3);
  margin-bottom: 3px;
}
.c-col-lbl.gl {
  color: #d03050;
}
.c-col-lbl.mine {
  color: var(--t-primary);
}
.c-val {
  font-size: 12px;
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--t-border);
  background: var(--t-surface-2, var(--t-hover));
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 180px;
  overflow-y: auto;
}
.c-val.theirs {
  border-color: #d0305066;
}
.c-val.ours {
  border-color: var(--t-primary);
}
.c-val.empty {
  color: var(--t-text3);
  font-style: italic;
}
/* inline diff highlight: tint only the spans that diverged from the base */
.ch-theirs {
  background: color-mix(in srgb, #d03050 30%, transparent);
  border-radius: 3px;
  padding: 0 1px;
}
.ch-ours {
  background: color-mix(in srgb, var(--t-primary) 30%, transparent);
  border-radius: 3px;
  padding: 0 1px;
}
.c-manual {
  margin-top: 8px;
}
.c-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--t-border);
}
</style>
