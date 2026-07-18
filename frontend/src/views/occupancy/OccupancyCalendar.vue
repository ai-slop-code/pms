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
import { nightsCount, activeNights } from './status'
import type {
  CalendarAvailabilityBlock,
  CalendarNamedStay,
  CalendarRawBookingBlock,
  Occupancy as Occ,
  OccupancyCalendarView,
} from '@/api/types/occupancy'

const props = defineProps<{
  month: string
  occupancies: Occ[]
  calendar?: OccupancyCalendarView | null
}>()

const emit = defineEmits<{
  'update:month': [value: string]
  prev: []
  next: []
  current: []
  'cell-click': [payload: { dateKey: string; stays: Occ[] }]
  'calendar-cell-click': [payload: CalendarCellPayload]
}>()

type CalendarCellPayload = {
  dateKey: string
  rawBlocks: CalendarRawBookingBlock[]
  namedStays: CalendarNamedStay[]
  availabilityBlocks: CalendarAvailabilityBlock[]
}

type CalendarCell = {
  label: number | ''
  key: string | null
  count: number
  checkIns: number
  closedCount: number
  externalSaleCount: number
  cleaningExcludedCount: number
  stayCount: number
  rawBlocks: CalendarRawBookingBlock[]
  namedStays: CalendarNamedStay[]
  availabilityBlocks: CalendarAvailabilityBlock[]
  sourceWarningCount: number
  nukiErrorCount: number
  cleaningErrorCount: number
}

const emptyCell = (): CalendarCell => ({
  label: '',
  key: null,
  count: 0,
  checkIns: 0,
  closedCount: 0,
  externalSaleCount: 0,
  cleaningExcludedCount: 0,
  stayCount: 0,
  rawBlocks: [],
  namedStays: [],
  availabilityBlocks: [],
  sourceWarningCount: 0,
  nukiErrorCount: 0,
  cleaningErrorCount: 0,
})

