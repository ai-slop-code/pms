<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { formatShortDate, isoTitle } from '@/utils/format'
import { displayStatus, statusTone, canGenerate } from './status'
import type { NukiUpcomingStay as UpcomingStay } from '@/api/types/nuki'

defineProps<{
  stays: UpcomingStay[]
  pinNames: Record<number, string>
  savingStayName: Record<number, boolean>
  generatingOccupancyId: number | null
  revealingCodeId: number | null
}>()

const emit = defineEmits<{
  'update:pinName': [occupancyId: number, value: string]
  savePinName: [occupancyId: number]
  generate: [occupancyId: number]
  reveal: [stay: UpcomingStay]
}>()

function onInput(occupancyId: number, event: Event) {
  emit('update:pinName', occupancyId, (event.target as HTMLInputElement).value)
}
</script>

<template>
  <UiSection title="Upcoming stays" description="Name each stay so PINs can be labelled, then generate a guest PIN.">
    <UiTable
      :empty="!stays.length"
      empty-text="No upcoming stays found for the selected property."
    >
      <template #head>
        <tr>
          <th>Stay</th>
          <th>Dates</th>
          <th>Generated PIN</th>
          <th>Status</th>
          <th>Error</th>
          <th aria-label="Actions" />
        </tr>
      </template>
      <tr v-for="s in stays" :key="s.occupancy_id">
        <td>
          <input
            :value="pinNames[s.occupancy_id] || ''"
            :placeholder="s.summary || s.source_event_uid || 'Stay name'"
            class="row-input"
            @input="onInput(s.occupancy_id, $event)"
            @blur="emit('savePinName', s.occupancy_id)"
          />
          <small v-if="savingStayName[s.occupancy_id]" class="muted">Saving…</small>
        </td>
        <td>
          <time :datetime="s.start_at" :title="isoTitle(s.start_at)">{{ formatShortDate(s.start_at) }}</time>
          →
          <time :datetime="s.end_at" :title="isoTitle(s.end_at)">{{ formatShortDate(s.end_at) }}</time>
        </td>
        <td>
          <template v-if="s.generated_status === 'generated' && s.generated_code_id">
            <span v-if="s.generated_masked" class="muted">{{ s.generated_masked }}</span>
            <UiButton
              variant="ghost"
              size="sm"
              :loading="revealingCodeId === s.generated_code_id"
              @click="emit('reveal', s)"
            >Reveal</UiButton>
          </template>
          <template v-else>—</template>
        </td>
        <td>
          <UiBadge :tone="statusTone(s.generated_status || 'not_generated')" dot>
            {{ displayStatus(s.generated_status || 'not_generated') }}
          </UiBadge>
        </td>
        <td class="error-cell">{{ s.generated_error || '—' }}</td>
        <td class="row-actions">
          <UiButton
            v-if="canGenerate(s.generated_status)"
            variant="primary"
            size="sm"
            :loading="generatingOccupancyId === s.occupancy_id"
            @click="emit('generate', s.occupancy_id)"
          >Generate PIN</UiButton>
        </td>
      </tr>
    </UiTable>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.row-input {
  min-width: 12rem;
  min-height: 32px;
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: var(--font-size-sm) / 1.4 var(--font-family-sans);
  color: var(--color-text);
  background: var(--color-surface);
}
.row-input:focus {
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
  outline: none;
}
.row-actions {
  text-align: right;
  white-space: nowrap;
}
.error-cell {
  color: var(--danger-fg);
  font-size: var(--font-size-sm);
}
</style>
