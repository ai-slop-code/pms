<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import OccupancyCalendar from '@/views/occupancy/OccupancyCalendar.vue'
import OccupancySyncPanel from '@/views/occupancy/OccupancySyncPanel.vue'
import { stayOutcomeLabel, stayOutcomeTone } from '@/views/occupancy/outcome'
import { monthKey, parseMonthKey } from '@/utils/month'
import type {
  CalendarAvailabilityBlock,
  CalendarNamedStay,
  CalendarRawBookingBlock,
  OccupancyCalendarView,
  OccupancySyncRun,
  StayOutcome,
} from '@/api/types/occupancy'

const { pid } = useCurrentProperty()
const tab = ref<'calendar' | 'sync'>('calendar')
const tabs = [
  { id: 'calendar', label: 'Calendar' },
  { id: 'sync', label: 'Sync & export' },
]
const month = ref(monthKey(new Date()))
const calendar = ref<OccupancyCalendarView | null>(null)
const runs = ref<OccupancySyncRun[]>([])
const source = ref<{ active: boolean; source_type: string } | null>(null)
const syncing = ref(false)
const error = ref('')
const success = ref('')

const dayDialogOpen = ref(false)
const dayDialogDate = ref('')
const dayRawBlocks = ref<CalendarRawBookingBlock[]>([])
const dayNamedStays = ref<CalendarNamedStay[]>([])
const dayAvailabilityBlocks = ref<CalendarAvailabilityBlock[]>([])
const dayHasAssignedNight = computed(
  () => dayNamedStays.value.length > 0 || dayAvailabilityBlocks.value.length > 0,
)
const promotableRawBlocks = computed(() => (dayHasAssignedNight.value ? [] : dayRawBlocks.value))

const promoteDialogOpen = ref(false)
const promoteBusy = ref(false)
const promoteError = ref('')
const promoteRawBlock = ref<CalendarRawBookingBlock | null>(null)
const promoteCheckIn = ref('')
const promoteCheckOut = ref('')
const promoteDisplayName = ref('')
const promoteStayType = ref('booking_com')
const promoteCleaningRequired = ref(true)
const promoteCleaningManuallyChanged = ref(false)

const manualStayDialogOpen = ref(false)
const manualStayBusy = ref(false)
const manualStayError = ref('')
const manualStayDisplayName = ref('')
const manualStayType = ref('external')
const manualStayCheckIn = ref('')
const manualStayCheckOut = ref('')
const manualStayCleaningRequired = ref(true)
const manualStayCleaningManuallyChanged = ref(false)

const availabilityDialogOpen = ref(false)
const availabilityBusy = ref(false)
const availabilityError = ref('')
const availabilityEditingID = ref<number | null>(null)
const availabilityBlockType = ref('closed')
const availabilityStart = ref('')
const availabilityEnd = ref('')
const availabilityReason = ref('')
const availabilityStatus = ref<'active' | 'archived'>('active')
const availabilityStatusBusyID = ref<number | null>(null)

const editStayDialogOpen = ref(false)
const editStayBusy = ref(false)
const editStayError = ref('')
const editStayTarget = ref<CalendarNamedStay | null>(null)
const editStayDisplayName = ref('')
const editStayType = ref('external')
const editStayCheckIn = ref('')
const editStayCheckOut = ref('')
const editStayCleaningRequired = ref(true)
const stayStatusBusyID = ref<number | null>(null)

const outcomeDialogOpen = ref(false)
const outcomeBusy = ref(false)
const outcomeError = ref('')
const outcomeTarget = ref<CalendarNamedStay | null>(null)
const outcomeValue = ref<StayOutcome>('cancelled_non_refundable')
const outcomeReason = ref('')
const outcomeTitle = computed(() =>
  outcomeValue.value === 'cancelled_non_refundable' ? 'Mark non-refundable cancellation' : 'Mark no-show',
)

const reviewDialogOpen = ref(false)
const reviewBusy = ref(false)
const reviewError = ref('')
const reviewTarget = ref<CalendarNamedStay | null>(null)
const reviewStatus = ref<'confirmed' | 'rejected'>('confirmed')
const reviewReason = ref('')

function addISODate(value: string, days: number) {
  const [year, monthNumber, day] = value.split('-').map(Number)
  if (!year || !monthNumber || !day) return ''
  return new Date(Date.UTC(year, monthNumber - 1, day + days)).toISOString().slice(0, 10)
}

function isISODate(value: string) {
  return /^\d{4}-\d{2}-\d{2}$/.test(value) && addISODate(value, 0) === value
}

function defaultCleaningRequired(stayType: string) {
  return stayType === 'booking_com' || stayType === 'external'
}

function prevMonth() {
  const parsed = parseMonthKey(month.value)
  month.value = monthKey(new Date(parsed.year, parsed.month - 2, 1))
}

function nextMonth() {
  const parsed = parseMonthKey(month.value)
  month.value = monthKey(new Date(parsed.year, parsed.month, 1))
}

