<script setup lang="ts">
import { computed } from 'vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { formatShortDate, isoTitle } from '@/utils/format'
import {
  eur, pct, todayIso, leadBucketLabel, losBucketLabel,
  dowLabel as dowLabelFn, weekdayOfIso as weekdayOfIsoFn,
} from './helpers'
import type { DemandResponse, ReturningGuestRow } from '@/api/types/analytics'

const props = defineProps<{
  demand: DemandResponse | null
  demandFrom: string
  demandTo: string
  rgTop5: ReturningGuestRow[]
  weekStartsOn: 'monday' | 'sunday'
}>()

const emit = defineEmits<{
  'update:demandFrom': [v: string]
  'update:demandTo': [v: string]
  apply: []
  openReturning: []
}>()

const dowLabel = (dow: number) => dowLabelFn(dow, props.weekStartsOn)
const weekdayOfIso = (iso: string) => weekdayOfIsoFn(iso, props.weekStartsOn)

const gapNightsFuture = computed(() => {
  const t = todayIso()
  return (props.demand?.gap_nights ?? []).filter((g) => (g.date || '') >= t)
})
const gapNightsPastCount = computed(() => {
  const t = todayIso()
  return (props.demand?.gap_nights ?? []).filter((g) => (g.date || '') < t).length
})
const orphanMidweekFuture = computed(() => {
  const t = todayIso()
  return (props.demand?.orphan_midweek ?? []).filter((g) => (g.date || '') >= t)
})
const orphanMidweekPastCount = computed(() => {
  const t = todayIso()
  return (props.demand?.orphan_midweek ?? []).filter((g) => (g.date || '') < t).length
})

const leadMax = computed(() => Math.max(1, ...(props.demand?.lead_time.map((x) => x.count) ?? [0])))
const losMax = computed(() => Math.max(1, ...(props.demand?.length_of_stay.map((x) => x.count) ?? [0])))
</script>

