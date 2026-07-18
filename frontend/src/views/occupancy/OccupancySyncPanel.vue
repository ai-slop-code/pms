<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { displayStatus, statusTone } from './status'
import type { OccupancySyncRun as Run, OccupancyRepairReport } from '@/api/types/occupancy'

function deletionTitle(r: Run): string {
  const parts = [`${r.representations_deleted_from_source ?? 0} rows deleted from source`]
  if (r.named_stays_deleted_from_source) parts.push(`${r.named_stays_deleted_from_source} named`)
  if (r.duplicate_nights_resolved) parts.push(`${r.duplicate_nights_resolved} duplicate nights resolved`)
  return parts.join(' · ')
}

defineProps<{
  source: { active: boolean; source_type: string } | null
  runs: Run[]
  syncing: boolean
  repairBusy: boolean
  repairReport: OccupancyRepairReport | null
}>()

const emit = defineEmits<{
  toggleSource: []
  runSync: []
  repairDryRun: []
  repairApply: []
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
          <span>Source type: <code>{{ source.source_type }}</code></span>
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

    <UiSection
      title="ICS reconciliation repair"
      description="Dry-run duplicate and source-disappearance repairs before applying them. Repair never hard-deletes occupancy rows."
    >
      <UiCard>
        <div class="sync-actions">
          <UiButton variant="secondary" :loading="repairBusy" @click="emit('repairDryRun')">
            Dry-run repair
          </UiButton>
          <UiButton variant="primary" :loading="repairBusy" :disabled="!repairReport" @click="emit('repairApply')">
            Apply repair
          </UiButton>
        </div>
        <div v-if="repairReport" class="repair-report">
          <UiBadge
            :tone="(repairReport.duplicates_resolved || repairReport.rows_deleted_from_source) ? 'warning' : 'success'"
          >
            {{ (repairReport.duplicates_resolved || repairReport.rows_deleted_from_source) ? 'Repair needed' : 'No repair needed' }}
          </UiBadge>
          <span>{{ repairReport.nights_resolved }} nights resolved</span>
          <span>{{ repairReport.duplicates_resolved }} duplicate rows</span>
          <span>{{ repairReport.rows_deleted_from_source ?? 0 }} deleted-from-source rows</span>
        </div>
        <ul v-if="repairReport?.resolutions?.length" class="repair-list">
          <li v-for="r in repairReport.resolutions.slice(0, 5)" :key="`${r.local_night}-${r.winner_occupancy_id}`">
            {{ r.local_night }}: keep #{{ r.winner_occupancy_id }} ({{ r.reason }}), supersede {{ r.loser_occupancy_ids.join(', ') }}
          </li>
        </ul>
      </UiCard>
    </UiSection>

    <UiSection title="Sync history">
      <UiTable :empty="!runs.length" empty-text="No sync runs yet.">
        <template #head>
          <tr>
            <th>Started</th>
            <th>Status</th>
            <th class="num">Events</th>
            <th class="num">Upserted</th>
            <th>Deletion</th>
            <th>Trigger</th>
            <th>Error</th>
          </tr>
        </template>
        <tr v-for="r in runs" :key="r.id">
          <td>{{ r.started_at }}</td>
          <td>
            <UiBadge :tone="statusTone(r.status)" dot>{{ displayStatus(r.status) }}</UiBadge>
            <div v-if="r.status === 'partial_no_mutation'" class="sync-note">No occupancy changes applied.</div>
          </td>
          <td class="num">{{ r.events_seen }}</td>
          <td class="num">{{ r.occupancies_upserted }}</td>
          <td>
            <template v-if="r.deletion_enabled === false">
              <UiBadge tone="neutral">Skipped</UiBadge>
            </template>
            <template v-else>
              <span :title="deletionTitle(r)">
                {{ (r.representations_deleted_from_source ?? 0) }} deleted
              </span>
            </template>
          </td>
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
.repair-report {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  margin-top: var(--space-3);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.repair-list {
  margin: var(--space-3) 0 0;
  padding-left: var(--space-5);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
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
