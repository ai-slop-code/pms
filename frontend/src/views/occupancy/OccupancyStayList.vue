<script setup lang="ts">
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { displayStatus, statusTone } from './status'
import type { Occupancy as Occ } from '@/api/types/occupancy'

defineProps<{
  month: string
  statusFilter: string
  occupancies: Occ[]
}>()

const emit = defineEmits<{
  'update:month': [value: string]
  'update:statusFilter': [value: string]
  prev: []
  next: []
  refresh: []
}>()
</script>

<template>
  <div>
    <UiToolbar>
      <UiButton variant="ghost" aria-label="Previous month" @click="emit('prev')">
        <template #iconLeft><ChevronLeft :size="16" aria-hidden="true" /></template>
      </UiButton>
      <UiInput
        :model-value="month"
        type="month"
        @update:model-value="emit('update:month', String($event)); emit('refresh')"
      />
      <UiButton variant="ghost" aria-label="Next month" @click="emit('next')">
        <template #iconLeft><ChevronRight :size="16" aria-hidden="true" /></template>
      </UiButton>
      <UiSelect
        :model-value="statusFilter"
        label="Status"
        @update:model-value="emit('update:statusFilter', String($event)); emit('refresh')"
      >
        <option value="">Any</option>
        <option value="active">Active</option>
        <option value="updated">Updated</option>
        <option value="cancelled">Cancelled</option>
        <option value="deleted_from_source">Deleted from source</option>
      </UiSelect>
      <template #trailing>
        <UiButton variant="primary" @click="emit('refresh')">Refresh</UiButton>
      </template>
    </UiToolbar>

    <UiTable
      sticky-header
      :empty="!occupancies.length"
      empty-text="No occupancies found. Configure an ICS URL and run occupancy sync."
    >
      <template #head>
        <tr>
          <th>Start</th>
          <th>End</th>
          <th>Status</th>
          <th>Summary</th>
          <th>Payout</th>
          <th>UID</th>
        </tr>
      </template>
      <tr v-for="o in occupancies" :key="o.id">
        <td>{{ o.start_at?.slice(0, 10) }}</td>
        <td>{{ o.end_at?.slice(0, 10) }}</td>
        <td>
          <UiBadge :tone="statusTone(o.status)" dot>{{ displayStatus(o.status) }}</UiBadge>
        </td>
        <td>{{ o.raw_summary || '—' }}</td>
        <td>{{ o.has_payout_data ? 'Yes' : '—' }}</td>
        <td class="uid-cell">{{ o.source_event_uid }}</td>
      </tr>
    </UiTable>
  </div>
</template>

<style scoped>
.uid-cell {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  word-break: break-all;
}
</style>