const calendarCells = computed<CalendarCell[]>(() => {
  const { year: y, month: m } = parseMonthKey(props.month)
  const first = new Date(y, m - 1, 1)
  const last = new Date(y, m, 0)
  const startPad = (first.getDay() + 6) % 7
  const days: CalendarCell[] = []
  for (let i = 0; i < startPad; i++) days.push(emptyCell())
  for (let d = 1; d <= last.getDate(); d++) {
    const key = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    if (props.calendar) {
      const rawBlocks = props.calendar.raw_blocks.filter(
        (b) => b.status === 'active' && b.covered_nights.includes(key),
      )
      const namedStays = props.calendar.named_stays.filter(
        (s) => s.status === 'active' && s.covered_nights.includes(key),
      )
      const availabilityBlocks = props.calendar.availability_blocks.filter(
        (b) => b.status === 'active' && b.covered_nights.includes(key),
      )
      const sourceWarningCount = namedStays.filter((s) =>
        s.source_links.some((l) => l.link_status === 'conflict' || l.link_status === 'source_deleted'),
      ).length
      const nukiErrorCount = namedStays.filter((s) => s.nuki_generation_status === 'error').length
      const cleaningErrorCount = [...rawBlocks, ...namedStays].filter((item) =>
        item.cleaning_events.some((e) => e.status === 'error'),
      ).length
      days.push({
        label: d,
        key,
        count: namedStays.filter((s) => s.stay_type === 'booking_com' || s.stay_type === 'external').length,
        checkIns: namedStays.filter((s) => s.check_in_date === key).length,
        closedCount:
          availabilityBlocks.length +
          namedStays.filter((s) => s.stay_type === 'maintenance' || s.stay_type === 'personal_use').length,
        externalSaleCount: namedStays.filter((s) => s.stay_type === 'external').length,
        cleaningExcludedCount: namedStays.filter((s) => !s.cleaning_required).length,
        stayCount: rawBlocks.length + namedStays.length + availabilityBlocks.length,
        rawBlocks,
        namedStays,
        availabilityBlocks,
        sourceWarningCount,
        nukiErrorCount,
        cleaningErrorCount,
      })
      continue
    }
    let count = 0
    let checkIns = 0
    let closedCount = 0
    let externalSaleCount = 0
    let cleaningExcludedCount = 0
    let stayCount = 0
    for (const o of props.occupancies) {
      if (o.status === 'deleted_from_source' || o.superseded) continue
      const inNight = activeNights(o).has(key)
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
    days.push({
      ...emptyCell(),
      label: d,
      key,
      count,
      checkIns,
      closedCount,
      externalSaleCount,
      cleaningExcludedCount,
      stayCount,
    })
  }
  while (days.length % 7 !== 0) days.push(emptyCell())
  return days
})

function staysOnDay(dateKey: string): Occ[] {
  return props.occupancies.filter((o) => {
    if (o.status === 'deleted_from_source' || o.superseded) return false
    return activeNights(o).has(dateKey)
  })
}

function onCellClick(c: CalendarCell) {
  if (props.calendar) {
    if (!c.key) return
    emit('calendar-cell-click', {
      dateKey: c.key,
      rawBlocks: c.rawBlocks,
      namedStays: c.namedStays,
      availabilityBlocks: c.availabilityBlocks,
    })
    return
  }
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
  if (props.calendar) {
    if (c.namedStays.length)
      parts.push(`${c.namedStays.length} named stay${c.namedStays.length > 1 ? 's' : ''}`)
    if (c.rawBlocks.length)
      parts.push(`${c.rawBlocks.length} raw booking block${c.rawBlocks.length > 1 ? 's' : ''}`)
    if (c.availabilityBlocks.length)
      parts.push(
        `${c.availabilityBlocks.length} availability block${c.availabilityBlocks.length > 1 ? 's' : ''}`,
      )
    if (!c.stayCount) parts.push('empty night')
    return parts.join(', ')
  }
  if (c.count) parts.push(`${c.count} occupied night${c.count > 1 ? 's' : ''}`)
  else parts.push('no occupancy')
  if (c.closedCount) parts.push(`${c.closedCount} closed`)
  if (c.externalSaleCount) parts.push(`${c.externalSaleCount} externally sold`)
  if (c.cleaningExcludedCount) parts.push(`${c.cleaningExcludedCount} with no cleaning event`)
  if (c.checkIns) parts.push(`${c.checkIns} check-in${c.checkIns > 1 ? 's' : ''}`)
  return parts.join(', ')
}

const staysInMonth = computed(() =>
  props.calendar
    ? props.calendar.named_stays
        .filter((s) => s.status !== 'archived')
        .map((s) => ({
          id: s.id,
          summary: s.display_name,
          start: s.check_in_date,
          end: s.check_out_date,
          nights: s.covered_nights.length || dateNights(s.check_in_date, s.check_out_date),
          status: s.status,
          uid: s.source_links
            .map((l) => l.source_event_uid)
            .filter(Boolean)
            .join(', '),
          hasPayoutData: false,
          outcome: null,
          cleaningExcluded: !s.cleaning_required,
          stayType: s.stay_type,
          nukiStatus: s.nuki_generation_status,
        }))
        .sort((a, b) => a.start.localeCompare(b.start))
    : props.occupancies
        .filter((o) => o.status !== 'deleted_from_source' && !o.superseded)
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
          stayType:
            o.closure_state === 'closed'
              ? 'closed'
              : o.closure_state === 'external_sale'
                ? 'external'
                : 'legacy',
          nukiStatus: '',
        }))
        .sort((a, b) => a.start.localeCompare(b.start)),
)

const monthNightSummary = computed(() => {
  const { year: y, month: m } = parseMonthKey(props.month)
  const daysInMonth = new Date(y, m, 0).getDate()
  let occupiedNights = 0
  let closedNights = 0
  let rawOnlyNights = 0
  for (let d = 1; d <= daysInMonth; d++) {
    const key = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    if (props.calendar) {
      const named = props.calendar.named_stays.filter(
        (s) => s.status === 'active' && s.covered_nights.includes(key),
      )
      const unavailable =
        props.calendar.availability_blocks.some(
          (b) => b.status === 'active' && b.covered_nights.includes(key),
        ) || named.some((s) => s.stay_type === 'maintenance' || s.stay_type === 'personal_use')
      const sold = named.some((s) => s.stay_type === 'booking_com' || s.stay_type === 'external')
      if (sold) occupiedNights++
      else if (unavailable) closedNights++
      else if (props.calendar.raw_blocks.some((b) => b.status === 'active' && b.covered_nights.includes(key)))
        rawOnlyNights++
      continue
    }
    let occupied = false
    let closed = false
    for (const o of props.occupancies) {
      if (o.status === 'deleted_from_source' || o.status === 'cancelled' || o.superseded) continue
      if (!activeNights(o).has(key)) continue
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
    rawOnlyNights,
    occupancyPct: bookable ? Math.round((occupiedNights / bookable) * 100) : 0,
  }
})

function dateNights(start: string, end: string) {
  const startMs = Date.parse(`${start}T00:00:00Z`)
  const endMs = Date.parse(`${end}T00:00:00Z`)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) return 0
  return Math.round((endMs - startMs) / 86_400_000)
}

