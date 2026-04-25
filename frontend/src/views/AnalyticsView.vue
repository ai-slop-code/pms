<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { formatShortDate, isoTitle } from '@/utils/format'
import AnalyticsGlossary from '@/views/analytics/AnalyticsGlossary.vue'
import AnalyticsOutlookTab from '@/views/analytics/AnalyticsOutlookTab.vue'
import AnalyticsPerformanceTab from '@/views/analytics/AnalyticsPerformanceTab.vue'
import AnalyticsDemandTab from '@/views/analytics/AnalyticsDemandTab.vue'
import { freshnessTone, freshnessLabel } from '@/views/analytics/helpers'
import type {
  FreshnessResponse,
  OutlookResponse,
  PerformanceResponse,
  DemandResponse,
  ReturningGuestRow,
  ReturningGuestsResponse,
} from '@/api/types/analytics'

type Tab = 'outlook' | 'performance' | 'demand'

const analyticsTabs: Array<{ id: string; label: string }> = [
  { id: 'outlook', label: 'Outlook' },
  { id: 'performance', label: 'Performance' },
  { id: 'demand', label: 'Demand' },
]

const { pid, currentProperty } = useCurrentProperty()
const weekStartsOn = computed<'monday' | 'sunday'>(() =>
  currentProperty.value?.week_starts_on === 'sunday' ? 'sunday' : 'monday',
)

const tab = ref<Tab>('outlook')
const error = ref('')

const freshness = ref<FreshnessResponse | null>(null)
const outlook = ref<OutlookResponse | null>(null)
const performance = ref<PerformanceResponse | null>(null)
const demand = ref<DemandResponse | null>(null)

