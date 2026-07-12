<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { displayStatus, statusTone } from './status'
import {
  canExcludeCleaningCalendar,
  canMarkStayOutcome,
  cleaningCalendarStatusLabel,
  closureLabel,
  closureTone,
  formatExternalAmount,
  hasCleaningCalendarExclusion,
  hasStayOutcome,
  isLabelled,
  stayOutcomeLabel,
  stayOutcomeTone,
} from './closure'
import type { Occupancy as Occ } from '@/api/types/occupancy'

const props = defineProps<{
  month: string
  statusFilter: string
  occupancies: Occ[]
  busy?: boolean
}>()

const emit = defineEmits<{
  'update:month': [value: string]
  'update:statusFilter': [value: string]
  prev: []
  next: []
  refresh: []
  close: [occ: Occ]
  externalSale: [occ: Occ]
  reopen: [occ: Occ]
  markOutcome: [occ: Occ, outcome: 'cancelled_non_refundable' | 'no_show']
  clearOutcome: [occ: Occ]
  excludeCleaningCalendar: [occ: Occ]
  includeCleaningCalendar: [occ: Occ]
}>()

// PMS_19 §8: the default active stay list must not mix in deleted or superseded
// rows. They stay available behind the "Deleted from source" / "Superseded"
// filters for audit.
const visibleOccupancies = computed<Occ[]>(() => {
  const f = props.statusFilter
  return props.occupancies.filter((o) => {
    if (f === 'superseded') return !!o.superseded
    if (f === 'deleted_from_source') return o.status === 'deleted_from_source'
    if (f === 'cancelled') return o.status === 'cancelled'
    if (f === 'active' || f === 'updated') return o.status === f && !o.superseded
    // Default ("Any"): active representations only.
    return o.status !== 'deleted_from_source' && o.status !== 'cancelled' && !o.superseded
  })
})
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
        <option value="superseded">Superseded (audit)</option>
      </UiSelect>
      <template #trailing>
        <UiButton variant="primary" @click="emit('refresh')">Refresh</UiButton>
      </template>
    </UiToolbar>

    <UiTable
      sticky-header
      :empty="!visibleOccupancies.length"
      empty-text="No occupancies found. Configure an ICS URL and run occupancy sync."
    >
      <template #head>
        <tr>
          <th>Start</th>
          <th>End</th>
          <th>Status</th>
          <th>Label</th>
          <th>Outcome</th>
          <th>Cleaning</th>
          <th>Summary</th>
          <th>Payout</th>
          <th>UID</th>
          <th class="actions-col">Actions</th>
        </tr>
      </template>
      <tr v-for="o in visibleOccupancies" :key="o.id" :class="{ 'row-closed': o.closure_state === 'closed' }">
        <td>{{ o.start_at?.slice(0, 10) }}</td>
        <td>{{ o.end_at?.slice(0, 10) }}</td>
        <td>
          <UiBadge :tone="statusTone(o.status)" dot>{{ displayStatus(o.status) }}</UiBadge>
          <UiBadge v-if="o.superseded" tone="neutral">Superseded</UiBadge>
        </td>
        <td>
          <template v-if="isLabelled(o)">
            <UiBadge :tone="closureTone(o.closure_state)">
              {{ closureLabel(o.closure_state) }}
            </UiBadge>
            <span v-if="o.closure_state === 'external_sale'" class="ext-amount">
              {{ formatExternalAmount(o) }}
            </span>
          </template>
          <span v-else class="muted">—</span>
        </td>
        <td>
          <UiBadge v-if="hasStayOutcome(o)" :tone="stayOutcomeTone(o.stay_outcome)">
            {{ stayOutcomeLabel(o.stay_outcome) }}
          </UiBadge>
          <span v-else class="muted">—</span>
        </td>
        <td>
          <UiBadge :tone="hasCleaningCalendarExclusion(o) ? 'warning' : 'success'">
            {{ cleaningCalendarStatusLabel(o) }}
          </UiBadge>
          <div v-if="o.cleaning_calendar_exclusion_reason" class="cleaning-reason">
            {{ o.cleaning_calendar_exclusion_reason }}
          </div>
        </td>
        <td>{{ o.raw_summary || '—' }}</td>
        <td>{{ o.has_payout_data ? 'Yes' : '—' }}</td>
        <td class="uid-cell">{{ o.source_event_uid }}</td>
        <td class="actions-col">
          <template v-if="!isLabelled(o) && !hasStayOutcome(o)">
            <UiButton size="sm" variant="ghost" :disabled="busy" @click="emit('close', o)">
              Close
            </UiButton>
            <UiButton size="sm" variant="ghost" :disabled="busy" @click="emit('externalSale', o)">
              Externally sold
            </UiButton>
            <UiButton
              v-if="canMarkStayOutcome(o)"
              size="sm"
              variant="ghost"
              :disabled="busy"
              @click="emit('markOutcome', o, 'cancelled_non_refundable')"
            >
              Mark non-refundable cancellation
            </UiButton>
            <UiButton
              v-if="canMarkStayOutcome(o)"
              size="sm"
              variant="ghost"
              :disabled="busy"
              @click="emit('markOutcome', o, 'no_show')"
            >
              Mark no-show
            </UiButton>
          </template>
          <UiButton v-else-if="isLabelled(o)" size="sm" variant="ghost" :disabled="busy" @click="emit('reopen', o)">
            Reopen
          </UiButton>
          <UiButton v-else size="sm" variant="ghost" :disabled="busy" @click="emit('clearOutcome', o)">
            Clear outcome
          </UiButton>
          <UiButton
            v-if="canExcludeCleaningCalendar(o)"
            size="sm"
            variant="ghost"
            :disabled="busy"
            @click="emit('excludeCleaningCalendar', o)"
          >
            Do not send cleaning event
          </UiButton>
          <UiButton
            v-else-if="hasCleaningCalendarExclusion(o)"
            size="sm"
            variant="ghost"
            :disabled="busy"
            @click="emit('includeCleaningCalendar', o)"
          >
            Mark as cleaned by cleaning lady
          </UiButton>
        </td>
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
.actions-col {
  white-space: nowrap;
  text-align: right;
}
.ext-amount {
  margin-left: var(--space-2);
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.row-closed {
  opacity: 0.7;
}
.muted {
  color: var(--color-text-muted);
}
.cleaning-reason {
  margin-top: var(--space-1);
  max-width: 18rem;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: normal;
}
</style>
