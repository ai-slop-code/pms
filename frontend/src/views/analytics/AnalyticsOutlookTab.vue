<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { eur, pct, addDaysIso, type UnsoldRange } from './helpers'
import type { OutlookResponse } from '@/api/types/analytics'

const UiLineChart = defineAsyncComponent(() => import('@/components/charts/UiLineChart.vue'))

const props = defineProps<{ outlook: OutlookResponse | null }>()

const paceMax = computed(() =>
  Math.max(1, ...(props.outlook?.pacing_series.map((p) => p.count) ?? [0])),
)

const pacingLabels = computed(() =>
  (props.outlook?.pacing_series ?? []).map((p) => p.date.slice(5)),
)

const pacingSeries = computed(() => [
  {
    label: 'Reservations',
    data: (props.outlook?.pacing_series ?? []).map((p) => p.count),
    formatValue: (v: number) => `${Math.round(v)} reservations`,
  },
])

const pacingChartAriaLabel = computed(
  () =>
    `Booking pace line chart. Cumulative reservations over the next 90 days. Peak value ${paceMax.value} across ${props.outlook?.pacing_series.length ?? 0} points.`,
)

const unsoldRanges = computed<UnsoldRange[]>(() => {
  const nights = [...(props.outlook?.unsold_nights ?? [])].sort((a, b) => a.date.localeCompare(b.date))
  const out: UnsoldRange[] = []
  for (const n of nights) {
    const last = out[out.length - 1]
    const adjacent = last && addDaysIso(last.to, 1) === n.date && last.next_guest === n.next_guest
    if (adjacent) {
      last.to = n.date
      last.nights += 1
    } else {
      out.push({
        from: n.date,
        to: n.date,
        nights: 1,
        prev_guest: n.prev_guest,
        next_guest: n.next_guest,
      })
    }
  }
  return out
})
const unsoldRangesCapped = computed(() => unsoldRanges.value.slice(0, 5))
const unsoldRangesExtra = computed(() => Math.max(0, unsoldRanges.value.length - 5))
</script>

<template>
  <div v-if="outlook">
    <h2 class="section-heading money">Forward-looking KPIs</h2>
    <div v-if="outlook.revenue_as_of" class="as-of-note">
      Revenue as of {{ outlook.revenue_as_of }} · Trailing 90-day ADR {{ eur(outlook.trailing_adr_cents) }}
    </div>
    <div class="kpi-grid">
      <div v-for="w in outlook.windows" :key="w.days" class="card kpi-card">
        <div class="kpi-head">Next {{ w.days }} days</div>
        <div class="kpi-row">
          <span>Nights sold</span><strong>{{ w.nights_sold }} / {{ w.available_nights }}</strong>
        </div>
        <div class="kpi-row">
          <span>Occupancy</span><strong>{{ pct(w.available_nights ? w.nights_sold / w.available_nights : 0) }}</strong>
        </div>
        <div class="kpi-row">
          <span>Confirmed</span><strong>{{ eur(w.confirmed_cents) }}</strong>
        </div>
        <div class="kpi-row">
          <span>Estimated</span><strong class="text-warning">{{ eur(w.estimated_cents) }}</strong>
        </div>
        <div class="kpi-row kpi-row--total">
          <span>Total</span><strong>{{ eur(w.total_revenue_cents) }}</strong>
        </div>
      </div>
    </div>

    <h2 class="section-heading occupancy mt-section">
      Booking pace — cumulative reservations received
    </h2>
    <div class="card">
      <div class="caption-muted">
        Target window: next 90 days. Each point = reservations received up to that booking date.
      </div>
      <div v-if="outlook.pacing_series.length && paceMax > 0">
        <UiLineChart
          class="pacing-chart"
          :ariaLabel="pacingChartAriaLabel"
          :labels="pacingLabels"
          :series="pacingSeries"
          :height="180"
        />
        <div class="chart-legend">
          <span class="chart-swatch chart-swatch--1" aria-hidden="true">●</span> This 90 days
        </div>
      </div>
      <p v-else class="muted-block">Not enough booking activity in this window yet.</p>
    </div>

    <h2 class="section-heading occupancy mt-section">Unsold nights (next 14 days)</h2>
    <div v-if="unsoldRangesCapped.length" class="card card--flush">
      <table>
        <thead>
          <tr><th>From</th><th>To</th><th>Nights</th><th>Previous guest → Next guest</th></tr>
        </thead>
        <tbody>
          <tr v-for="r in unsoldRangesCapped" :key="r.from">
            <td>{{ r.from }}</td>
            <td>{{ r.to }}</td>
            <td>{{ r.nights }}</td>
            <td>{{ r.prev_guest || '—' }} → {{ r.next_guest || '—' }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="unsoldRangesExtra" class="range-more">
        +{{ unsoldRangesExtra }} more ranges…
      </p>
    </div>
    <p v-else class="card happy-note">Fully booked for the next 14 days.</p>
  </div>
</template>

<style scoped>
.card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}
.section-heading {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: var(--space-2) 0 var(--space-3);
  font-size: var(--font-size-h2);
  font-weight: 600;
  color: var(--color-text);
}
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
  gap: var(--space-3);
}
.kpi-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.kpi-head {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
}
.kpi-row {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
.kpi-row--total {
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-2);
  margin-top: var(--space-1);
}
.mt-section { margin-top: var(--space-6); }
.as-of-note,
.caption-muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  margin-bottom: var(--space-2);
}
.muted-block { color: var(--color-text-muted); margin: 0; }
.text-warning { color: var(--warning-fg); }
.happy-note { color: var(--success-fg); }
.card--flush { padding: 0; }
.range-more {
  margin: var(--space-2) var(--space-4);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.chart-legend {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  margin-top: var(--space-2);
}
.chart-swatch--1 { color: var(--viz-1); }
table { width: 100%; border-collapse: collapse; }
th, td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  text-align: left;
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
th {
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-size: var(--font-size-xs);
}
</style>
