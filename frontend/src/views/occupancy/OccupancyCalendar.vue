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
  cleaningErrorCount: number
}

type CalendarStaySegment = {
  stay: CalendarNamedStay
  startColumn: number
  endColumn: number
  startDate: string
  endDate: string
  lane: number
  continuesBefore: boolean
  continuesAfter: boolean
  sourceWarning: boolean
  nukiError: boolean
  cleaningError: boolean
}

type CalendarWeek = {
  cells: CalendarCell[]
  staySegments: CalendarStaySegment[]
  laneCount: number
}

const stayLaneHeight = 28
const stayBandTop = 30

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

function buildStaySegments(cells: CalendarCell[]) {
  const segments: CalendarStaySegment[] = []
  for (let column = 0; column < cells.length; column++) {
    for (const stay of cells[column]!.namedStays) {
      if (column > 0 && cells[column - 1]!.namedStays.some((candidate) => candidate.id === stay.id)) continue
      let endColumn = column
      while (
        endColumn + 1 < cells.length &&
        cells[endColumn + 1]!.namedStays.some((candidate) => candidate.id === stay.id)
      ) {
        endColumn++
      }
      const startDate = cells[column]!.key
      const endDate = cells[endColumn]!.key
      if (!startDate || !endDate) continue
      segments.push({
        stay,
        startColumn: column + 1,
        endColumn: endColumn + 1,
        startDate,
        endDate,
        lane: 0,
        continuesBefore: stay.check_in_date < startDate,
        continuesAfter: stay.check_out_date > adjacentDate(endDate, 1),
        sourceWarning: stay.source_links.some(
          (link) =>
            link.link_status === 'conflict' ||
            (link.link_status === 'source_deleted' && !stay.has_finance_evidence),
        ),
        nukiError: stay.nuki_generation_status === 'error',
        cleaningError: stay.cleaning_events.some((event) => event.status === 'error'),
      })
    }
  }

  const laneEnds: number[] = []
  for (const segment of segments) {
    let lane = laneEnds.findIndex((endColumn) => endColumn < segment.startColumn)
    if (lane === -1) lane = laneEnds.length
    segment.lane = lane
    laneEnds[lane] = segment.endColumn
  }
  return { staySegments: segments, laneCount: laneEnds.length }
}

const calendarWeeks = computed<CalendarWeek[]>(() => {
  const weeks: CalendarWeek[] = []
  for (let i = 0; i < calendarCells.value.length; i += 7) {
    const cells = calendarCells.value.slice(i, i + 7)
    const { staySegments, laneCount } = props.calendar
      ? buildStaySegments(cells)
      : { staySegments: [], laneCount: 0 }
    weeks.push({ cells, staySegments, laneCount })
  }
  return weeks
})