<template>
  <div>
    <UiToolbar>
      <UiInput
        :model-value="demandFrom"
        type="date"
        label="From"
        @update:model-value="emit('update:demandFrom', String($event))"
      />
      <UiInput
        :model-value="demandTo"
        type="date"
        label="To"
        @update:model-value="emit('update:demandTo', String($event))"
      />
      <template #trailing>
        <UiButton variant="primary" size="md" @click="emit('apply')">Apply</UiButton>
      </template>
    </UiToolbar>

    <div v-if="demand">
      <h2 class="section-heading occupancy">Lead time (days between booking and arrival)</h2>
      <div class="card" role="img" aria-label="Lead time distribution bar chart">
        <div v-for="b in demand.lead_time" :key="b.bucket" class="dim-row" aria-hidden="true">
          <div class="dim-label">{{ leadBucketLabel(b.bucket) }}</div>
          <div class="bar-track">
            <div class="bar-fill bar-fill--primary" :style="{ width: `${(b.count / leadMax) * 100}%` }" />
          </div>
          <div class="dim-count">{{ b.count }}</div>
        </div>
        <table class="sr-only">
          <caption>Reservations by lead-time bucket</caption>
          <thead><tr><th scope="col">Lead time</th><th scope="col">Reservations</th></tr></thead>
          <tbody>
            <tr v-for="b in demand.lead_time" :key="b.bucket">
              <th scope="row">{{ leadBucketLabel(b.bucket) }}</th>
              <td>{{ b.count }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2 class="section-heading occupancy mt-section">Length of stay (nights per reservation)</h2>
      <div class="card" role="img" aria-label="Length of stay distribution bar chart">
        <div v-for="b in demand.length_of_stay" :key="b.bucket" class="dim-row" aria-hidden="true">
          <div class="dim-label">{{ losBucketLabel(b.bucket) }}</div>
          <div class="bar-track">
            <div class="bar-fill bar-fill--success" :style="{ width: `${(b.count / losMax) * 100}%` }" />
          </div>
          <div class="dim-count">{{ b.count }}</div>
        </div>
        <table class="sr-only">
          <caption>Reservations by length-of-stay bucket</caption>
          <thead><tr><th scope="col">Nights</th><th scope="col">Reservations</th></tr></thead>
          <tbody>
            <tr v-for="b in demand.length_of_stay" :key="b.bucket">
              <th scope="row">{{ losBucketLabel(b.bucket) }}</th>
              <td>{{ b.count }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2 class="section-heading money mt-section">ADR by dimension</h2>

      <h3 class="sub-head">ADR by month</h3>
      <div class="card">
        <div v-for="r in demand.adr_by_month" :key="r.bucket" class="adr-row">
          <div class="adr-label adr-label--sm">{{ r.bucket }}</div>
          <div class="adr-meta">{{ r.matched_nights }} nights</div>
          <strong>{{ eur(r.adr_cents) }}</strong>
        </div>
      </div>

      <h3 class="sub-head">ADR by day of week</h3>
      <div class="card">
        <div v-for="r in demand.adr_by_dow" :key="r.bucket" class="adr-row">
          <div class="adr-label adr-label--sm">{{ dowLabel(Number(r.bucket)) }}</div>
          <div class="adr-meta">{{ r.matched_nights }} nights</div>
          <strong>{{ eur(r.adr_cents) }}</strong>
        </div>
      </div>

      <h3 class="sub-head">ADR by lead-time bucket</h3>
      <div class="card">
        <div v-for="r in demand.adr_by_lead_bucket" :key="r.bucket" class="adr-row">
          <div class="adr-label">{{ leadBucketLabel(r.bucket) }}</div>
          <div class="adr-meta">{{ r.matched_nights }} nights</div>
          <strong>{{ eur(r.adr_cents) }}</strong>
        </div>
      </div>

      <h2 class="section-heading occupancy mt-section">
        Gap nights ({{ gapNightsFuture.length }} upcoming<span v-if="gapNightsPastCount">, {{ gapNightsPastCount }} past</span>)
      </h2>
      <div v-if="gapNightsFuture.length" class="card card--flush scroll-260">
        <table>
          <thead><tr><th>Date</th><th>Previous check-out → Next check-in</th></tr></thead>
          <tbody>
            <tr v-for="g in gapNightsFuture" :key="g.date">
              <td>{{ g.date }} <span class="muted-inline">({{ weekdayOfIso(g.date) }})</span></td>
              <td class="muted-inline">
                ← {{ g.prev_checkout_date || '—' }} <span v-if="g.prev_checkout_date">({{ weekdayOfIso(g.prev_checkout_date) }})</span>
                &nbsp;→&nbsp;
                {{ g.next_checkin_date || '—' }} <span v-if="g.next_checkin_date">({{ weekdayOfIso(g.next_checkin_date) }})</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="card happy-note">No upcoming single-night gaps in this window.</p>

      <h2 class="section-heading occupancy mt-section">
        Orphan midweek nights ({{ orphanMidweekFuture.length }} upcoming<span v-if="orphanMidweekPastCount">, {{ orphanMidweekPastCount }} past</span>)
      </h2>
      <div v-if="orphanMidweekFuture.length" class="card card--flush scroll-260">
        <table>
          <thead><tr><th>Date</th><th>Wrapping stays</th></tr></thead>
          <tbody>
            <tr v-for="g in orphanMidweekFuture" :key="g.date + String(g.prev_stay_id)">
              <td>{{ g.date }} <span class="muted-inline">({{ weekdayOfIso(g.date) }})</span></td>
              <td class="muted-inline">
                prev out {{ g.prev_checkout_date || '—' }}
                <span v-if="g.prev_checkout_date">({{ weekdayOfIso(g.prev_checkout_date) }})</span>
                · next in {{ g.next_checkin_date || '—' }}
                <span v-if="g.next_checkin_date">({{ weekdayOfIso(g.next_checkin_date) }})</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="card happy-note">No upcoming orphan midweek gaps.</p>

      <h2 class="section-heading money mt-section">Returning guests</h2>
      <div class="card">
        <p>
          <strong>{{ demand.returning_guests.returning }}</strong>
          of {{ demand.returning_guests.total_active }} unique guests returned
          ({{ pct(demand.returning_guests.returning_rate) }})
        </p>
        <div v-if="rgTop5.length" class="rg-top">
          <div class="rg-top__label">Top 5</div>
          <table class="rg-top__table">
            <thead><tr><th>Guest</th><th>Stays</th><th>First</th><th>Last</th></tr></thead>
            <tbody>
              <tr v-for="g in rgTop5" :key="g.normalized">
                <td>{{ g.display_name }}</td>
                <td>{{ g.stay_count }} {{ g.stay_count === 1 ? 'stay' : 'stays' }}</td>
                <td :title="isoTitle(g.first_stay)">{{ formatShortDate(g.first_stay) }}</td>
                <td :title="isoTitle(g.last_stay)">{{ formatShortDate(g.last_stay) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="rg-action">
          <UiButton variant="secondary" size="sm" @click="emit('openReturning')">
            Show all returning guests
          </UiButton>
        </div>
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
.section-heading {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: var(--space-2) 0 var(--space-3);
  font-size: var(--font-size-h2);
  font-weight: 600;
  color: var(--color-text);
}
.mt-section { margin-top: var(--space-6); }
.sub-head { margin: var(--space-3) 0 var(--space-2); }
.muted-inline { color: var(--color-text-muted); }
.happy-note { color: var(--success-fg); }
.card--flush { padding: 0; }
.scroll-260 { max-height: 260px; overflow-y: auto; }
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
.dim-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-1) 0;
}
.dim-label { width: 7rem; font-size: var(--font-size-sm); }
.dim-count { width: 3rem; text-align: right; }
.bar-track {
  flex: 1;
  background: var(--color-sunken);
  height: 14px;
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.bar-fill { height: 100%; }
.bar-fill--primary { background: var(--color-primary); }
.bar-fill--success { background: var(--success-fg); }
.adr-row {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  padding: var(--space-1) 0;
}
.adr-label { width: 7rem; font-size: var(--font-size-sm); }
.adr-label--sm { width: 5rem; }
.adr-meta {
  flex: 1;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.rg-top { margin-top: var(--space-2); }
.rg-top__label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-bottom: var(--space-1);
}
.rg-top__table { width: auto; min-width: 320px; }
.rg-action { margin-top: var(--space-3); }
</style>
