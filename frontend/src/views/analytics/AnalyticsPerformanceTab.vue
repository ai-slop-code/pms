<script setup lang="ts">
import { computed, defineAsyncComponent, ref } from 'vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import {
  eur, pct, heatCellColor, cancellationBucketLabel, dowLabel as dowLabelFn, monthLabels,
} from './helpers'
import type { HeatmapCell, MonthlyTrendRow, NetPerStayRow, PerformanceResponse } from '@/api/types/analytics'

const UiLineChart = defineAsyncComponent(() => import('@/components/charts/UiLineChart.vue'))

const props = defineProps<{
  performance: PerformanceResponse | null
  perfFrom: string
  perfTo: string
  perfYear: number
  perfYoy: boolean
  weekStartsOn: 'monday' | 'sunday'
}>()

const emit = defineEmits<{
  'update:perfFrom': [v: string]
  'update:perfTo': [v: string]
  'update:perfYear': [v: number]
  'update:perfYoy': [v: boolean]
  apply: []
}>()

const dowLabel = (dow: number) => dowLabelFn(dow, props.weekStartsOn)

const yearlyCleaningMax = computed(() =>
  Math.max(1, ...(props.performance?.yearly_cleaning.series.map((r) => r.count) ?? [0])),
)

const monthlyTrendVisible = computed<MonthlyTrendRow[]>(() => {
  const rows = props.performance?.monthly_trend ?? []
  const firstIdx = rows.findIndex((r) => r.gross_cents > 0 || r.nights_sold > 0)
  if (firstIdx < 0) return []
  return rows.slice(firstIdx)
})

const monthlyTrendLabels = computed(() =>
  monthlyTrendVisible.value.map((r) => {
    const [y, m] = r.month.split('-')
    const idx = Math.max(0, Math.min(11, Number(m) - 1))
    return `${monthLabels[idx]} ${(y ?? '').slice(2)}`
  }),
)

const monthlyTrendSeries = computed(() => [
  {
    label: 'Occupancy',
    data: monthlyTrendVisible.value.map((r) => r.occupancy_rate),
    yAxisId: 'y',
    formatValue: (v: number) => pct(v),
  },
  {
    label: 'ADR',
    data: monthlyTrendVisible.value.map((r) => r.adr_cents / 100),
    yAxisId: 'adr',
    formatValue: (v: number) =>
      v.toLocaleString('en-US', { style: 'currency', currency: 'EUR', maximumFractionDigits: 0 }),
  },
])

const monthlyTrendAxes = [
  { id: 'y', position: 'left' as const, tickFormat: (v: number) => `${Math.round(v * 100)}%`, min: 0, max: 1 },
  { id: 'adr', position: 'right' as const, tickFormat: (v: number) => `€${Math.round(v)}` },
]

const monthlyTrendChartAriaLabel =
  'Monthly occupancy and ADR trend over the visible months. Blue line is occupancy rate, amber line is average daily rate.'

const netRowsFiltered = computed<NetPerStayRow[]>(() => {
  const rows = props.performance?.net_per_stay ?? []
  return rows
    .filter((r) => r.gross_cents > 0 || r.commission_cents > 0 || r.payment_fee_cents > 0)
    .slice()
    .sort((a, b) => (a.start_at || '').localeCompare(b.start_at || ''))
})

const netChartLabels = computed(() =>
  netRowsFiltered.value.map((r) => (r.start_at || '').slice(0, 10)),
)

const netChartSeries = computed(() => [
  {
    label: 'Net per stay',
    data: netRowsFiltered.value.map((r) => r.net_cents / 100),
    formatValue: (v: number) =>
      v.toLocaleString('en-US', { style: 'currency', currency: 'EUR', maximumFractionDigits: 0 }),
  },
])

const netChartAxes = [
  { id: 'y', position: 'left' as const, tickFormat: (v: number) => `€${Math.round(v)}` },
]

const netChartAriaLabel = computed(
  () => `Net per stay line chart across ${netRowsFiltered.value.length} stays.`,
)

const dowRows = computed(() => {
  const rows = props.performance?.dow_occupancy ?? []
  const idx = (dow: number) =>
    props.weekStartsOn === 'monday' ? (dow === 0 ? 6 : dow - 1) : dow
  return [...rows].sort((a, b) => idx(a.dow) - idx(b.dow))
})
const dowMax = computed(() => Math.max(0.01, ...dowRows.value.map((r) => r.occupancy_rate)))

