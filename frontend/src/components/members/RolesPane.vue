<script setup>
// «Роли и права» of the members window (#3063): what each of the three roles
// may do here, read off the server's guards (utils/memberActivity ROLE_MATRIX).
// Read-only — the editor of roles (#3076) and the guest role with access to
// chosen projects (#3075) need a per-project permission check on every read and
// are their own tasks; they stand here as «Скоро» stubs.
import { NIcon, NTooltip } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { CheckmarkOutline, RemoveOutline } from '@vicons/ionicons5'
import { ROLES, ROLE_MATRIX } from '@/utils/memberActivity'
import { stubTask } from '@/utils/featureStubs'

defineProps({
  myRole: { type: String, default: '' },
})
const { t } = useI18n()
const guestTask = stubTask('members.guests')
const editorTask = stubTask('members.roleEditor')
</script>

<template>
  <div class="rp" data-testid="roles-pane">
    <p class="rp-lead">{{ t('shell.members.rolesLead') }}</p>
    <div class="rp-table" role="table">
      <div class="rp-tr rp-head" role="row">
        <span role="columnheader" />
        <span
          v-for="r in ROLES"
          :key="r"
          role="columnheader"
          class="rp-role"
          :class="[`r-${r}`, { mine: r === myRole }]"
        >
          {{ t(`shell.members.${r}`) }}
        </span>
        <span role="columnheader" class="rp-role stub">
          <n-tooltip :delay="300">
            <template #trigger>
              <span>{{ t('shell.members.guest') }}</span>
            </template>
            {{ t('common.stub.soon', { n: guestTask }) }}
          </n-tooltip>
        </span>
      </div>
      <div v-for="row in ROLE_MATRIX" :key="row.key" class="rp-tr" role="row">
        <span role="cell" class="rp-what">{{ t(`shell.members.rights.${row.key}`) }}</span>
        <span v-for="r in ROLES" :key="r" role="cell" class="rp-cell" :class="{ yes: row[r] }">
          <n-icon :component="row[r] ? CheckmarkOutline : RemoveOutline" :size="15" />
        </span>
        <span role="cell" class="rp-cell stub">·</span>
      </div>
    </div>
    <div class="rp-stubs">
      <n-tooltip :delay="300">
        <template #trigger>
          <span class="kit-btn small stub" aria-disabled="true" data-testid="roles-editor-stub">
            {{ t('shell.members.rolesEditor') }}
          </span>
        </template>
        {{ t('common.stub.soon', { n: editorTask }) }}
      </n-tooltip>
      <span class="rp-note">{{ t('shell.members.rolesNote') }}</span>
    </div>
  </div>
</template>

<style scoped>
.rp-lead {
  margin: 0 0 14px;
  font-size: 13px;
  color: var(--t-text2);
}
.rp-table {
  display: flex;
  flex-direction: column;
  border-radius: 14px;
  background: var(--t-sub);
  padding: 6px;
}
.rp-tr {
  display: grid;
  grid-template-columns: minmax(0, 1fr) repeat(4, 92px);
  align-items: center;
  min-height: 36px;
  padding: 0 8px;
  border-radius: 9px;
}
.rp-tr:not(.rp-head):hover {
  background: var(--t-hover);
}
.rp-head {
  min-height: 40px;
}
.rp-role {
  justify-self: center;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 650;
  color: var(--t-text2);
  background: var(--t-pill);
}
.rp-role.r-owner {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, transparent),
    color-mix(in srgb, var(--t-primary) 24%, transparent)
  );
  color: var(--t-text1);
}
.rp-role.r-admin {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #2f80ed 14%, transparent),
    color-mix(in srgb, #2f80ed 24%, transparent)
  );
  color: var(--t-text1);
}
.rp-role.mine {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.rp-what {
  font-size: 13px;
  color: var(--t-text1);
}
.rp-cell {
  justify-self: center;
  display: inline-flex;
  color: var(--t-text3);
  opacity: 0.6;
}
.rp-cell.yes {
  color: var(--t-primary);
  opacity: 1;
}
.stub {
  opacity: 0.55;
  cursor: default;
}
.rp-stubs {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.rp-note {
  font-size: 12px;
  color: var(--t-text3);
}
.lw-mobile .rp-tr {
  grid-template-columns: minmax(0, 1fr) repeat(4, 56px);
}
.lw-mobile .rp-role {
  padding: 3px 6px;
  font-size: 11px;
}
</style>