const today = new Date()
const thisMonthStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}`
const perfFrom = ref(`${thisMonthStr}-01`)
const perfTo = ref(nextMonthStartIso())
const perfYoy = ref(false)
const perfYear = ref(today.getFullYear())

const demandFrom = ref(`${today.getFullYear() - 1}-${String(today.getMonth() + 1).padStart(2, '0')}-01`)
const demandTo = ref(`${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-01`)

const glossaryOpen = ref(false)

const rgOpen = ref(false)
const rgData = ref<ReturningGuestsResponse | null>(null)
const rgOffset = ref(0)
const rgLimit = 50
const rgTop5 = ref<ReturningGuestRow[]>([])

function nextMonthStartIso(): string {
  const d = new Date(today.getFullYear(), today.getMonth() + 1, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}

async function loadFreshness() {
  if (!pid.value) return
  try {
    freshness.value = await api<FreshnessResponse>(`/api/properties/${pid.value}/analytics/freshness`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function loadOutlook() {
  if (!pid.value) return
  try {
    outlook.value = await api<OutlookResponse>(`/api/properties/${pid.value}/analytics/outlook`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function loadPerformance() {
  if (!pid.value) return
  try {
    const params = new URLSearchParams({
      from: perfFrom.value,
      to: perfTo.value,
      year: String(perfYear.value),
    })
    if (perfYoy.value) params.set('yoy', 'true')
    performance.value = await api<PerformanceResponse>(
      `/api/properties/${pid.value}/analytics/performance?${params.toString()}`,
    )
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function loadDemand() {
  if (!pid.value) return
  try {
    const params = new URLSearchParams({ from: demandFrom.value, to: demandTo.value })
    demand.value = await api<DemandResponse>(
      `/api/properties/${pid.value}/analytics/demand?${params.toString()}`,
    )
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function openReturningGuests() {
  rgOpen.value = true
  rgOffset.value = 0
  await loadReturningGuests()
}

async function loadReturningGuests() {
  if (!pid.value) return
  try {
    const params = new URLSearchParams({
      from: demandFrom.value,
      to: demandTo.value,
      limit: String(rgLimit),
      offset: String(rgOffset.value),
    })
    rgData.value = await api<ReturningGuestsResponse>(
      `/api/properties/${pid.value}/analytics/returning-guests?${params.toString()}`,
    )
  } catch (e) {
    error.value = (e as Error).message
  }
}

function rgNext() {
  if (rgData.value && rgOffset.value + rgLimit < rgData.value.total) {
    rgOffset.value += rgLimit
    loadReturningGuests()
  }
}
function rgPrev() {
  if (rgOffset.value > 0) {
    rgOffset.value = Math.max(0, rgOffset.value - rgLimit)
    loadReturningGuests()
  }
}

async function loadRgTop5() {
  if (!pid.value) {
    rgTop5.value = []
    return
  }
  try {
    const params = new URLSearchParams({
      from: demandFrom.value,
      to: demandTo.value,
      limit: '5',
      offset: '0',
    })
    const r = await api<ReturningGuestsResponse>(
      `/api/properties/${pid.value}/analytics/returning-guests?${params.toString()}`,
    )
    rgTop5.value = r.guests
  } catch {
    rgTop5.value = []
  }
}

onMounted(() => {
  loadFreshness()
  loadOutlook()
})

watch(pid, () => {
  freshness.value = null
  outlook.value = null
  performance.value = null
  demand.value = null
  rgTop5.value = []
  loadFreshness()
  if (tab.value === 'outlook') loadOutlook()
  if (tab.value === 'performance') loadPerformance()
  if (tab.value === 'demand') {
    loadDemand()
    loadRgTop5()
  }
})

watch(tab, (t) => {
  if (t === 'outlook' && !outlook.value) loadOutlook()
  if (t === 'performance' && !performance.value) loadPerformance()
  if (t === 'demand') {
    if (!demand.value) loadDemand()
    loadRgTop5()
  }
})

function applyDemand() {
  loadDemand()
  loadRgTop5()
}
</script>

<template>
  <section class="analytics-page">
    <UiPageHeader
      title="Analytics"
      lede="Forward-looking outlook, historical performance, and demand patterns."
    >
      <template #actions>
        <UiButton
          variant="ghost"
          size="sm"
          :aria-expanded="glossaryOpen"
          @click="glossaryOpen = !glossaryOpen"
        >
          {{ glossaryOpen ? 'Hide glossary' : 'Show glossary' }}
        </UiButton>
      </template>
    </UiPageHeader>

    <UiInlineBanner v-if="error" tone="danger" title="Analytics error">{{ error }}</UiInlineBanner>

    <AnalyticsGlossary :open="glossaryOpen" />

    <div v-if="freshness" class="freshness-card">
      <div class="freshness-stat">
        <span class="freshness-stat__label">Last ICS sync</span>
        <strong>{{ freshness.last_ics_sync_at ? new Date(freshness.last_ics_sync_at).toLocaleString() : '—' }}</strong>
      </div>
      <div class="freshness-stat">
        <span class="freshness-stat__label">Last payout date</span>
        <strong>{{ freshness.last_payout_date || '—' }}</strong>
      </div>
      <div class="freshness-stat">
        <span class="freshness-stat__label">Unmatched payouts</span>
        <strong>{{ freshness.unmatched_payouts_count }}</strong>
      </div>
      <UiBadge
        class="freshness-badge"
        :tone="freshnessTone(freshness.staleness_level)"
        :label="freshnessLabel(freshness.staleness_level)"
        dot
      />
    </div>

    <UiTabs
      :model-value="tab"
      :tabs="analyticsTabs"
      aria-label="Analytics views"
      @update:model-value="(v) => (tab = v as Tab)"
    />

    <AnalyticsOutlookTab v-if="tab === 'outlook'" :outlook="outlook" />

    <AnalyticsPerformanceTab
      v-if="tab === 'performance'"
      :performance="performance"
      :perf-from="perfFrom"
      :perf-to="perfTo"
      :perf-year="perfYear"
      :perf-yoy="perfYoy"
      :week-starts-on="weekStartsOn"
      @update:perf-from="perfFrom = $event"
      @update:perf-to="perfTo = $event"
      @update:perf-year="perfYear = $event"
      @update:perf-yoy="perfYoy = $event"
      @apply="loadPerformance"
    />

    <AnalyticsDemandTab
      v-if="tab === 'demand'"
      :demand="demand"
      :demand-from="demandFrom"
      :demand-to="demandTo"
      :rg-top5="rgTop5"
      :week-starts-on="weekStartsOn"
      @update:demand-from="demandFrom = $event"
      @update:demand-to="demandTo = $event"
      @apply="applyDemand"
      @open-returning="openReturningGuests"
    />

    <UiDialog v-model:open="rgOpen" title="Returning guests" size="lg">
      <div v-if="rgData">
        <table class="rg-table">
          <thead><tr><th>Guest</th><th>Stays</th><th>First</th><th>Last</th></tr></thead>
          <tbody>
            <tr v-for="g in rgData.guests" :key="g.normalized">
              <td>{{ g.display_name }}</td>
              <td>{{ g.stay_count }} {{ g.stay_count === 1 ? 'stay' : 'stays' }}</td>
              <td :title="isoTitle(g.first_stay)">{{ formatShortDate(g.first_stay) }}</td>
              <td :title="isoTitle(g.last_stay)">{{ formatShortDate(g.last_stay) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <template #footer>
        <span v-if="rgData" class="rg-pager-info">
          {{ rgOffset + 1 }}–{{ Math.min(rgOffset + rgLimit, rgData.total) }} of {{ rgData.total }}
        </span>
        <UiButton variant="secondary" size="sm" :disabled="rgOffset === 0" @click="rgPrev">Prev</UiButton>
        <UiButton
          variant="secondary"
          size="sm"
          :disabled="!rgData || rgOffset + rgLimit >= (rgData?.total ?? 0)"
          @click="rgNext"
        >Next</UiButton>
        <UiButton variant="primary" size="sm" @click="rgOpen = false">Close</UiButton>
      </template>
    </UiDialog>
  </section>
</template>

<style scoped>
.analytics-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.freshness-card {
  display: flex;
  gap: var(--space-5);
  flex-wrap: wrap;
  align-items: center;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-4);
}
.freshness-stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.freshness-stat__label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.freshness-badge { margin-left: auto; }
.rg-table { width: 100%; }
.rg-pager-info {
  margin-right: auto;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
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
