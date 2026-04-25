<script setup lang="ts">
import { RouterLink } from 'vue-router'
import UiCard from '@/components/ui/UiCard.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { widgetTitle, statusTone } from './status'
import type { DashboardAlert } from './alerts'
import type { SyncStatusWidget } from '@/api/types/dashboard'

defineProps<{
  alerts: DashboardAlert[]
  syncStatus?: SyncStatusWidget
}>()
</script>

<template>
  <UiCard v-if="syncStatus || alerts.length">
    <template #header>
      <div class="widget-head">
        <h2 class="widget-title">Alerts</h2>
      </div>
    </template>
    <ul v-if="alerts.length" class="alert-list">
      <li v-for="alert in alerts" :key="alert.id" class="alert-row">
        <div class="alert-row__body">
          <UiBadge :tone="alert.tone" :label="alert.title" dot />
          <span class="alert-row__detail">{{ alert.detail }}</span>
        </div>
        <RouterLink v-if="alert.to" :to="alert.to" class="widget-link">Open</RouterLink>
      </li>
    </ul>
    <div v-else class="sync-grid">
      <div v-if="syncStatus?.occupancy" class="sync-item">
        <span class="sync-label">Occupancy sync</span>
        <UiBadge
          :tone="statusTone(syncStatus.occupancy)"
          :label="widgetTitle(syncStatus.occupancy)"
          dot
        />
      </div>
      <div v-if="syncStatus?.nuki" class="sync-item">
        <span class="sync-label">Nuki access</span>
        <UiBadge :tone="statusTone(syncStatus.nuki)" :label="widgetTitle(syncStatus.nuki)" dot />
      </div>
      <p v-if="!syncStatus?.occupancy && !syncStatus?.nuki" class="muted">
        All systems healthy.
      </p>
    </div>
  </UiCard>
</template>

<style scoped>
.widget-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}
.widget-title {
  margin: 0;
  font-size: var(--font-size-h4);
  font-weight: 600;
}
.widget-link {
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
}
.alert-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.alert-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) 0;
  border-top: 1px solid var(--color-border);
}
.alert-row:first-child {
  border-top: none;
  padding-top: 0;
}
.alert-row__body {
  display: inline-flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}
.alert-row__detail {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sync-grid {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
}
.sync-item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.sync-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  margin: 0;
}
</style>
