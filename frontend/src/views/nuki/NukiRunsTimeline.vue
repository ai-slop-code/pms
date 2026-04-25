<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { formatShortDateTime, isoTitle } from '@/utils/format'
import { displayStatus, statusTone } from './status'
import type { NukiRun } from '@/api/types/nuki'

defineProps<{
  runs: NukiRun[]
  page: number
  hasMore: boolean
  loading: boolean
}>()

const emit = defineEmits<{ prev: []; next: [] }>()

const fmt = formatShortDateTime
</script>

<template>
  <UiSection title="Sync history">
    <UiToolbar>
      <UiButton variant="secondary" size="sm" :disabled="loading || page <= 1" @click="emit('prev')">
        Previous
      </UiButton>
      <span class="muted">Page {{ page }}</span>
      <UiButton variant="secondary" size="sm" :disabled="loading || !hasMore" @click="emit('next')">
        Next
      </UiButton>
    </UiToolbar>

    <UiTable :empty="!runs.length" empty-text="No sync runs yet.">
      <template #head>
        <tr>
          <th>Started</th>
          <th>Trigger</th>
          <th>Status</th>
          <th>Counters</th>
          <th>Error</th>
        </tr>
      </template>
      <tr v-for="r in runs" :key="r.id">
        <td>
          <time :datetime="r.started_at" :title="isoTitle(r.started_at)">{{ fmt(r.started_at) }}</time>
        </td>
        <td>{{ displayStatus(r.trigger) }}</td>
        <td>
          <UiBadge :tone="statusTone(r.status)" dot>{{ displayStatus(r.status) }}</UiBadge>
        </td>
        <td class="muted">
          {{ r.processed_count }} processed · {{ r.created_count }} created ·
          {{ r.updated_count }} updated · {{ r.revoked_count }} revoked ·
          {{ r.failed_count }} failed
        </td>
        <td class="error-cell">{{ r.error_message || '—' }}</td>
      </tr>
    </UiTable>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.error-cell {
  color: var(--danger-fg);
  font-size: var(--font-size-sm);
}
</style>