const seasonalityGrid = computed(() => {
  if (!props.performance) return []
  const byYear = new Map<number, HeatmapCell[]>()
  for (const c of props.performance.seasonality_heatmap) {
    if (!byYear.has(c.year)) byYear.set(c.year, [])
    byYear.get(c.year)!.push(c)
  }
  return Array.from(byYear.entries())
    .map(([year, cells]) => ({ year, cells: cells.slice().sort((a, b) => a.week - b.week) }))
    .filter(({ cells }) => cells.some((c) => (c.occupancy_rate || 0) > 0))
    .sort((a, b) => a.year - b.year)
})

const seasonalityWeekAxis = computed<number[]>(() => {
  const first = seasonalityGrid.value[0]
  if (!first) return []
  return first.cells.map((c) => c.week)
})

const heatFocus = ref({ row: 0, col: 0 })

function onHeatKeydown(e: KeyboardEvent) {
  const rows = seasonalityGrid.value.length
  const cols = seasonalityWeekAxis.value.length
  if (!rows || !cols) return
  let { row, col } = heatFocus.value
  switch (e.key) {
    case 'ArrowRight': col = Math.min(cols - 1, col + 1); break
    case 'ArrowLeft': col = Math.max(0, col - 1); break
    case 'ArrowDown': row = Math.min(rows - 1, row + 1); break
    case 'ArrowUp': row = Math.max(0, row - 1); break
    case 'Home': col = 0; break
    case 'End': col = cols - 1; break
    case 'PageUp': row = 0; break
    case 'PageDown': row = rows - 1; break
    default: return
  }
  e.preventDefault()
  heatFocus.value = { row, col }
  const grid = e.currentTarget as HTMLElement
  const next = grid.querySelector<HTMLElement>(`[data-heat-row="${row}"][data-heat-col="${col}"]`)
  next?.focus()
}
</script>

