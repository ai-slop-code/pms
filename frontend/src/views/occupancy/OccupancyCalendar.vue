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
}>()

type CalendarCell = { label: number | ''; key: string | null; count: number; checkIns: number }

const calendarCells = computed<CalendarCell[]>(() => {
  const { year: y, month: m } = parseMonthKey(props.month)
  const first = new Date(y, m - 1, 1)
  const last = new Date(y, m, 0)
  const startPad = (first.getDay() + 6) % 7
  const days: CalendarCell[] = []
  for (let i = 0; i < startPad; i++) days.push({ label: '', key: null, count: 0, checkIns: 0 })
  for (let d = 1; d <= last.getDate(); d++) {
    const key = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    let count = 0
    let checkIns = 0
    for (const o of props.occupancies) {
      if (o.status === 'deleted_from_source') continue
      if (nightsBetween(o.start_at, o.end_at).has(key)) count++
      if (o.start_at?.slice(0, 10) === key) checkIns++
    }
    days.push({ label: d, key, count, checkIns })
  }
  while (days.length % 7 !== 0) days.push({ label: '', key: null, count: 0, checkIns: 0 })
  return days
})

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
    }))
    .sort((a, b) => a.start.localeCompare(b.start)),
)

const monthNightSummary = computed(() => {
  const { year: y, month: m } = parseMonthKey(props.month)
  const daysInMonth = new Date(y, m, 0).getDate()
  let occupiedNights = 0
  for (let d = 1; d <= daysInMonth; d++) {
    const key = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    const occupied = props.occupancies.some((o) => {
      if (o.status === 'deleted_from_source' || o.status === 'cancelled') return false
      return nightsBetween(o.start_at, o.end_at).has(key)
    })
    if (occupied) occupiedNights++
  }
  return {
    daysInMonth,
    occupiedNights,
    unoccupiedNights: Math.max(0, daysInMonth - occupiedNights),
    occupancyPct: daysInMonth ? Math.round((occupiedNights / daysInMonth) * 100) : 0,
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
        label="Unoccupied nights"
        :value="monthNightSummary.unoccupiedNights"
        tone="default"
      />
      <UiKpiCard
        label="Occupancy"
        :value="`${monthNightSummary.occupancyPct}%`"
        :tone="monthNightSummary.occupancyPct >= 70 ? 'success' : 'default'"
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
            :role="c.label === '' ? 'presentation' : 'gridcell'"
            :aria-label="cellAriaLabel(c) || undefined"
            :class="{
              'calendar__cell--empty': c.label === '',
              'calendar__cell--occupied': !!c.count,
            }"
          >
            <template v-if="c.label !== ''">
              <div class="calendar__day">{{ c.label }}</div>
              <div v-if="c.count" class="calendar__chip" aria-hidden="true">{{ c.count }} night{{ c.count > 1 ? 's' : '' }}</div>
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
            <th>Payout</th>
          </tr>
        </template>
        <tr v-for="s in staysInMonth" :key="s.id">
          <td>{{ s.start }}</td>
          <td>{{ s.end }}</td>
          <td class="num"><strong>{{ s.nights }}</strong></td>
          <td>{{ s.summary }}</td>
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
.calendar__cell--occupied {
  background: color-mix(in srgb, var(--color-primary) 12%, transparent);
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
.calendar__note {
  margin: var(--space-3) 0 0;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
</style>
