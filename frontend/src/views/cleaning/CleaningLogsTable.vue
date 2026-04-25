<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import type { CleaningLogRow } from '@/api/types/cleaning'

defineProps<{ logs: CleaningLogRow[] }>()
</script>

<template>
  <UiSection title="Daily logs">
    <UiEmptyState
      v-if="!logs.length"
      illustration="sparkles"
      title="No cleaning logs yet"
      description="No cleaning logs for this month yet. Arrivals will appear here once they sync."
    />
    <UiTable v-else>
      <template #head>
        <tr>
          <th>Day</th>
          <th>First entry</th>
          <th>Counted</th>
          <th>Nuki reference</th>
        </tr>
      </template>
      <tr v-for="l in logs" :key="l.day_date">
        <td>{{ l.day_date }}</td>
        <td>{{ l.first_entry_at || '—' }}</td>
        <td>
          <UiBadge :tone="l.counted_for_salary ? 'success' : 'neutral'" dot>
            {{ l.counted_for_salary ? 'Counted' : 'Skipped' }}
          </UiBadge>
        </td>
        <td class="muted">{{ l.nuki_event_reference || '—' }}</td>
      </tr>
    </UiTable>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