<template>
  <div>
    <UiToolbar>
      <UiInput
        :model-value="perfFrom"
        type="date"
        label="From"
        @update:model-value="emit('update:perfFrom', String($event))"
      />
      <UiInput
        :model-value="perfTo"
        type="date"
        label="To"
        @update:model-value="emit('update:perfTo', String($event))"
      />
      <UiInput
        :model-value="perfYear"
        type="number"
        label="Year"
        @update:model-value="emit('update:perfYear', Number($event))"
      />
      <label class="checkbox-control">
        <input
          type="checkbox"
          :checked="perfYoy"
          @change="emit('update:perfYoy', ($event.target as HTMLInputElement).checked)"
        /> Year-over-year
      </label>
      <template #trailing>
        <UiButton variant="primary" size="md" @click="emit('apply')">Apply</UiButton>
      </template>
    </UiToolbar>

    <div v-if="performance">
      <h2 class="section-heading money">
        <span class="accent" />Money — operating cashflow &amp; revenue
      </h2>

      <h3 class="sub-head">
        Operating cashflow ({{ performance.yearly_finance.year }})
      </h3>
      <div class="kpi-grid">
        <div class="card kpi-card">
          <div class="kpi-head">Incoming</div>
          <strong class="text-success">{{ eur(performance.yearly_finance.incoming_cents) }}</strong>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">Outgoing</div>
          <strong class="text-danger">{{ eur(performance.yearly_finance.outgoing_cents) }}</strong>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">Net</div>
          <strong :class="performance.yearly_finance.net_cents >= 0 ? 'text-success' : 'text-danger'">
            {{ eur(performance.yearly_finance.net_cents) }}
          </strong>
        </div>
      </div>

      <h3 class="block-head">
        Revenue KPIs {{ performance.from }} → {{ performance.to }}
      </h3>
      <div v-if="performance.revenue_as_of" class="as-of-note">
        Revenue as of {{ performance.revenue_as_of }}
      </div>
      <div class="kpi-grid">
        <div class="card kpi-card">
          <div class="kpi-head">Gross</div>
          <strong>{{ eur(performance.kpis.gross_cents) }}</strong>
          <div v-if="performance.prior_kpis" class="kpi-prior">Prior: {{ eur(performance.prior_kpis.gross_cents) }}</div>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">Net</div>
          <strong>{{ eur(performance.kpis.net_cents) }}</strong>
          <div v-if="performance.prior_kpis" class="kpi-prior">Prior: {{ eur(performance.prior_kpis.net_cents) }}</div>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">ADR</div>
          <strong>{{ eur(performance.kpis.adr_cents) }}</strong>
          <div v-if="performance.prior_kpis" class="kpi-prior">Prior: {{ eur(performance.prior_kpis.adr_cents) }}</div>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">RevPAR</div>
          <strong>{{ eur(performance.kpis.revpar_cents) }}</strong>
          <div v-if="performance.prior_kpis" class="kpi-prior">Prior: {{ eur(performance.prior_kpis.revpar_cents) }}</div>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">Take rate</div>
          <strong>{{ pct(performance.kpis.effective_take_rate) }}</strong>
          <div v-if="performance.prior_kpis" class="kpi-prior">Prior: {{ pct(performance.prior_kpis.effective_take_rate) }}</div>
        </div>
      </div>

      <h3 class="block-head">Net per stay</h3>
      <div v-if="netRowsFiltered.length" class="card">
        <p class="caption-muted">
          Stays missing finance data are hidden. X axis shows stay start date; Y axis shows net amount in EUR.
        </p>
        <UiLineChart
          :ariaLabel="netChartAriaLabel"
          :labels="netChartLabels"
          :series="netChartSeries"
          :axes="netChartAxes"
          :height="220"
        />
      </div>
      <p v-else class="card muted-block">No stays with finance data in this window yet.</p>

      <h2 class="section-heading occupancy mt-section">
        <span class="accent" />Occupancy
      </h2>

      <div class="kpi-grid">
        <div class="card kpi-card">
          <div class="kpi-head">Occupancy rate</div>
          <strong>{{ pct(performance.kpis.occupancy_rate) }}</strong>
          <div v-if="performance.prior_kpis" class="kpi-prior">Prior: {{ pct(performance.prior_kpis.occupancy_rate) }}</div>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">Nights sold</div>
          <strong>{{ performance.kpis.nights_sold }}</strong>
        </div>
        <div class="card kpi-card">
          <div class="kpi-head">Available nights</div>
          <strong>{{ performance.kpis.available_nights }}</strong>
        </div>
      </div>

      <h3 class="block-head">Monthly trend (last 24 months)</h3>
      <div class="card">
        <template v-if="monthlyTrendVisible.length >= 2">
          <UiLineChart
            class="monthly-trend-chart"
            :ariaLabel="monthlyTrendChartAriaLabel"
            :labels="monthlyTrendLabels"
            :series="monthlyTrendSeries"
            :axes="monthlyTrendAxes"
            :height="180"
          />
          <div class="chart-legend">
            <span class="chart-swatch chart-swatch--1" aria-hidden="true">●</span> Occupancy &nbsp;
            <span class="chart-swatch chart-swatch--3" aria-hidden="true">●</span> ADR
          </div>
        </template>
        <p v-else class="muted-block">Not enough history for a trend yet.</p>
      </div>

      <h3 class="block-head">Seasonality heatmap (by ISO week)</h3>
      <div class="card chart-scroll">
        <div
          class="heat-grid"
          role="grid"
          aria-label="Seasonality heatmap: occupancy rate by year and ISO week"
          @keydown="onHeatKeydown"
        >
          <div
            v-for="(row, ri) in seasonalityGrid"
            :key="row.year"
            class="heat-row"
            role="row"
          >
            <div class="heat-row__year" role="rowheader">{{ row.year }}</div>
            <div
              v-for="(c, ci) in row.cells" :key="c.week"
              class="heat-cell"
              role="gridcell"
              :tabindex="ri === heatFocus.row && ci === heatFocus.col ? 0 : -1"
              :data-heat-row="ri"
              :data-heat-col="ci"
              :aria-label="`${row.year} week ${c.week}: ${pct(c.occupancy_rate)} occupancy`"
              :title="`W${c.week}: ${pct(c.occupancy_rate)}`"
              :style="{ background: heatCellColor(c.occupancy_rate) }"
              @focus="heatFocus = { row: ri, col: ci }"
            />
          </div>
        </div>
        <div v-if="seasonalityWeekAxis.length" class="heat-axis" aria-hidden="true">
          <div class="heat-axis__label">wk #</div>
          <div
            v-for="(w, idx) in seasonalityWeekAxis"
            :key="w + '-' + idx"
            class="heat-axis__cell"
          >
            {{ w }}
          </div>
        </div>
        <div class="heat-legend">
          <span>0 %</span>
          <span class="heat-legend__bar" />
          <span>100 %</span>
        </div>
      </div>

      <h3 class="block-head">
        Day-of-week occupancy ({{ weekStartsOn === 'monday' ? 'Mon' : 'Sun' }}-first)
      </h3>
      <div class="card dow-card" role="img" aria-label="Day-of-week occupancy bar chart">
        <div v-for="d in dowRows" :key="d.dow" class="dow-col" aria-hidden="true">
          <div class="dow-bar" :style="{ height: `${(d.occupancy_rate / dowMax) * 80}px` }" />
          <div class="dow-label">{{ dowLabel(d.dow) }}</div>
          <div class="dow-value"><strong>{{ pct(d.occupancy_rate) }}</strong></div>
        </div>
        <table class="sr-only">
          <caption>Day-of-week occupancy rate</caption>
          <thead><tr><th scope="col">Day</th><th scope="col">Occupancy</th></tr></thead>
          <tbody>
            <tr v-for="d in dowRows" :key="d.dow">
              <th scope="row">{{ dowLabel(d.dow) }}</th>
              <td>{{ pct(d.occupancy_rate) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h3 class="block-head">Cancellations</h3>
      <div class="card">
        <p>Cancellation rate: <strong>{{ pct(performance.cancellation.rate) }}</strong>
          ({{ performance.cancellation.total_cancelled }} of {{ performance.cancellation.total_active_plus_cancelled }} arrivals)
        </p>
        <div class="pill-row">
          <div
            v-for="b in performance.cancellation.buckets"
            :key="b.bucket"
            class="pill-bucket"
          >
            {{ cancellationBucketLabel(b.bucket) }}: <strong>{{ b.count }}</strong>
          </div>
        </div>
      </div>

      <h2 class="section-heading cleaning mt-section">
        <span class="accent" />Cleaning
      </h2>
      <h3 class="sub-head">
        Cleaning activity ({{ performance.yearly_cleaning.year }})
      </h3>
      <div class="card cleaning-card" role="img" aria-label="Monthly cleaning activity bar chart">
        <div v-for="m in performance.yearly_cleaning.series" :key="m.month" class="cleaning-col" aria-hidden="true">
          <div class="cleaning-bar" :style="{ height: `${(m.count / yearlyCleaningMax) * 80}px` }" />
          <div class="cleaning-label">{{ monthLabels[m.month - 1] }}</div>
          <div class="cleaning-value"><strong>{{ m.count }}</strong></div>
        </div>
        <table class="sr-only">
          <caption>Monthly cleaning activity in {{ performance.yearly_cleaning.year }}</caption>
          <thead><tr><th scope="col">Month</th><th scope="col">Cleanings</th></tr></thead>
          <tbody>
            <tr v-for="m in performance.yearly_cleaning.series" :key="m.month">
              <th scope="row">{{ monthLabels[m.month - 1] }}</th>
              <td>{{ m.count }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}
.checkbox-control {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
  color: var(--color-text);
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
.section-heading .accent {
  display: inline-block;
  width: 4px;
  height: 1.2rem;
  border-radius: 2px;
}
.section-heading.money .accent { background: var(--success-fg); }
.section-heading.occupancy .accent { background: var(--color-primary); }
.section-heading.cleaning .accent { background: var(--warning-fg); }
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
  gap: var(--space-3);
}
.kpi-card { display: flex; flex-direction: column; gap: var(--space-1); }
.kpi-head {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
}
.kpi-prior { font-size: var(--font-size-xs); color: var(--color-text-muted); }
.mt-section { margin-top: var(--space-6); }
.block-head { margin: var(--space-4) 0 var(--space-2); }
.sub-head { margin: var(--space-3) 0 var(--space-2); }
.as-of-note,
.caption-muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  margin-bottom: var(--space-2);
}
.muted-block { color: var(--color-text-muted); margin: 0; }
.text-success { color: var(--success-fg); }
.text-danger { color: var(--danger-fg); }
.chart-scroll { overflow-x: auto; }
.chart-legend {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  margin-top: var(--space-2);
}
.chart-swatch--1 { color: var(--viz-1); }
.chart-swatch--3 { color: var(--viz-3); }
.heat-row { display: flex; gap: 1px; margin-bottom: 2px; align-items: center; }
.heat-row__year {
  width: 3rem;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.heat-cell { width: 12px; height: 18px; }
.heat-cell:focus {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
  z-index: 1;
}
.heat-axis { display: flex; gap: 1px; margin-top: 4px; align-items: center; }
.heat-axis__label {
  width: 3rem;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.heat-axis__cell {
  width: 12px;
  text-align: center;
  font-size: 0.6rem;
  color: var(--color-text-muted);
}
.heat-legend {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-top: var(--space-2);
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.heat-legend__bar {
  display: inline-block;
  width: 140px;
  height: 10px;
  background: linear-gradient(to right, rgb(243, 244, 246), rgb(6, 95, 70));
}
.dow-card, .cleaning-card {
  display: flex;
  gap: var(--space-1);
  align-items: flex-end;
  min-height: 120px;
}
.dow-col, .cleaning-col { flex: 1; text-align: center; }
.dow-bar { background: var(--color-primary); border-radius: var(--radius-sm); min-height: 2px; }
.cleaning-bar { background: var(--success-fg); border-radius: var(--radius-sm); min-height: 2px; }
.dow-label, .cleaning-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-top: var(--space-1);
}
.dow-value, .cleaning-value { font-size: var(--font-size-sm); }
.pill-row { display: flex; gap: var(--space-2); flex-wrap: wrap; }
.pill-bucket {
  background: var(--danger-weak);
  color: var(--danger-fg);
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  font-weight: 500;
}
</style>