function onCalendarCellClick(payload: {
  dateKey: string
  rawBlocks: CalendarRawBookingBlock[]
  namedStays: CalendarNamedStay[]
  availabilityBlocks: CalendarAvailabilityBlock[]
}) {
  dayDialogDate.value = payload.dateKey
  dayRawBlocks.value = payload.rawBlocks
  dayNamedStays.value = payload.namedStays
  dayAvailabilityBlocks.value = payload.availabilityBlocks
  dayDialogOpen.value = true
}

function openPromoteDialog(block: CalendarRawBookingBlock) {
  promoteRawBlock.value = block
  const clicked = dayDialogDate.value
  promoteCheckIn.value =
    clicked >= block.check_in_date && clicked < block.check_out_date ? clicked : block.check_in_date
  promoteCheckOut.value = addISODate(promoteCheckIn.value, 1)
  if (promoteCheckOut.value > block.check_out_date) promoteCheckOut.value = block.check_out_date
  promoteDisplayName.value = ''
  promoteStayType.value = 'booking_com'
  promoteCleaningRequired.value = true
  promoteCleaningManuallyChanged.value = false
  promoteError.value = ''
  dayDialogOpen.value = false
  promoteDialogOpen.value = true
}

function openManualStayDialog(dateKey = dayDialogDate.value) {
  manualStayDisplayName.value = ''
  manualStayType.value = 'external'
  manualStayCheckIn.value = dateKey || `${month.value}-01`
  manualStayCheckOut.value = addISODate(manualStayCheckIn.value, 1)
  manualStayCleaningRequired.value = true
  manualStayCleaningManuallyChanged.value = false
  manualStayError.value = ''
  dayDialogOpen.value = false
  manualStayDialogOpen.value = true
}

watch(promoteStayType, (stayType) => {
  if (!promoteCleaningManuallyChanged.value) {
    promoteCleaningRequired.value = defaultCleaningRequired(stayType)
  }
})

watch(manualStayType, (stayType) => {
  if (!manualStayCleaningManuallyChanged.value) {
    manualStayCleaningRequired.value = defaultCleaningRequired(stayType)
  }
})

async function submitManualStay() {
  if (!pid.value) return
  const name = manualStayDisplayName.value.trim()
  if (!name) {
    manualStayError.value = 'Enter a stay name.'
    return
  }
  if (
    !isISODate(manualStayCheckIn.value) ||
    !isISODate(manualStayCheckOut.value) ||
    manualStayCheckOut.value <= manualStayCheckIn.value
  ) {
    manualStayError.value = 'Choose a valid check-in and later check-out.'
    return
  }
  manualStayBusy.value = true
  manualStayError.value = ''
  try {
    const result = await api<{
      ok?: boolean
      stay_saved?: boolean
      error?: string
      nuki_generation_status?: string
      cleaning_calendar?: { status: string; error?: string }
    }>(
      `/api/properties/${pid.value}/stays`,
      {
        method: 'POST',
        json: {
          display_name: name,
          check_in: manualStayCheckIn.value,
          check_out: manualStayCheckOut.value,
          stay_type: manualStayType.value,
          cleaning_required: manualStayCleaningRequired.value,
        },
      },
    )
    if (result.stay_saved && result.cleaning_calendar?.status === 'error') {
      manualStayDialogOpen.value = false
      error.value = result.error || 'Stay saved, but cleaning calendar sync failed. Retry calendar synchronization.'
      await loadCalendar()
      return
    }
    if (result.ok === false) throw new Error(result.error || 'Failed to create stay')
    manualStayDialogOpen.value = false
    success.value = `Named stay “${name}” saved.`
    await loadCalendar()
  } catch (cause) {
    manualStayError.value = cause instanceof Error ? cause.message : 'Failed to create stay'
  } finally {
    manualStayBusy.value = false
  }
}

