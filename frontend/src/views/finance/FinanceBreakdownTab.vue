<script setup lang="ts">
import { computed } from 'vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import { formatEuros } from '@/utils/format'
import { VIZ_PALETTE } from './helpers'
import type { FinanceSummary } from '@/api/types/finance'

const props = defineProps<{ summary: FinanceSummary | null }>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)

const outgoingBreakdown = computed(() =>
  (props.summary?.breakdown || [])
    .filter((b) => (b.outgoing_cents || 0) > 0)
    .sort((a, b) => b.outgoing_cents - a.outgoing_cents),
)
const outgoingTotalForPie = computed(() =>
  outgoingBreakdown.value.reduce((acc, b) => acc + (b.outgoing_cents || 0), 0),
)
const outgoingPieSegments = computed(() => {
  const total = outgoingTotalForPie.value
  if (total <= 0) return []
  let acc = 0
  return outgoingBreakdown.value.map((b, idx) => {
    const start = (acc / total) * 100
    acc += b.outgoing_cents
    const end = (acc / total) * 100
    return {
      ...b,
      color: VIZ_PALETTE[idx % VIZ_PALETTE.length],
      start,
      end,
      percent: (b.outgoing_cents / total) * 100,
    }
  })
})
const outgoingPieBackground = computed(() => {
  if (!outgoingPieSegments.value.length)
    return 'conic-gradient(var(--color-border) 0deg, var(--color-border) 360deg)'
  const stops = outgoingPieSegments.value.map((s) => `${s.color} ${s.start}% ${s.end}%`).join(', ')
  return `conic-gradient(${stops})`
})
</script>

<template>
  <UiSection v-if="summary" title="Monthly breakdown" description="Category split for the selected month.">
    <div class="breakdown-split">
      <UiCard>
        <UiTable :empty="!summary.breakdown.length" empty-text="No breakdown data for this month.">
          <template #head>
            <tr>
              <th>Category</th>
              <th class="num">Incoming</th>
              <th class="num">Outgoing</th>
            </tr>
          </template>
          <tr v-for="b in summary.breakdown" :key="`${b.category_id || 'none'}-${b.category_code || ''}`">
            <td>{{ b.category_title || b.category_code || 'Uncategorized' }}</td>
            <td class="num">{{ eur(b.incoming_cents) }}</td>
            <td class="num">{{ eur(b.outgoing_cents) }}</td>
          </tr>
        </UiTable>
      </UiCard>

      <UiCard>
        <h3 class="pie-title">Outgoing mix</h3>
        <div v-if="outgoingPieSegments.length" class="pie-wrap">
          <div
            class="pie-chart"
            role="img"
            aria-label="Donut chart of outgoing categories for the selected month"
            :style="{ background: outgoingPieBackground }"
          />
          <div class="pie-legend">
            <div
              v-for="(item, idx) in outgoingPieSegments"
              :key="`${item.category_code || idx}`"
              class="pie-legend-item"
            >
              <span class="pie-dot" :style="{ background: item.color }" />
              <span class="pie-label">{{ item.category_title || item.category_code || 'Uncategorized' }}</span>
              <span class="pie-value num">
                {{ eur(item.outgoing_cents) }} ({{ item.percent.toFixed(1) }}%)
              </span>
            </div>
          </div>
        </div>
        <p v-else class="muted">No outgoing categories in this month.</p>
      </UiCard>
    </div>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.breakdown-split {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 0.85fr);
  gap: var(--space-4);
  align-items: start;
}
@media (max-width: 1100px) {
  .breakdown-split {
    grid-template-columns: 1fr;
  }
}
.pie-title {
  margin: 0 0 var(--space-3);
  font-size: var(--font-size-md);
  font-weight: 600;
}
.pie-wrap {
  display: flex;
  gap: var(--space-4);
  align-items: center;
  flex-wrap: wrap;
}
.pie-chart {
  width: 156px;
  height: 156px;
  border-radius: 999px;
  border: 1px solid var(--color-border);
  flex: 0 0 auto;
  position: relative;
}
.pie-chart::after {
  content: '';
  position: absolute;
  inset: 33%;
  border-radius: 999px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
}
.pie-legend {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  flex: 1;
}
.pie-legend-item {
  display: grid;
  grid-template-columns: 10px minmax(0, 1fr) auto;
  gap: var(--space-2);
  align-items: center;
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--space-1);
  font-size: var(--font-size-sm);
}
.pie-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}
.pie-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text);
}
.pie-value {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
}
</style>
