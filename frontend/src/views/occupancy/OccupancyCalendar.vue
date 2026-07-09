<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { hasCleaningCalendarExclusion, stayOutcomeLabel, stayOutcomeTone } from './closure'
import { parseMonthKey } from '@/utils/month'
import { nightsBetween, nightsCount } from './status'
import type { Occupancy as Occ } from '@/api/types/occupancy'

const props = defineProps<{
  month: string
  occupancies: Occ[]
}>()

const emit = defineEmits<{
  'update:month': [value: string]
  prev: []
  next: []
  current: []
  'cell-click': [payload: { dateKey: string; stays: Occ[] }]
}>()

type CalendarCell = {
  label: number | ''
  key: string | null
  count: number
  checkIns: number
  closedCount: number
  externalSaleCount: number
  cleaningExcludedCount: number
  stayCount: number
}

const calendarCells = computed<CalendarCell[]>(() => {
  const { year: y, month: m } = parseMonthKey(props.month)
  const first = new Date(y, m - 1, 1)
  const last = new Date(y, m, 0)
  const startPad = (first.getDay() + 6) % 7
  const days: CalendarCell[] = []
  for (let i = 0; i < startPad; i++)
    days.push({ label: '', key: null, count: 0, checkIns: 0, closedCount: 0, externalSaleCount: 0, cleaningExcludedCount: 0, stayCount: 0 })
  for (let d = 1; d <= last.getDate(); d++) {
    const key = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    let count = 0
    let checkIns = 0
    let closedCount = 0
    let externalSaleCount = 0
    let cleaningExcludedCount = 0
    let stayCount = 0
    for (const o of props.occupancies) {
      if (o.status === 'deleted_from_source') continue
      const inNight = nightsBetween(o.start_at, o.end_at).has(key)
      if (inNight) {
        stayCount++
        if (hasCleaningCalendarExclusion(o)) cleaningExcludedCount++
        if (o.closure_state === 'closed') closedCount++
        else if (o.closure_state === 'external_sale') {
          externalSaleCount++
          count++
        } else count++
      }
      if (o.start_at?.slice(0, 10) === key && o.closure_state !== 'closed') checkIns++
    }
    days.push({ label: d, key, count, checkIns, closedCount, externalSaleCount, cleaningExcludedCount, stayCount })
  }
  while (days.length % 7 !== 0)
    days.push({ label: '', key: null, count: 0, checkIns: 0, closedCount: 0, externalSaleCount: 0, cleaningExcludedCount: 0, stayCount: 0 })
  return days
})

function staysOnDay(dateKey: string): Occ[] {
  return props.occupancies.filter((o) => {
    if (o.status === 'deleted_from_source') return false
    return nightsBetween(o.start_at, o.end_at).has(dateKey)
  })
}

function onCellClick(c: CalendarCell) {
  if (!c.key || c.stayCount === 0) return
  emit('cell-click', { dateKey: c.key, stays: staysOnDay(c.key) })
}

function onCellKeydown(e: KeyboardEvent, c: CalendarCell) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    onCellClick(c)
  }
}

const calendarWeeks = computed<CalendarCell[][]>(() => {
  const weeks: CalendarCell[][] = []
  for (let i = 0; i < calendarCells.value.length; i += 7) {
    weeks.push(calendarCells.value.slice(i, i + 7))
  }
  return weeks
})

function cellAriaLabel(c: CalendarCell): string {
  if (c.label === '' || !c.key) return ''
  const parts: string[] = [c.key]
  if (c.count) parts.push(`${c.count} occupied night${c.count > 1 ? 's' : ''}`)
  else parts.push('no occupancy')
  if (c.closedCount) parts.push(`${c.closedCount} closed`)
  if (c.externalSaleCount) parts.push(`${c.externalSaleCount} externally sold`)
  if (c.cleaningExcludedCount) parts.push(`${c.cleaningExcludedCount} with no cleaning event`)
  if (c.checkIns) parts.push(`${c.checkIns} check-in${c.checkIns > 1 ? 's' : ''}`)
  return parts.join(', ')
}

const staysInMonth = computed(() =>
  props.occupancies
    .filter((o) => o.status !== 'deleted_from_source')
    .map((o) => ({
      id: o.id,
      summary: o.raw_summary || 'Stay',
      start: o.start_at?.slice(0, 10),
      end: o.end_at?.slice(0, 10),
      nights: nightsCount(o.start_at, o.end_at),
      status: o.status,
      uid: o.source_event_uid,
      hasPayoutData: !!o.has_payout_data,
      outcome: o.stay_outcome,
      cleaningExcluded: hasCleaningCalendarExclusion(o),
    }))
    .sort((a, b) => a.start.localeCompare(b.start)),
)