async function submitPromoteRawBlock() {
  if (!pid.value || !promoteRawBlock.value) return
  const name = promoteDisplayName.value.trim()
  if (!name) {
    promoteError.value = 'Enter a guest / stay name.'
    return
  }
  if (
    !isISODate(promoteCheckIn.value) ||
    !isISODate(promoteCheckOut.value) ||
    promoteCheckOut.value <= promoteCheckIn.value
  ) {
    promoteError.value = 'Choose a valid check-in and later check-out.'
    return
  }
  if (
    promoteCheckIn.value < promoteRawBlock.value.check_in_date ||
    promoteCheckOut.value > promoteRawBlock.value.check_out_date
  ) {
    promoteError.value = `Stay must stay within ${promoteRawBlock.value.check_in_date} → ${promoteRawBlock.value.check_out_date}.`
    return
  }
  promoteBusy.value = true
  promoteError.value = ''
  try {
    const result = await api<{
      ok?: boolean
      stay_saved?: boolean
      error?: string
      nuki_generation_status?: string
      cleaning_calendar?: { status: string; error?: string }
    }>(
      `/api/properties/${pid.value}/booking-blocks/${promoteRawBlock.value.id}/promote`,
      {
        method: 'POST',
        json: {
          display_name: name,
          check_in: promoteCheckIn.value,
          check_out: promoteCheckOut.value,
          stay_type: promoteStayType.value,
          cleaning_required: promoteCleaningRequired.value,
        },
      },
    )
    if (result.stay_saved && result.cleaning_calendar?.status === 'error') {
      promoteDialogOpen.value = false
      error.value = result.error || 'Stay saved, but cleaning calendar sync failed. Retry calendar synchronization.'
      await loadCalendar()
      return
    }
    if (result.ok === false) throw new Error(result.error || 'Failed to promote raw block')
    promoteDialogOpen.value = false
    success.value = `Named stay “${name}” saved.`
    await loadCalendar()
  } catch (cause) {
    promoteError.value = cause instanceof Error ? cause.message : 'Failed to promote raw block'
  } finally {
    promoteBusy.value = false
  }
}

function openAvailabilityDialog(block?: CalendarAvailabilityBlock) {
  availabilityEditingID.value = block?.id ?? null
  availabilityBlockType.value = block?.block_type || 'closed'
  availabilityStart.value = block?.start_date || dayDialogDate.value || `${month.value}-01`
  availabilityEnd.value = block?.end_date || addISODate(availabilityStart.value, 1)
  availabilityReason.value = block?.reason || ''
  availabilityStatus.value = block?.status === 'archived' ? 'archived' : 'active'
  availabilityError.value = ''
  dayDialogOpen.value = false
  availabilityDialogOpen.value = true
}

async function submitAvailabilityBlock() {
  if (!pid.value) return
  if (
    !isISODate(availabilityStart.value) ||
    !isISODate(availabilityEnd.value) ||
    availabilityEnd.value <= availabilityStart.value
  ) {
    availabilityError.value = 'Choose a valid start and later end date.'
    return
  }
  availabilityBusy.value = true
  availabilityError.value = ''
  try {
    const path = availabilityEditingID.value
      ? `/api/properties/${pid.value}/availability-blocks/${availabilityEditingID.value}`
      : `/api/properties/${pid.value}/availability-blocks`
    const result = await api<{ ok?: boolean; error?: string }>(path, {
      method: availabilityEditingID.value ? 'PATCH' : 'POST',
      json: {
        block_type: availabilityBlockType.value,
        start_date: availabilityStart.value,
        end_date: availabilityEnd.value,
        reason: availabilityReason.value.trim(),
        ...(availabilityEditingID.value ? { status: availabilityStatus.value } : {}),
      },
    })
    if (result.ok === false) throw new Error(result.error || 'Failed to save availability block')
    availabilityDialogOpen.value = false
    success.value = availabilityEditingID.value
      ? 'Availability block updated.'
      : 'Availability block created.'
    await loadCalendar()
  } catch (cause) {
    availabilityError.value = cause instanceof Error ? cause.message : 'Failed to save availability block'
  } finally {
    availabilityBusy.value = false
  }
}

async function updateAvailabilityBlockStatus(
  block: CalendarAvailabilityBlock,
  status: 'active' | 'archived',
) {
  if (!pid.value) return
  availabilityStatusBusyID.value = block.id
  error.value = ''
  try {
    const result = await api<{ ok?: boolean; error?: string }>(
      `/api/properties/${pid.value}/availability-blocks/${block.id}`,
      {
        method: 'PATCH',
        json: {
          block_type: block.block_type,
          start_date: block.start_date,
          end_date: block.end_date,
          reason: block.reason || '',
          status,
        },
      },
    )
    if (result.ok === false) throw new Error(result.error || 'Failed to update availability block status')
    success.value = status === 'active' ? 'Availability block reactivated.' : 'Availability block archived.'
    dayDialogOpen.value = false
    await loadCalendar()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Failed to update availability block status'
  } finally {
    availabilityStatusBusyID.value = null
  }
}

function openEditStayDialog(stay: CalendarNamedStay) {
  editStayTarget.value = stay
  editStayDisplayName.value = stay.display_name
  editStayType.value = stay.stay_type
  editStayCheckIn.value = stay.check_in_date
  editStayCheckOut.value = stay.check_out_date
  editStayCleaningRequired.value = stay.cleaning_required
  editStayError.value = ''
  dayDialogOpen.value = false
  editStayDialogOpen.value = true
}