function cellAriaLabel(c: CalendarCell): string {
  if (c.label === '' || !c.key) return ''
  const parts: string[] = [c.key]
  if (props.calendar) {
    if (c.namedStays.length)
      parts.push(`${c.namedStays.length} named stay${c.namedStays.length > 1 ? 's' : ''}`)
    if (c.rawBlocks.length && !c.namedStays.length)
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

function adjacentDate(dateKey: string, days: number) {
  const date = new Date(`${dateKey}T00:00:00Z`)
  date.setUTCDate(date.getUTCDate() + days)
  return date.toISOString().slice(0, 10)
}

function stayChipLabel(s: CalendarNamedStay) {
  const nights = dateNights(s.check_in_date, s.check_out_date)
  return `${s.display_name} · ${nights} ${nights === 1 ? 'night' : 'nights'}`
}

function stayBandClass(segment: CalendarStaySegment) {
  return {
    [`calendar__stay-band--${segment.stay.stay_type}`]: true,
    'calendar__stay-band--continues-before': segment.continuesBefore,
    'calendar__stay-band--continues-after': segment.continuesAfter,
    'calendar__stay-band--warning': segment.sourceWarning,
    'calendar__stay-band--error': segment.nukiError || segment.cleaningError,
  }
}

function stayBandStyle(segment: CalendarStaySegment) {
  return {
    gridColumn: `${segment.startColumn} / ${segment.endColumn + 1}`,
    gridRow: '1',
    marginTop: `${stayBandTop + segment.lane * stayLaneHeight}px`,
  }
}

function stayBandTitle(segment: CalendarStaySegment) {
  const issues = [
    segment.sourceWarning ? 'raw source issue' : '',
    segment.nukiError ? 'Nuki error' : '',
    segment.cleaningError ? 'cleaning error' : '',
  ].filter(Boolean)
  const continuation = segment.continuesBefore || segment.continuesAfter ? 'continues across week boundary' : ''
  return [
    `${stayChipLabel(segment.stay)} · ${segment.stay.check_in_date} → ${segment.stay.check_out_date}`,
    continuation,
    ...issues,
  ]
    .filter(Boolean)
    .join(' · ')
}

function onStaySegmentClick(week: CalendarWeek, segment: CalendarStaySegment) {
  const cell = week.cells.find((candidate) => candidate.key === segment.startDate)
  if (cell) onCellClick(cell)
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
      const sold = named.some((s) => s.counts_as_sold)
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
        <div v-for="(week, wi) in calendarWeeks" :key="wi" class="calendar__row calendar__week" role="row">
          <div
            v-for="(c, i) in week.cells"
            :key="i"
            class="calendar__cell"
            :style="{ gridColumn: i + 1, gridRow: '1' }"
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
                  class="calendar__band-space"
                  :style="{ height: `${Math.max(1, week.laneCount) * stayLaneHeight}px` }"
                >
                  <div
                    v-if="c.rawBlocks.length && !c.namedStays.length"
                    class="calendar__chip calendar__chip--raw"
                    aria-hidden="true"
                    :title="rawBlockTitle(c.rawBlocks)"
                  >
                    raw{{ c.rawBlocks.length > 1 ? ` ×${c.rawBlocks.length}` : '' }}
                  </div>
                </div>
                <div
                  v-if="c.availabilityBlocks.length"
                  class="calendar__chip calendar__chip--closed"
                  aria-hidden="true"
                >
                  blocked
                </div>
                <div
                  v-if="c.cleaningErrorCount && !c.namedStays.length"
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
          <button
            v-for="segment in week.staySegments"
            :key="`${segment.stay.id}-${segment.startDate}`"
            type="button"
            class="calendar__stay-band"
            :class="stayBandClass(segment)"
            :style="stayBandStyle(segment)"
            :title="stayBandTitle(segment)"
            :aria-label="stayBandTitle(segment)"
            @click.stop="onStaySegmentClick(week, segment)"
          >
            <span v-if="segment.continuesBefore" class="calendar__stay-continuation" aria-hidden="true">‹</span>
            <span class="calendar__stay-label">{{ stayChipLabel(segment.stay) }}</span>
            <span v-if="segment.sourceWarning || segment.nukiError || segment.cleaningError" class="calendar__stay-alert" aria-hidden="true">!</span>
            <span v-if="segment.continuesAfter" class="calendar__stay-continuation" aria-hidden="true">›</span>
          </button>
        </div>
      </div>
      <p class="calendar__note">
        {{
          calendar
            ? 'Raw Booking.com coverage is shown until a named stay covers the night. Connected ribbons represent one continuous stay.'
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
  width: 100%;
  min-width: 0;
  gap: 4px;
  text-align: center;
  font-size: var(--font-size-sm);
}
.calendar__row {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  width: 100%;
  min-width: 0;
  gap: 4px;
}
.calendar__week {
  position: relative;
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
  min-width: 0;
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
.calendar__band-space {
  display: flex;
  flex: 0 0 auto;
  align-items: flex-start;
  justify-content: center;
  width: 100%;
  padding-top: 3px;
  box-sizing: border-box;
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
.calendar__stay-band {
  --stay-color: var(--success-fg);
  --stay-fill: color-mix(in srgb, var(--stay-color) 16%, var(--color-surface, white));
  z-index: 2;
  align-self: start;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  height: 26px;
  min-width: 0;
  margin-inline: 2px;
  padding: 0 var(--space-2);
  overflow: hidden;
  color: var(--stay-color);
  background: var(--stay-fill);
  border: 2px solid color-mix(in srgb, var(--stay-color) 75%, transparent);
  border-radius: var(--radius-sm);
  font-family: inherit;
  font-weight: 700;
  font-size: var(--font-size-xs);
  line-height: 1;
  cursor: pointer;
  transition:
    filter 120ms ease,
    box-shadow 120ms ease;
}
.calendar__stay-band:hover {
  filter: brightness(0.97);
  box-shadow: var(--shadow-sm, 0 1px 2px rgba(0, 0, 0, 0.08));
}
.calendar__stay-band:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.calendar__stay-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.calendar__stay-continuation {
  flex: 0 0 auto;
  font-size: 1rem;
  line-height: 1;
}
.calendar__stay-alert {
  display: inline-grid;
  flex: 0 0 16px;
  width: 16px;
  height: 16px;
  place-items: center;
  color: var(--color-surface, white);
  background: var(--warning-fg, #b58400);
  border-radius: 50%;
  font-size: 10px;
  line-height: 1;
}
.calendar__stay-band--continues-before {
  border-inline-start-style: dashed;
  border-start-start-radius: 0;
  border-end-start-radius: 0;
}
.calendar__stay-band--continues-after {
  border-inline-end-style: dashed;
  border-start-end-radius: 0;
  border-end-end-radius: 0;
}
.calendar__stay-band--warning {
  border-color: var(--warning-fg, #b58400);
}
.calendar__stay-band--error {
  border-color: var(--danger-fg, #b42318);
}
.calendar__stay-band--error .calendar__stay-alert {
  background: var(--danger-fg, #b42318);
}
.calendar__stay-band--maintenance {
  --stay-color: var(--danger-fg, #b42318);
}
.calendar__stay-band--personal_use {
  --stay-color: var(--color-text-muted);
}
.calendar__stay-band--external {
  --stay-color: var(--success-fg);
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