const monthNightSummary = computed(() => {
  const { year: y, month: m } = parseMonthKey(props.month)
  const daysInMonth = new Date(y, m, 0).getDate()
  let occupiedNights = 0
  let closedNights = 0
  for (let d = 1; d <= daysInMonth; d++) {
    const key = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    let occupied = false
    let closed = false
    for (const o of props.occupancies) {
      if (o.status === 'deleted_from_source' || o.status === 'cancelled') continue
      if (!nightsBetween(o.start_at, o.end_at).has(key)) continue
      if (o.closure_state === 'closed') closed = true
      else occupied = true // active or external_sale both count as sold per PMS_14 §4
    }
    if (occupied) occupiedNights++
    else if (closed) closedNights++
  }
  // Bookable = calendar days minus closed nights (PMS_14 §4).
  const bookable = Math.max(0, daysInMonth - closedNights)
  const unoccupiedNights = Math.max(0, bookable - occupiedNights)
  return {
    daysInMonth,
    occupiedNights,
    closedNights,
    unoccupiedNights,
    occupancyPct: bookable ? Math.round((occupiedNights / bookable) * 100) : 0,
  }
})
</script>

<template>
  <div class="calendar-stack">
    <UiToolbar>
      <UiButton variant="ghost" aria-label="Previous month" @click="emit('prev')">
        <template #iconLeft><ChevronLeft :size="16" aria-hidden="true" /></template>
      </UiButton>
      <UiInput
        :model-value="month"
        type="month"
        @update:model-value="emit('update:month', String($event))"
      />
      <UiButton variant="ghost" aria-label="Next month" @click="emit('next')">
        <template #iconLeft><ChevronRight :size="16" aria-hidden="true" /></template>
      </UiButton>
      <template #trailing>
        <UiButton variant="secondary" @click="emit('current')">Current month</UiButton>
      </template>
    </UiToolbar>

    <div class="kpi-grid">
      <UiKpiCard
        label="Occupied nights"
        :value="monthNightSummary.occupiedNights"
        :hint="`of ${monthNightSummary.daysInMonth} days`"
      />
      <UiKpiCard
        label="Closed nights"
        :value="monthNightSummary.closedNights"
        tone="warning"
        hint="not bookable"
      />
      <UiKpiCard
        label="Unoccupied nights"
        :value="monthNightSummary.unoccupiedNights"
        tone="default"
      />
      <UiKpiCard
        label="Occupancy"
        :value="`${monthNightSummary.occupancyPct}%`"
        :tone="monthNightSummary.occupancyPct >= 70 ? 'success' : 'default'"
        hint="excludes closed nights"
      />
    </div>

    <UiCard>
      <div
        class="calendar"
        role="grid"
        :aria-label="`Occupancy calendar for ${month}`"
      >
        <div class="calendar__row" role="row">
          <div
            v-for="d in ['Mon','Tue','Wed','Thu','Fri','Sat','Sun']"
            :key="d"
            class="calendar__head"
            role="columnheader"
          >{{ d }}</div>
        </div>
        <div
          v-for="(week, wi) in calendarWeeks"
          :key="wi"
          class="calendar__row"
          role="row"
        >
          <div
            v-for="(c, i) in week"
            :key="i"
            class="calendar__cell"
            :role="c.label === '' ? 'presentation' : c.stayCount ? 'button' : 'gridcell'"
            :tabindex="c.stayCount ? 0 : undefined"
            :aria-label="cellAriaLabel(c) || undefined"
            :class="{
              'calendar__cell--empty': c.label === '',
              'calendar__cell--occupied': !!c.count,
              'calendar__cell--closed': !!c.closedCount && !c.count,
              'calendar__cell--external-sale': !!c.externalSaleCount,
              'calendar__cell--clickable': !!c.stayCount,
            }"
            @click="onCellClick(c)"
            @keydown="onCellKeydown($event, c)"
          >
            <template v-if="c.label !== ''">
              <div class="calendar__day">{{ c.label }}</div>
              <div v-if="c.count" class="calendar__chip" aria-hidden="true">{{ c.count }} night{{ c.count > 1 ? 's' : '' }}</div>
              <div
                v-if="c.closedCount"
                class="calendar__chip calendar__chip--closed"
                aria-hidden="true"
                :title="`${c.closedCount} closed night${c.closedCount > 1 ? 's' : ''}`"
              >closed</div>
              <div
                v-if="c.externalSaleCount"
                class="calendar__chip calendar__chip--external"
                aria-hidden="true"
                :title="`${c.externalSaleCount} externally-sold night${c.externalSaleCount > 1 ? 's' : ''}`"
              >ext. sale</div>
              <div
                v-if="c.cleaningExcludedCount"
                class="calendar__chip calendar__chip--cleaning-excluded"
                aria-hidden="true"
                :title="`${c.cleaningExcludedCount} stay${c.cleaningExcludedCount > 1 ? 's' : ''} with no cleaning event`"
              >No cleaning event</div>
              <div v-if="c.checkIns" class="calendar__chip calendar__chip--checkin" aria-hidden="true">
                {{ c.checkIns }} check-in{{ c.checkIns > 1 ? 's' : '' }}
              </div>
            </template>
          </div>
        </div>
      </div>
      <p class="calendar__note">Cells show nightly occupancy. One stay can span multiple nights.</p>
    </UiCard>

    <UiSection :title="`Stays in ${month}`">
      <UiTable :empty="!staysInMonth.length" empty-text="No stays found for this month.">
        <template #head>
          <tr>
            <th>Check-in</th>
            <th>Check-out</th>
            <th class="num">Nights</th>
            <th>Summary</th>
            <th>Outcome</th>
            <th>Cleaning</th>
            <th>Payout</th>
          </tr>
        </template>
        <tr v-for="s in staysInMonth" :key="s.id">
          <td>{{ s.start }}</td>
          <td>{{ s.end }}</td>
          <td class="num"><strong>{{ s.nights }}</strong></td>
          <td>{{ s.summary }}</td>
          <td>
            <UiBadge v-if="s.outcome" :tone="stayOutcomeTone(s.outcome)">
              {{ stayOutcomeLabel(s.outcome) }}
            </UiBadge>
            <span v-else class="calendar__muted">—</span>
          </td>
          <td>
            <UiBadge v-if="s.cleaningExcluded" tone="warning">No cleaning event</UiBadge>
            <span v-else class="calendar__muted">Cleaning lady: Yes</span>
          </td>
          <td>
            <UiBadge :tone="s.hasPayoutData ? 'success' : 'neutral'" dot>
              {{ s.hasPayoutData ? 'Linked' : 'Pending' }}
            </UiBadge>
          </td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.calendar-stack {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
}
.calendar {
  display: flex;
  flex-direction: column;
  gap: 4px;
  text-align: center;
  font-size: var(--font-size-sm);
}
.calendar__row {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 4px;
}
.calendar__head {
  font-weight: 600;
  color: var(--color-text-muted);
  padding: var(--space-2);
  font-size: var(--font-size-xs);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.calendar__cell {
  min-height: 64px;
  padding: var(--space-2);
  background: var(--color-sunken);
  border-radius: var(--radius-sm);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}
.calendar__cell--empty {
  background: transparent;
}
.calendar__cell--clickable {
  cursor: pointer;
  transition: transform 120ms ease, box-shadow 120ms ease, outline-color 120ms ease;
  outline: 1px solid transparent;
}
.calendar__cell--clickable:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-sm, 0 1px 2px rgba(0, 0, 0, 0.08));
  outline-color: color-mix(in srgb, var(--color-primary) 40%, transparent);
}
.calendar__cell--clickable:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.calendar__cell--occupied {
  background: color-mix(in srgb, var(--color-primary) 12%, transparent);
}
.calendar__cell--closed {
  background: repeating-linear-gradient(
    45deg,
    color-mix(in srgb, var(--color-text-muted) 14%, transparent),
    color-mix(in srgb, var(--color-text-muted) 14%, transparent) 4px,
    transparent 4px,
    transparent 8px
  );
}
.calendar__cell--external-sale {
  background: color-mix(in srgb, var(--warning-fg, #b58400) 14%, transparent);
}
.calendar__day {
  font-weight: 500;
  color: var(--color-text);
}
.calendar__chip {
  font-size: var(--font-size-xs);
  color: var(--color-primary);
}
.calendar__chip--checkin {
  color: var(--success-fg);
}
.calendar__chip--closed {
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
}
.calendar__chip--external {
  color: var(--warning-fg, #b58400);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
}
.calendar__chip--cleaning-excluded {
  color: var(--danger-fg, #b42318);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
}
.calendar__note {
  margin: var(--space-3) 0 0;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.calendar__muted {
  color: var(--color-text-muted);
}
</style>