async function submitEditStay() {
  if (!pid.value || !editStayTarget.value) return
  const name = editStayDisplayName.value.trim()
  if (!name) {
    editStayError.value = 'Enter a stay name.'
    return
  }
  if (
    !isISODate(editStayCheckIn.value) ||
    !isISODate(editStayCheckOut.value) ||
    editStayCheckOut.value <= editStayCheckIn.value
  ) {
    editStayError.value = 'Choose a valid check-in and later check-out.'
    return
  }
  editStayBusy.value = true
  editStayError.value = ''
  try {
    const result = await api<{ ok?: boolean; error?: string }>(
      `/api/properties/${pid.value}/stays/${editStayTarget.value.id}`,
      {
        method: 'PATCH',
        json: {
          display_name: name,
          check_in: editStayCheckIn.value,
          check_out: editStayCheckOut.value,
          stay_type: editStayType.value,
          cleaning_required: editStayCleaningRequired.value,
        },
      },
    )
    if (result.ok === false) throw new Error(result.error || 'Failed to update stay')
    editStayDialogOpen.value = false
    success.value = `Named stay “${name}” updated.`
    await loadCalendar()
  } catch (cause) {
    editStayError.value = cause instanceof Error ? cause.message : 'Failed to update stay'
  } finally {
    editStayBusy.value = false
  }
}

async function updateNamedStayStatus(stay: CalendarNamedStay, status: 'active' | 'cancelled' | 'archived') {
  if (!pid.value) return
  stayStatusBusyID.value = stay.id
  error.value = ''
  try {
    const result = await api<{ ok?: boolean; error?: string }>(
      `/api/properties/${pid.value}/stays/${stay.id}/status`,
      { method: 'PATCH', json: { status } },
    )
    if (result.ok === false) throw new Error(result.error || 'Failed to update stay status')
    success.value = status === 'active' ? 'Named stay reactivated.' : `Named stay ${status}.`
    dayDialogOpen.value = false
    await loadCalendar()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Failed to update stay status'
  } finally {
    stayStatusBusyID.value = null
  }
}

function openOutcomeDialog(stay: CalendarNamedStay, outcome: StayOutcome) {
  outcomeTarget.value = stay
  outcomeValue.value = outcome
  outcomeReason.value = ''
  outcomeError.value = ''
  dayDialogOpen.value = false
  outcomeDialogOpen.value = true
}

async function patchOutcome(stay: CalendarNamedStay, outcome: StayOutcome | null, reason = '') {
  if (!pid.value) return
  outcomeBusy.value = true
  outcomeError.value = ''
  try {
    const result = await api<{ ok?: boolean; error?: string }>(
      `/api/properties/${pid.value}/stays/${stay.id}/outcome`,
      {
        method: 'PATCH',
        json: { outcome, ...(reason ? { reason } : {}) },
      },
    )
    if (result.ok === false) throw new Error(result.error || 'Failed to update outcome')
    outcomeDialogOpen.value = false
    success.value = outcome ? `${stayOutcomeLabel(outcome)} marked.` : 'Stay outcome cleared.'
    await loadCalendar()
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : 'Failed to update outcome'
    if (outcomeDialogOpen.value) outcomeError.value = message
    else error.value = message
  } finally {
    outcomeBusy.value = false
  }
}

async function submitOutcome() {
  if (!outcomeTarget.value) return
  await patchOutcome(outcomeTarget.value, outcomeValue.value, outcomeReason.value.trim())
}

function openReviewDialog(stay: CalendarNamedStay, status: 'confirmed' | 'rejected') {
  reviewTarget.value = stay
  reviewStatus.value = status
  reviewReason.value = ''
  reviewError.value = ''
  dayDialogOpen.value = false
  reviewDialogOpen.value = true
}