function stayTypeLabel(type: string) {
  return type === 'booking_com'
    ? 'Booking.com'
    : type === 'external'
      ? 'External'
      : type === 'maintenance'
        ? 'Maintenance'
        : type === 'personal_use'
          ? 'Personal use'
          : type
}

function rawBlockTitle(blocks: CalendarRawBookingBlock[]) {
  if (!blocks.length) return ''
  return blocks
    .map((b) => `${b.check_in_date} → ${b.check_out_date} · ${b.raw_summary || b.source_event_uid}`)
    .join('\n')
}
</script>

<template>
  <div class="calendar-stack">
    <UiToolbar>
      <UiButton variant="ghost" aria-label="Previous month" @click="emit('prev')">
        <template #iconLeft><ChevronLeft :size="16" aria-hidden="true" /></template>
      </UiButton>
      <UiInput :model-value="month" type="month" @update:model-value="emit('update:month', String($event))" />
      <UiButton variant="ghost" aria-label="Next month" @click="emit('next')">
        <template #iconLeft><ChevronRight :size="16" aria-hidden="true" /></template>
      </UiButton>
      <template #trailing>
        <UiButton variant="secondary" @click="emit('current')">Current month</UiButton>
      </template>
    </UiToolbar>

    <div class="kpi-grid">
      <UiKpiCard
        :label="calendar ? 'Named guest nights' : 'Occupied nights'"
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
        v-if="calendar"
        :label="calendar ? 'Raw-only nights' : 'Unoccupied nights'"
        :value="calendar ? monthNightSummary.rawOnlyNights : monthNightSummary.unoccupiedNights"
        tone="default"
      />
      <UiKpiCard
        v-if="!calendar"
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
      <div class="calendar" role="grid" :aria-label="`Occupancy calendar for ${month}`">
        <div class="calendar__row" role="row">
          <div
            v-for="d in ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']"
            :key="d"
            class="calendar__head"
            role="columnheader"
          >
            {{ d }}
          </div>
        </div>
        <div v-for="(week, wi) in calendarWeeks" :key="wi" class="calendar__row" role="row">
          <div
            v-for="(c, i) in week"
            :key="i"
            class="calendar__cell"
            :role="c.label === '' ? 'presentation' : calendar || c.stayCount ? 'button' : 'gridcell'"
            :tabindex="calendar && c.label !== '' ? 0 : c.stayCount ? 0 : undefined"
            :aria-label="cellAriaLabel(c) || undefined"
            :class="{
              'calendar__cell--empty': c.label === '',
              'calendar__cell--empty-night': calendar && c.label !== '' && !c.stayCount,
              'calendar__cell--occupied': !!c.count,
              'calendar__cell--closed': !!c.closedCount && !c.count,
              'calendar__cell--external-sale': !!c.externalSaleCount,
              'calendar__cell--raw': calendar && !!c.rawBlocks.length && !c.namedStays.length,
              'calendar__cell--clickable': calendar ? c.label !== '' : !!c.stayCount,
            }"
            @click="onCellClick(c)"
            @keydown="onCellKeydown($event, c)"
          >
            <template v-if="c.label !== ''">
              <div class="calendar__day">{{ c.label }}</div>
              <template v-if="calendar">
                <div
                  v-if="c.rawBlocks.length"
                  class="calendar__chip calendar__chip--raw"
                  aria-hidden="true"
                  :title="rawBlockTitle(c.rawBlocks)"
                >
                  raw{{ c.rawBlocks.length > 1 ? ` ×${c.rawBlocks.length}` : '' }}
                </div>
                <div
                  v-for="s in c.namedStays.slice(0, 2)"
                  :key="s.id"
                  class="calendar__chip calendar__chip--named"
                  :class="`calendar__chip--stay-${s.stay_type}`"
                  aria-hidden="true"
                  :title="`${s.check_in_date} → ${s.check_out_date} · ${stayTypeLabel(s.stay_type)}`"
                >
                  {{ s.display_name }}
                </div>
                <div
                  v-if="c.namedStays.length > 2"
                  class="calendar__chip calendar__chip--named"
                  aria-hidden="true"
                >
                  +{{ c.namedStays.length - 2 }} more
                </div>
                <div
                  v-if="c.availabilityBlocks.length"
                  class="calendar__chip calendar__chip--closed"
                  aria-hidden="true"
                >
                  blocked
                </div>
                <div
                  v-if="c.sourceWarningCount"
                  class="calendar__chip calendar__chip--warning"
                  aria-hidden="true"
                >
                  Raw source issue
                </div>
                <div v-if="c.nukiErrorCount" class="calendar__chip calendar__chip--danger" aria-hidden="true">
                  Nuki error
                </div>
                <div
                  v-if="c.cleaningErrorCount"
                  class="calendar__chip calendar__chip--danger"
                  aria-hidden="true"
                >
                  Cleaning error
                </div>
              </template>
              <template v-else>
                <div v-if="c.count" class="calendar__chip" aria-hidden="true">
                  {{ c.count }} night{{ c.count > 1 ? 's' : '' }}
                </div>
                <div
                  v-if="c.closedCount"
                  class="calendar__chip calendar__chip--closed"
                  aria-hidden="true"
                  :title="`${c.closedCount} closed night${c.closedCount > 1 ? 's' : ''}`"
                >
                  closed
                </div>
                <div
                  v-if="c.externalSaleCount"
                  class="calendar__chip calendar__chip--external"
                  aria-hidden="true"
                  :title="`${c.externalSaleCount} externally-sold night${c.externalSaleCount > 1 ? 's' : ''}`"
                >
                  ext. sale
                </div>
                <div
                  v-if="c.cleaningExcludedCount"
                  class="calendar__chip calendar__chip--cleaning-excluded"
                  aria-hidden="true"
                  :title="`${c.cleaningExcludedCount} stay${c.cleaningExcludedCount > 1 ? 's' : ''} with no cleaning event`"
                >
                  No cleaning event
                </div>
                <div v-if="c.checkIns" class="calendar__chip calendar__chip--checkin" aria-hidden="true">
                  {{ c.checkIns }} check-in{{ c.checkIns > 1 ? 's' : '' }}
                </div>
              </template>
            </template>
          </div>
        </div>
      </div>
      <p class="calendar__note">
        {{
          calendar
            ? 'Cells distinguish raw Booking.com coverage from named stays. Raw-only nights are visible but do not count as occupied.'
            : 'Cells show nightly occupancy. One stay can span multiple nights.'
        }}
      </p>
    </UiCard>

    <UiSection :title="`Stays in ${month}`">
      <UiTable :empty="!staysInMonth.length" empty-text="No stays found for this month.">
        <template #head>
          <tr>
            <th>Check-in</th>
            <th>Check-out</th>
            <th class="num">Nights</th>
            <th>Summary</th>
            <th v-if="calendar">Type</th>
            <th>Outcome</th>
            <th>Cleaning</th>
            <th>{{ calendar ? 'Nuki' : 'Payout' }}</th>
          </tr>
        </template>
        <tr v-for="s in staysInMonth" :key="s.id">
          <td>{{ s.start }}</td>
          <td>{{ s.end }}</td>
          <td class="num">
            <strong>{{ s.nights }}</strong>
          </td>
          <td>{{ s.summary }}</td>
          <td v-if="calendar">{{ stayTypeLabel(s.stayType) }}</td>
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
            <UiBadge
              v-if="calendar"
              :tone="
                s.nukiStatus === 'error' ? 'danger' : s.nukiStatus === 'generated' ? 'success' : 'neutral'
              "
              dot
            >
              {{ s.nukiStatus || 'not_applicable' }}
            </UiBadge>
            <UiBadge v-else :tone="s.hasPayoutData ? 'success' : 'neutral'" dot>
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
.calendar__cell--empty-night {
  background: repeating-linear-gradient(
    135deg,
    color-mix(in srgb, var(--color-text-muted) 12%, transparent),
    color-mix(in srgb, var(--color-text-muted) 12%, transparent) 3px,
    transparent 3px,
    transparent 10px
  );
}
.calendar__cell--clickable {
  cursor: pointer;
  transition:
    transform 120ms ease,
    box-shadow 120ms ease,
    outline-color 120ms ease;
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
.calendar__cell--raw {
  background: color-mix(in srgb, var(--warning-fg, #b58400) 10%, transparent);
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
.calendar__chip--raw {
  color: var(--warning-fg, #b58400);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 700;
}
.calendar__chip--named {
  max-width: 100%;
  overflow: hidden;
  color: var(--success-fg);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.calendar__chip--stay-maintenance {
  color: var(--danger-fg, #b42318);
}
.calendar__chip--stay-personal_use {
  color: var(--color-text-muted);
}
.calendar__chip--stay-external {
  color: var(--success-fg);
}
.calendar__chip--warning {
  color: var(--warning-fg, #b58400);
  font-weight: 700;
}
.calendar__chip--danger {
  color: var(--danger-fg, #b42318);
  font-weight: 700;
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
