<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { displayStatus, statusTone } from './status'
import type { OccupancySyncRun as Run } from '@/api/types/occupancy'

defineProps<{
  source: { active: boolean; source_type: string } | null
  runs: Run[]
  syncing: boolean
}>()

const emit = defineEmits<{
  toggleSource: []
  runSync: []
}>()
</script>

<template>
  <div>
    <UiSection
      title="ICS source"
      description="Set the Booking.com (or other) calendar URL under Properties → Profile & integrations. Toggle sync below."
    >
      <UiCard>
        <div v-if="source" class="source-row">
          <span
            >Source type: <code>{{ source.source_type }}</code></span
          >
          <UiBadge :tone="source.active ? 'success' : 'warning'" dot>
            {{ source.active ? 'Active' : 'Paused' }}
          </UiBadge>
          <UiButton variant="secondary" size="sm" @click="emit('toggleSource')">
            {{ source.active ? 'Pause sync' : 'Enable sync' }}
          </UiButton>
        </div>
        <div class="sync-actions">
          <UiButton variant="primary" :loading="syncing" @click="emit('runSync')">
            Run occupancy sync
          </UiButton>
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="Sync history">
      <UiTable :empty="!runs.length" empty-text="No sync runs yet.">
        <template #head>
          <tr>
            <th>Started</th>
            <th>Status</th>
            <th class="num">Events seen</th>
            <th class="num">Raw blocks inserted</th>
            <th class="num">Raw blocks updated</th>
            <th class="num">Raw blocks unchanged</th>
            <th class="num">Raw blocks deleted from source</th>
            <th class="num">Raw block conflicts</th>
            <th>Trigger</th>
            <th>Error</th>
          </tr>
        </template>
        <tr v-for="r in runs" :key="r.id">
          <td>{{ r.started_at }}</td>
          <td>
            <UiBadge :tone="statusTone(r.status)" dot>{{ displayStatus(r.status) }}</UiBadge>
            <div v-if="r.status === 'partial_no_mutation'" class="sync-note">
              No raw-block changes applied.
            </div>
          </td>
          <td class="num">{{ r.events_seen }}</td>
          <td class="num">{{ r.raw_blocks_inserted }}</td>
          <td class="num">{{ r.raw_blocks_updated }}</td>
          <td class="num">{{ r.raw_blocks_unchanged }}</td>
          <td class="num">{{ r.raw_blocks_deleted_from_source }}</td>
          <td class="num">{{ r.raw_block_conflicts }}</td>
          <td>{{ displayStatus(r.trigger) }}</td>
          <td class="error-cell">{{ r.error_message || '—' }}</td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.source-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  margin-bottom: var(--space-3);
}
.sync-actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-2);
  flex-wrap: wrap;
}
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--space-3);
}
.error-cell {
  font-size: var(--font-size-xs);
  color: var(--danger-fg);
}
.sync-note {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-top: 2px;
}
</style>