async function submitReview() {
  if (!pid.value || !reviewTarget.value) return
  reviewBusy.value = true
  reviewError.value = ''
  try {
    const reason = reviewReason.value.trim()
    const result = await api<{ ok?: boolean; error?: string }>(
      `/api/properties/${pid.value}/stays/${reviewTarget.value.id}/review`,
      {
        method: 'PATCH',
        json: { review_status: reviewStatus.value, ...(reason ? { reason } : {}) },
      },
    )
    if (result.ok === false) throw new Error(result.error || 'Failed to review stay')
    reviewDialogOpen.value = false
    success.value = reviewStatus.value === 'confirmed' ? 'Stay confirmed.' : 'Stay rejected.'
    await loadCalendar()
  } catch (cause) {
    reviewError.value = cause instanceof Error ? cause.message : 'Failed to review stay'
  } finally {
    reviewBusy.value = false
  }
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

function cleaningSummary(events: { status: string }[]) {
  if (!events.length) return 'Cleaning: not generated'
  if (events.some((event) => event.status === 'error')) return 'Cleaning: error'
  if (events.some((event) => event.status === 'synced')) return 'Cleaning: synced'
  if (events.some((event) => event.status === 'pending')) return 'Cleaning: pending'
  return 'Cleaning: tracked'
}

function actionableSourceLinks(stay: CalendarNamedStay) {
  return stay.source_links.filter(
    (link) =>
      link.link_status === 'conflict' ||
      (link.link_status === 'source_deleted' && !stay.has_finance_evidence),
  )
}

function hasFinanceConfirmedMissingSource(stay: CalendarNamedStay) {
  return stay.has_finance_evidence && stay.source_links.some((link) => link.link_status === 'source_deleted')
}

async function loadCalendar() {
  if (!pid.value) return
  error.value = ''
  try {
    const result = await api<{ calendar: OccupancyCalendarView }>(
      `/api/properties/${pid.value}/occupancy-calendar?month=${encodeURIComponent(month.value)}`,
    )
    calendar.value = result.calendar
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Failed to load occupancy calendar'
  }
}

async function loadSyncPanel() {
  if (!pid.value) return
  error.value = ''
  try {
    const [runResult, sourceResult] = await Promise.all([
      api<{ runs: OccupancySyncRun[] }>(`/api/properties/${pid.value}/occupancy-sync/runs`),
      api<{ source: { active: boolean; source_type: string } }>(
        `/api/properties/${pid.value}/occupancy-source`,
      ),
    ])
    runs.value = runResult.runs
    source.value = sourceResult.source
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Failed to load sync settings'
  }
}

async function runManualSync() {
  if (!pid.value) return
  syncing.value = true
  error.value = ''
  success.value = ''
  try {
    const result = await api<{ ok: boolean; error?: string }>(
      `/api/properties/${pid.value}/occupancy-sync/run`,
      { method: 'POST' },
    )
    if (!result.ok) throw new Error(result.error || 'Occupancy sync failed')
    success.value = 'Occupancy sync completed.'
    await loadSyncPanel()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Failed to run occupancy sync'
  } finally {
    syncing.value = false
  }
}

async function toggleSourceActive() {
  if (!pid.value || !source.value) return
  error.value = ''
  try {
    await api(`/api/properties/${pid.value}/occupancy-source`, {
      method: 'PATCH',
      json: { active: !source.value.active },
    })
    await loadSyncPanel()
    success.value = source.value?.active ? 'ICS sync enabled.' : 'ICS sync paused.'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Failed to update ICS sync state'
  }
}

watch(
  [pid, month, tab],
  () => {
    if (!pid.value) return
    if (tab.value === 'calendar') void loadCalendar()
    else void loadSyncPanel()
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <UiPageHeader
      title="Occupancy"
      lede="Named stays, raw Booking.com blocks, availability, and sync settings."
    />
    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to load occupancy."
    />
    <template v-else>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />
      <UiInlineBanner v-if="success" tone="success" :title="success" />
      <UiTabs v-model="tab" :tabs="tabs" aria-label="Occupancy views">
        <template #default="{ active }">
          <template v-if="active === 'calendar'">
            <OccupancyCalendar
              :month="month"
              :calendar="calendar"
              @update:month="month = $event"
              @prev="prevMonth"
              @next="nextMonth"
              @current="month = monthKey(new Date())"
              @calendar-cell-click="onCalendarCellClick"
            />
            <UiSection title="Availability blocks">
              <p v-if="!calendar?.availability_blocks.length" class="note">
                No availability blocks found for this month.
              </p>
              <div v-else class="detail-list">
                <article v-for="block in calendar.availability_blocks" :key="block.id" class="detail-item">
                  <div class="detail-row">
                    <div>
                      <strong>{{ block.start_date }} → {{ block.end_date }}</strong>
                      <div class="badges">
                        <UiBadge :tone="block.status === 'active' ? 'warning' : 'neutral'">{{
                          block.status
                        }}</UiBadge>
                        <span>{{ block.block_type }}{{ block.reason ? ` · ${block.reason}` : '' }}</span>
                      </div>
                    </div>
                    <div class="actions actions--end">
                      <UiButton size="sm" variant="ghost" @click="openAvailabilityDialog(block)"
                        >Edit</UiButton
                      >
                      <UiButton
                        v-if="block.status === 'active'"
                        size="sm"
                        variant="ghost"
                        :loading="availabilityStatusBusyID === block.id"
                        @click="updateAvailabilityBlockStatus(block, 'archived')"
                        >Archive</UiButton
                      >
                      <UiButton
                        v-else
                        size="sm"
                        variant="primary"
                        :loading="availabilityStatusBusyID === block.id"
                        @click="updateAvailabilityBlockStatus(block, 'active')"
                        >Reactivate</UiButton
                      >
                    </div>
                  </div>
                </article>
              </div>
            </UiSection>
          </template>
          <OccupancySyncPanel
            v-else
            :source="source"
            :runs="runs"
            :syncing="syncing"
            @toggle-source="toggleSourceActive"
            @run-sync="runManualSync"
          />
        </template>
      </UiTabs>

      <UiDialog v-model:open="dayDialogOpen" :title="`Calendar details for ${dayDialogDate}`" size="md">
        <div class="detail-list">
          <div v-if="!dayHasAssignedNight" class="actions">
            <UiButton size="sm" variant="primary" @click="openManualStayDialog()">Create stay</UiButton>
            <UiButton size="sm" variant="ghost" @click="openAvailabilityDialog()"
              >Block availability</UiButton
            >
          </div>
          <section v-if="promotableRawBlocks.length" class="detail-section">
            <h3>Raw Booking.com blocks</h3>
            <article v-for="block in promotableRawBlocks" :key="block.id" class="detail-item">
              <div class="detail-row">
                <div>
                  <strong>{{ block.check_in_date }} → {{ block.check_out_date }}</strong>
                  <div class="badges">
                    <UiBadge tone="warning">Raw block</UiBadge>
                    <span>{{ block.raw_summary || block.source_event_uid }}</span>
                    <UiBadge
                      :tone="
                        block.cleaning_events.some((event) => event.status === 'error') ? 'danger' : 'neutral'
                      "
                    >
                      {{ cleaningSummary(block.cleaning_events) }}
                    </UiBadge>
                  </div>
                </div>
                <UiButton size="sm" variant="primary" @click="openPromoteDialog(block)"
                  >Promote to stay</UiButton
                >
              </div>
            </article>
          </section>
          <section v-if="dayNamedStays.length" class="detail-section">
            <h3>Named stays</h3>
            <article v-for="stay in dayNamedStays" :key="stay.id" class="detail-item">
              <div class="detail-row">
                <div>
                  <strong>{{ stay.display_name }}</strong>
                  <div class="badges">
                    <UiBadge tone="success">{{ stayTypeLabel(stay.stay_type) }}</UiBadge>
                    <UiBadge tone="neutral">{{ stay.check_in_date }} → {{ stay.check_out_date }}</UiBadge>
                    <UiBadge
                      :tone="
                        stay.review_status === 'rejected'
                          ? 'danger'
                          : stay.review_status === 'confirmed'
                            ? 'success'
                            : 'warning'
                      "
                    >
                      Review: {{ stay.review_status }}
                    </UiBadge>
                    <UiBadge v-if="stay.outcome" :tone="stayOutcomeTone(stay.outcome)">{{
                      stayOutcomeLabel(stay.outcome)
                    }}</UiBadge>
                    <UiBadge :tone="stay.cleaning_required ? 'success' : 'neutral'">
                      {{ stay.cleaning_required ? 'Cleaning required' : 'No cleaning' }}
                    </UiBadge>
                    <UiBadge v-if="stay.nuki_generation_status === 'error'" tone="danger">Nuki error</UiBadge>
                    <UiBadge v-if="actionableSourceLinks(stay).length" tone="warning"
                      >Raw source issue</UiBadge
                    >
                    <UiBadge v-if="hasFinanceConfirmedMissingSource(stay)" tone="success"
                      >Finance confirmed</UiBadge
                    >
                  </div>
                  <p v-if="stay.review_reason" class="note">Review: {{ stay.review_reason }}</p>
                  <p v-if="stay.outcome_reason" class="note">Outcome: {{ stay.outcome_reason }}</p>
                  <p v-if="stay.nuki_generation_error" class="warning">
                    Nuki: {{ stay.nuki_generation_error }}
                  </p>
                  <p v-for="link in actionableSourceLinks(stay)" :key="link.id" class="warning">
                    Raw source {{ link.link_status
                    }}{{ link.conflict_reason ? `: ${link.conflict_reason}` : '' }}
                  </p>
                  <p v-if="hasFinanceConfirmedMissingSource(stay)" class="note">
                    Historical ICS source unavailable; payout or statement data confirms this stay.
                  </p>
                </div>
                <div class="actions actions--end">
                  <UiButton size="sm" variant="ghost" @click="openEditStayDialog(stay)">Edit</UiButton>
                  <UiButton
                    v-if="stay.review_status !== 'confirmed'"
                    size="sm"
                    variant="ghost"
                    @click="openReviewDialog(stay, 'confirmed')"
                    >Confirm</UiButton
                  >
                  <UiButton
                    v-if="stay.review_status !== 'rejected'"
                    size="sm"
                    variant="ghost"
                    @click="openReviewDialog(stay, 'rejected')"
                    >Reject</UiButton
                  >
                  <template v-if="stay.stay_type === 'booking_com'">
                    <UiButton
                      size="sm"
                      variant="ghost"
                      @click="openOutcomeDialog(stay, 'cancelled_non_refundable')"
                      >Non-refundable cancellation</UiButton
                    >
                    <UiButton size="sm" variant="ghost" @click="openOutcomeDialog(stay, 'no_show')"
                      >No-show</UiButton
                    >
                    <UiButton
                      v-if="stay.outcome"
                      size="sm"
                      variant="ghost"
                      :loading="outcomeBusy"
                      @click="patchOutcome(stay, null)"
                      >Clear outcome</UiButton
                    >
                  </template>
                  <UiButton
                    v-if="stay.status !== 'cancelled'"
                    size="sm"
                    variant="ghost"
                    :loading="stayStatusBusyID === stay.id"
                    @click="updateNamedStayStatus(stay, 'cancelled')"
                    >Cancel</UiButton
                  >
                  <UiButton
                    v-if="stay.status !== 'archived'"
                    size="sm"
                    variant="ghost"
                    :loading="stayStatusBusyID === stay.id"
                    @click="updateNamedStayStatus(stay, 'archived')"
                    >Archive</UiButton
                  >
                  <UiButton
                    v-if="stay.status !== 'active'"
                    size="sm"
                    variant="primary"
                    :loading="stayStatusBusyID === stay.id"
                    @click="updateNamedStayStatus(stay, 'active')"
                    >Reactivate</UiButton
                  >
                </div>
              </div>
            </article>
          </section>
          <section v-if="dayAvailabilityBlocks.length" class="detail-section">
            <h3>Availability blocks</h3>
            <article v-for="block in dayAvailabilityBlocks" :key="block.id" class="detail-item">
              <div class="detail-row">
                <span
                  >{{ block.start_date }} → {{ block.end_date }} · {{ block.block_type }} ·
                  {{ block.reason }}</span
                >
                <div class="actions actions--end">
                  <UiButton size="sm" variant="ghost" @click="openAvailabilityDialog(block)">Edit</UiButton>
                  <UiButton
                    size="sm"
                    variant="ghost"
                    :loading="availabilityStatusBusyID === block.id"
                    @click="updateAvailabilityBlockStatus(block, 'archived')"
                    >Archive</UiButton
                  >
                </div>
              </div>
            </article>
          </section>
        </div>
        <template #footer><UiButton variant="ghost" @click="dayDialogOpen = false">Close</UiButton></template>
      </UiDialog>

      <UiDialog v-model:open="promoteDialogOpen" title="Promote raw block to named stay" size="sm">
        <form class="dialog-form" @submit.prevent="submitPromoteRawBlock">
          <UiInput v-model="promoteDisplayName" label="Guest / stay name" required :disabled="promoteBusy" />
          <label class="field"
            ><span>Stay type</span
            ><select v-model="promoteStayType" :disabled="promoteBusy">
              <option value="booking_com">Booking.com</option>
              <option value="external">External</option>
              <option value="maintenance">Maintenance</option>
              <option value="personal_use">Personal use</option>
            </select></label
          >
          <div class="date-grid">
            <UiInput
              v-model="promoteCheckIn"
              type="date"
              label="Check-in"
              required
              :min="promoteRawBlock?.check_in_date"
              :max="promoteRawBlock ? addISODate(promoteRawBlock.check_out_date, -1) : ''"
            /><UiInput
              v-model="promoteCheckOut"
              type="date"
              label="Check-out"
              required
              :min="promoteCheckIn"
              :max="promoteRawBlock?.check_out_date"
            />
          </div>
          <label class="checkbox"
            ><input
              v-model="promoteCleaningRequired"
              type="checkbox"
              @change="promoteCleaningManuallyChanged = true"
            />
            Cleaning required</label
          >
          <p v-if="promoteError" class="form-error">{{ promoteError }}</p>
        </form>
        <template #footer
          ><UiButton variant="ghost" @click="promoteDialogOpen = false">Cancel</UiButton
          ><UiButton variant="primary" :loading="promoteBusy" @click="submitPromoteRawBlock"
            >Create named stay</UiButton
          ></template
        >
      </UiDialog>

      <UiDialog v-model:open="manualStayDialogOpen" title="Create named stay" size="sm">
        <form class="dialog-form" @submit.prevent="submitManualStay">
          <UiInput
            v-model="manualStayDisplayName"
            label="Guest / stay name"
            required
            :disabled="manualStayBusy"
          />
          <label class="field"
            ><span>Stay type</span
            ><select v-model="manualStayType" :disabled="manualStayBusy">
              <option value="booking_com">Booking.com</option>
              <option value="external">External</option>
              <option value="maintenance">Maintenance</option>
              <option value="personal_use">Personal use</option>
            </select></label
          >
          <div class="date-grid">
            <UiInput v-model="manualStayCheckIn" type="date" label="Check-in" required /><UiInput
              v-model="manualStayCheckOut"
              type="date"
              label="Check-out"
              required
              :min="manualStayCheckIn"
            />
          </div>
          <label class="checkbox"
            ><input
              v-model="manualStayCleaningRequired"
              type="checkbox"
              @change="manualStayCleaningManuallyChanged = true"
            />
            Cleaning required</label
          >
          <p v-if="manualStayError" class="form-error">{{ manualStayError }}</p>
        </form>
        <template #footer
          ><UiButton variant="ghost" @click="manualStayDialogOpen = false">Cancel</UiButton
          ><UiButton variant="primary" :loading="manualStayBusy" @click="submitManualStay"
            >Create stay</UiButton
          ></template
        >
      </UiDialog>

      <UiDialog v-model:open="editStayDialogOpen" title="Edit named stay" size="sm">
        <form class="dialog-form" @submit.prevent="submitEditStay">
          <UiInput v-model="editStayDisplayName" label="Guest / stay name" required />
          <label class="field"
            ><span>Stay type</span
            ><select v-model="editStayType">
              <option value="booking_com">Booking.com</option>
              <option value="external">External</option>
              <option value="maintenance">Maintenance</option>
              <option value="personal_use">Personal use</option>
            </select></label
          >
          <div class="date-grid">
            <UiInput v-model="editStayCheckIn" type="date" label="Check-in" required /><UiInput
              v-model="editStayCheckOut"
              type="date"
              label="Check-out"
              required
              :min="editStayCheckIn"
            />
          </div>
          <label class="checkbox"
            ><input v-model="editStayCleaningRequired" type="checkbox" /> Cleaning required</label
          >
          <p v-if="editStayError" class="form-error">{{ editStayError }}</p>
        </form>
        <template #footer
          ><UiButton variant="ghost" @click="editStayDialogOpen = false">Cancel</UiButton
          ><UiButton variant="primary" :loading="editStayBusy" @click="submitEditStay"
            >Save stay</UiButton
          ></template
        >
      </UiDialog>

      <UiDialog
        v-model:open="availabilityDialogOpen"
        :title="availabilityEditingID ? 'Edit availability block' : 'Block availability'"
        size="sm"
      >
        <form class="dialog-form" @submit.prevent="submitAvailabilityBlock">
          <label class="field"
            ><span>Block type</span
            ><select v-model="availabilityBlockType">
              <option value="closed">Closed</option>
              <option value="off_market">Off market</option>
            </select></label
          >
          <div class="date-grid">
            <UiInput v-model="availabilityStart" type="date" label="Start date" required /><UiInput
              v-model="availabilityEnd"
              type="date"
              label="End date"
              required
              :min="availabilityStart"
            />
          </div>
          <UiInput v-model="availabilityReason" label="Reason" />
          <p v-if="availabilityError" class="form-error">{{ availabilityError }}</p>
        </form>
        <template #footer
          ><UiButton variant="ghost" @click="availabilityDialogOpen = false">Cancel</UiButton
          ><UiButton variant="primary" :loading="availabilityBusy" @click="submitAvailabilityBlock"
            >Save block</UiButton
          ></template
        >
      </UiDialog>

      <UiDialog v-model:open="outcomeDialogOpen" :title="outcomeTitle" size="sm">
        <form class="dialog-form" @submit.prevent="submitOutcome">
          <p>{{ outcomeTarget?.display_name }}</p>
          <UiInput v-model="outcomeReason" label="Reason (optional)" maxlength="500" />
          <p v-if="outcomeError" class="form-error">{{ outcomeError }}</p>
        </form>
        <template #footer
          ><UiButton variant="ghost" @click="outcomeDialogOpen = false">Cancel</UiButton
          ><UiButton variant="primary" :loading="outcomeBusy" @click="submitOutcome">{{
            outcomeTitle
          }}</UiButton></template
        >
      </UiDialog>

      <UiDialog
        v-model:open="reviewDialogOpen"
        :title="reviewStatus === 'confirmed' ? 'Confirm stay' : 'Reject stay'"
        size="sm"
      >
        <form class="dialog-form" @submit.prevent="submitReview">
          <p>{{ reviewTarget?.display_name }}</p>
          <UiInput v-model="reviewReason" label="Reason (optional)" maxlength="500" />
          <p v-if="reviewError" class="form-error">{{ reviewError }}</p>
        </form>
        <template #footer
          ><UiButton variant="ghost" @click="reviewDialogOpen = false">Cancel</UiButton
          ><UiButton variant="primary" :loading="reviewBusy" @click="submitReview">{{
            reviewStatus === 'confirmed' ? 'Confirm' : 'Reject'
          }}</UiButton></template
        >
      </UiDialog>
    </template>
  </div>
</template>

<style scoped>
.detail-list,
.dialog-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.detail-section h3 {
  margin: 0 0 var(--space-2);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  text-transform: uppercase;
}
.detail-item {
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
}
.detail-row,
.actions,
.badges {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}
.detail-row {
  justify-content: space-between;
}
.actions--end {
  justify-content: flex-end;
  max-width: 34rem;
}
.badges {
  margin-top: var(--space-2);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.warning,
.note,
.form-error {
  margin: var(--space-2) 0 0;
  font-size: var(--font-size-sm);
}
.warning {
  color: var(--warning-fg);
}
.note {
  color: var(--color-text-muted);
}
.form-error {
  color: var(--danger-fg);
}
.date-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
}
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--font-size-sm);
  font-weight: 500;
}
.field select {
  min-height: 40px;
  padding: 0 var(--space-3);
  color: var(--color-text);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
}
.checkbox {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
}
@media (max-width: 767.98px) {
  .date-grid {
    grid-template-columns: 1fr;
  }
  .actions--end {
    justify-content: flex-start;
  }
}
</style>
