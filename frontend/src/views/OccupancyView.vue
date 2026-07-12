<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import { useConfirm } from '@/composables/useConfirm'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import OccupancyCalendar from '@/views/occupancy/OccupancyCalendar.vue'
import OccupancyStayList from '@/views/occupancy/OccupancyStayList.vue'
import OccupancySyncPanel from '@/views/occupancy/OccupancySyncPanel.vue'
import OccupancyClosureDialog, {
  type SubmitPayload as ClosureSubmit,
} from '@/views/occupancy/OccupancyClosureDialog.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInput from '@/components/ui/UiInput.vue'
import {
  canExcludeCleaningCalendar,
  canMarkStayOutcome,
  cleaningCalendarStatusLabel,
  closureLabel,
  closureTone,
  formatExternalAmount,
  hasCleaningCalendarExclusion,
  hasStayOutcome,
  isLabelled,
  stayOutcomeLabel,
  stayOutcomeTone,
  type StayOutcome,
} from '@/views/occupancy/closure'
import { monthKey, parseMonthKey } from '@/utils/month'
import type {
  Occupancy as Occ,
  OccupancySyncRun as Run,
  OccupancyApiToken as TokenRow,
  OccupancyRepairReport,
} from '@/api/types/occupancy'

const { pid } = useCurrentProperty()
const { confirm } = useConfirm()
const tab = ref<'calendar' | 'list' | 'sync'>('calendar')
const tabs = [
  { id: 'calendar', label: 'Calendar' },
  { id: 'list', label: 'List' },
  { id: 'sync', label: 'Sync & export' },
]
const error = ref('')
const success = ref('')
const month = ref(monthKey(new Date()))
const statusFilter = ref('')
const occupancies = ref<Occ[]>([])
const runs = ref<Run[]>([])
const tokens = ref<TokenRow[]>([])
const source = ref<{ active: boolean; source_type: string } | null>(null)
const newTokenPlain = ref('')
const syncing = ref(false)
const copiedExport = ref('')
const repairBusy = ref(false)
const repairReport = ref<OccupancyRepairReport | null>(null)

// PMS_14 manual labelling state.
const dialogOpen = ref(false)
const dialogMode = ref<'close' | 'external_sale'>('close')
const dialogBusy = ref(false)
const dialogError = ref('')
const dialogTarget = ref<Occ | null>(null)
const dialogTargetNight = ref('')
const dialogCheckIn = ref('')
const dialogCheckOut = ref('')
const dialogMinDate = ref('')
const dialogMaxDate = ref('')
const splitDialogOpen = ref(false)
const splitDialogBusy = ref(false)
const splitDialogError = ref('')
const splitDialogTarget = ref<Occ | null>(null)
const splitStartNight = ref('')
const splitEndNight = ref('')
const outcomeDialogOpen = ref(false)
const outcomeDialogBusy = ref(false)
const outcomeDialogError = ref('')
const outcomeDialogTarget = ref<Occ | null>(null)
const outcomeDialogOutcome = ref<StayOutcome>('cancelled_non_refundable')
const outcomeReason = ref('')
const cleaningDialogOpen = ref(false)
const cleaningDialogBusy = ref(false)
const cleaningDialogError = ref('')
const cleaningDialogTarget = ref<Occ | null>(null)
const cleaningReason = ref('')
// PMS_19 named-stay flow.
const nameStayDialogOpen = ref(false)
const nameStayBusy = ref(false)
const nameStayError = ref('')
const nameStayTarget = ref<Occ | null>(null)
const nameStayMode = ref<'create' | 'edit'>('create')
const nameStayCheckIn = ref('')
const nameStayCheckOut = ref('')
const nameStayGuest = ref('')
const nameStayLabel = computed(() => {
  const o = nameStayTarget.value
  if (!o) return ''
  return `${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} • Booking block`
})
const nameStayDialogTitle = computed(() =>
  nameStayMode.value === 'edit' ? 'Edit named stay' : 'Name stay / create guest stay',
)
const nameStaySubmitLabel = computed(() =>
  nameStayMode.value === 'edit' ? 'Save named stay' : 'Create named stay',
)
// §5.1 step 5: the selectable range is limited to the latest Booking.com block.
const nameStayBlock = computed(() => {
  const target = nameStayTarget.value
  if (!target?.upstream_event_uid) return target
  return occupancies.value.find((o) => o.source_event_uid === target.upstream_event_uid) || target
})
const nameStayBlockStart = computed(() => nameStayBlock.value?.start_at.slice(0, 10) ?? '')
const nameStayBlockEnd = computed(() => nameStayBlock.value?.end_at.slice(0, 10) ?? '')
const nameStayCheckInMax = computed(() =>
  nameStayBlockEnd.value ? addISODate(nameStayBlockEnd.value, -1) : '',
)
function canNameStay(o: Occ) {
  return (
    !isLabelled(o) &&
    !hasStayOutcome(o) &&
    !o.superseded &&
    (o.representation_kind === 'unnamed_block' || o.representation_kind === 'legacy_generated_night') &&
    !!o.upstream_event_uid
  )
}
function isProvisionalBlock(o: Occ) {
  return o.representation_kind === 'unnamed_block' || o.representation_kind === 'legacy_generated_night'
}
function canEditNamedStay(o: Occ) {
  return !o.superseded && o.representation_kind === 'named_stay' && o.status !== 'deleted_from_source' && o.status !== 'cancelled'
}
function openNameStayDialog(o: Occ, night = '') {
  nameStayMode.value = 'create'
  nameStayTarget.value = o
  const clicked = night || o.start_at.slice(0, 10)
  nameStayCheckIn.value = clicked
  nameStayCheckOut.value = addISODate(clicked, 1)
  nameStayGuest.value = ''
  nameStayError.value = ''
  nameStayDialogOpen.value = true
}
function openEditNameStayDialog(o: Occ) {
  dayDialogOpen.value = false
  nameStayMode.value = 'edit'
  nameStayTarget.value = o
  nameStayCheckIn.value = o.start_at.slice(0, 10)
  nameStayCheckOut.value = o.end_at.slice(0, 10)
  nameStayGuest.value = o.guest_display_name || o.raw_summary || ''
  nameStayError.value = ''
  nameStayDialogOpen.value = true
}
function nameStayFromDay(o: Occ) {
  dayDialogOpen.value = false
  openNameStayDialog(o, dayDialogDate.value)
}
async function submitNameStayDialog() {
  if (!pid.value || !nameStayTarget.value) return
  const uid = nameStayTarget.value.upstream_event_uid
  if (!uid) {
    nameStayError.value = 'This block has no upstream identity to attach a stay to.'
    return
  }
  const guest = nameStayGuest.value.trim()
  if (!guest) {
    nameStayError.value = 'Enter a guest / stay name.'
    return
  }
  if (!isISODate(nameStayCheckIn.value) || !isISODate(nameStayCheckOut.value) || nameStayCheckOut.value <= nameStayCheckIn.value) {
    nameStayError.value = 'Choose a valid check-in and later check-out.'
    return
  }
  if (nameStayCheckIn.value < nameStayBlockStart.value || nameStayCheckOut.value > nameStayBlockEnd.value) {
    nameStayError.value = `Stay must stay within the Booking.com block ${nameStayBlockStart.value} → ${nameStayBlockEnd.value}.`
    return
  }
  nameStayBusy.value = true
  nameStayError.value = ''
  error.value = ''
  success.value = ''
  try {
    const path =
      nameStayMode.value === 'edit'
        ? `/api/properties/${pid.value}/occupancies/${nameStayTarget.value.id}/named-stay`
        : `/api/properties/${pid.value}/occupancy-blocks/${encodeURIComponent(uid)}/named-stays`
    const r = await api<{ ok: boolean; error?: string }>(path, {
      method: nameStayMode.value === 'edit' ? 'PATCH' : 'POST',
      json: { check_in: nameStayCheckIn.value, check_out: nameStayCheckOut.value, guest_display_name: guest },
    })
    if (!r.ok) throw new Error(r.error || 'Failed to save named stay')
    nameStayDialogOpen.value = false
    success.value = nameStayMode.value === 'edit' ? `Named stay “${guest}” updated.` : `Named stay “${guest}” created.`
    await reloadCurrentOccupancyView()
  } catch (e) {
    nameStayError.value = e instanceof Error ? e.message : 'Failed to save named stay'
  } finally {
    nameStayBusy.value = false
  }
}
async function deleteNamedStay(o: Occ) {
  if (!pid.value) return
  dayDialogOpen.value = false
  const ok = await confirm({
    title: 'Delete named stay',
    message: 'Remove this named guest stay and return its source-covered nights to unnamed Booking block coverage?',
    confirmLabel: 'Delete named stay',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(`/api/properties/${pid.value}/occupancies/${o.id}/named-stay`, { method: 'DELETE' })
    if (!r.ok) throw new Error(r.error || 'Failed to delete named stay')
    success.value = 'Named stay deleted.'
    await reloadCurrentOccupancyView()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete named stay'
  }
}
const dialogStayLabel = computed(() => {
  const o = dialogTarget.value
  if (!o) return ''
  const prefix = dialogTargetNight.value ? `${dialogTargetNight.value} night from ` : ''
  return `${prefix}${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} • ${o.raw_summary || o.source_event_uid}`
})
const splitDialogStayLabel = computed(() => {
  const o = splitDialogTarget.value
  if (!o) return ''
  return `${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} • ${o.raw_summary || o.source_event_uid}`
})
const outcomeDialogTitle = computed(() =>
  outcomeDialogOutcome.value === 'cancelled_non_refundable'
    ? 'Mark non-refundable cancellation'
    : 'Mark no-show',
)
const outcomeDialogCopy = computed(() =>
  outcomeDialogOutcome.value === 'cancelled_non_refundable'
    ? 'This keeps the nights counted as occupied and removes the checkout cleaning event. It will not count as a normal cancellation.'
    : 'This removes the checkout cleaning event and marks Booking.com commission handling as no-show. Revenue still comes from imported Booking.com files.',
)
const outcomeDialogStayLabel = computed(() => {
  const o = outcomeDialogTarget.value
  if (!o) return ''
  return `${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} • ${o.raw_summary || o.source_event_uid}`
})
const cleaningDialogStayLabel = computed(() => {
  const o = cleaningDialogTarget.value
  if (!o) return ''
  return `${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} • ${o.raw_summary || o.source_event_uid}`
})
// Calendar day-actions popup state (PMS_14).
const dayDialogOpen = ref(false)
const dayDialogDate = ref('')
const dayDialogStays = ref<Occ[]>([])

function onCalendarCellClick(payload: { dateKey: string; stays: Occ[] }) {
  if (!payload.stays.length) return
  dayDialogDate.value = payload.dateKey
  dayDialogStays.value = payload.stays
  dayDialogOpen.value = true
}

function stayLabel(o: Occ) {
  return `${o.start_at?.slice(0, 10)} → ${o.end_at?.slice(0, 10)} · ${o.raw_summary || o.source_event_uid || 'Stay'}`
}

function isManualSplit(o: Occ) {
  return o.source_type === 'manual' || o.source_event_uid?.startsWith('manual_split:') || false
}

function metadataRows(o: Occ) {
  return [
    ['Source type', o.source_type || '—'],
    ['Upstream source', o.upstream_source_type || '—'],
    ['Upstream UID', o.upstream_event_uid || '—'],
    ['Representation', o.representation_kind || '—'],
    ['Status', o.status || '—'],
    ['Last sync run', o.last_sync_run_id ? String(o.last_sync_run_id) : '—'],
    ['Manual split', isManualSplit(o) ? 'Yes' : 'No'],
  ]
}

function stayNights(o: Occ) {
  const start = Date.parse(o.start_at)
  const end = Date.parse(o.end_at)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 0
  return Math.round((end - start) / 86_400_000)
}

function canSplitNights(o: Occ) {
  return !isLabelled(o) && !hasStayOutcome(o) && stayNights(o) > 1
}

function openCloseFromDay(o: Occ) {
  dayDialogOpen.value = false
  openCloseDialog(o, dayDialogDate.value)
}
function openExternalSaleFromDay(o: Occ) {
  dayDialogOpen.value = false
  openExternalSaleDialog(o)
}
async function reopenFromDay(o: Occ) {
  dayDialogOpen.value = false
  await reopenStay(o)
}
function markOutcomeFromDay(o: Occ, outcome: StayOutcome) {
  dayDialogOpen.value = false
  openOutcomeDialog(o, outcome)
}
async function clearOutcomeFromDay(o: Occ) {
  dayDialogOpen.value = false
  await clearOutcome(o)
}
function splitNightsFromDay(o: Occ) {
  dayDialogOpen.value = false
  openSplitDialog(o, dayDialogDate.value)
}
function excludeCleaningCalendarFromDay(o: Occ) {
  dayDialogOpen.value = false
  openCleaningExcludeDialog(o)
}
async function includeCleaningCalendarFromDay(o: Occ) {
  dayDialogOpen.value = false
  await includeCleaningCalendar(o)
}

function openOutcomeDialog(o: Occ, outcome: StayOutcome) {
  outcomeDialogTarget.value = o
  outcomeDialogOutcome.value = outcome
  outcomeReason.value = ''
  outcomeDialogError.value = ''
  outcomeDialogOpen.value = true
}

async function submitOutcomeDialog() {
  if (!pid.value || !outcomeDialogTarget.value) return
  const reason = outcomeReason.value.trim()
  if (reason.length > 500) {
    outcomeDialogError.value = 'Reason is too long.'
    return
  }
  const occID = outcomeDialogTarget.value.id
  const slug =
    outcomeDialogOutcome.value === 'cancelled_non_refundable'
      ? 'cancelled-non-refundable'
      : 'no-show'
  outcomeDialogBusy.value = true
  outcomeDialogError.value = ''
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(`/api/properties/${pid.value}/occupancies/${occID}/outcome/${slug}`, {
      method: 'POST',
      json: { reason },
    })
    if (!r.ok) throw new Error(r.error || 'Failed to mark outcome')
    outcomeDialogOpen.value = false
    success.value = `${stayOutcomeLabel(outcomeDialogOutcome.value)} marked.`
    if (tab.value === 'list') await loadList()
    else if (tab.value === 'calendar') await loadCalendar()
  } catch (e) {
    outcomeDialogError.value = e instanceof Error ? e.message : 'Failed to mark outcome'
  } finally {
    outcomeDialogBusy.value = false
  }
}

async function clearOutcome(o: Occ) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Clear outcome',
    message: 'Clear the stay outcome override and let normal occupancy, cleaning, and finance rules apply again?',
    confirmLabel: 'Clear outcome',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(`/api/properties/${pid.value}/occupancies/${o.id}/outcome/clear`, { method: 'POST' })
    if (!r.ok) throw new Error(r.error || 'Failed to clear outcome')
    success.value = 'Stay outcome cleared.'
    if (tab.value === 'list') await loadList()
    else if (tab.value === 'calendar') await loadCalendar()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to clear outcome'
  }
}

function openCleaningExcludeDialog(o: Occ) {
  cleaningDialogTarget.value = o
  cleaningReason.value = ''
  cleaningDialogError.value = ''
  cleaningDialogOpen.value = true
}

async function submitCleaningExcludeDialog() {
  if (!pid.value || !cleaningDialogTarget.value) return
  const reason = cleaningReason.value.trim()
  if (reason.length > 500) {
    cleaningDialogError.value = 'Reason is too long.'
    return
  }
  cleaningDialogBusy.value = true
  cleaningDialogError.value = ''
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(
      `/api/properties/${pid.value}/occupancies/${cleaningDialogTarget.value.id}/cleaning-calendar/exclude`,
      { method: 'POST', json: { reason } },
    )
    cleaningDialogOpen.value = false
    await reloadCurrentOccupancyView()
    if (!r.ok) {
      error.value = r.error || 'Cleaning calendar exclusion saved, but calendar reconciliation failed.'
      return
    }
    success.value = 'Cleaning calendar event excluded.'
  } catch (e) {
    cleaningDialogError.value = e instanceof Error ? e.message : 'Failed to exclude cleaning calendar event'
  } finally {
    cleaningDialogBusy.value = false
  }
}

async function includeCleaningCalendar(o: Occ) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Mark as cleaned by cleaning lady',
    message: 'This restores the default behavior. PMS will create the cleaning calendar event again if the stay is still eligible.',
    confirmLabel: 'Send cleaning event',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(
      `/api/properties/${pid.value}/occupancies/${o.id}/cleaning-calendar/include`,
      { method: 'POST' },
    )
    await reloadCurrentOccupancyView()
    if (!r.ok) {
      error.value = r.error || 'Cleaning calendar inclusion saved, but calendar reconciliation failed.'
      return
    }
    success.value = 'Cleaning calendar event restored.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to restore cleaning calendar event'
  }
}

async function reloadCurrentOccupancyView() {
  if (tab.value === 'list') await loadList()
  else if (tab.value === 'calendar') await loadCalendar()
}

function openCloseDialog(o: Occ, night = '') {
  dialogTarget.value = o
  dialogTargetNight.value = night
  const start = o.start_at.slice(0, 10)
  const end = o.end_at.slice(0, 10)
  const clicked = night || start
  dialogCheckIn.value = clicked
  dialogCheckOut.value = night ? addISODate(clicked, 1) : end
  dialogMinDate.value = start
  dialogMaxDate.value = end
  dialogMode.value = 'close'
  dialogError.value = ''
  dialogOpen.value = true
}

function openExternalSaleDialog(o: Occ) {
  dialogTarget.value = o
  dialogTargetNight.value = ''
  dialogCheckIn.value = ''
  dialogCheckOut.value = ''
  dialogMinDate.value = ''
  dialogMaxDate.value = ''
  dialogMode.value = 'external_sale'
  dialogError.value = ''
  dialogOpen.value = true
}

async function submitDialog(payload: ClosureSubmit) {
  if (!pid.value || !dialogTarget.value) return
  const occID = dialogTarget.value.id
  const path =
    dialogMode.value === 'close'
      ? `/api/properties/${pid.value}/occupancies/${occID}/close`
      : `/api/properties/${pid.value}/occupancies/${occID}/external-sale`
  dialogBusy.value = true
  dialogError.value = ''
  try {
    const json =
      dialogMode.value === 'close'
        ? { ...payload, check_in: dialogCheckIn.value, check_out: dialogCheckOut.value }
        : payload
    await api(path, { method: 'POST', json })
    dialogOpen.value = false
    dialogTargetNight.value = ''
    success.value =
      dialogMode.value === 'close' ? 'Stay marked as closed.' : 'Stay marked as externally sold.'
    if (tab.value === 'list') await loadList()
    else if (tab.value === 'calendar') await loadCalendar()
  } catch (e) {
    dialogError.value = e instanceof Error ? e.message : 'Failed to update stay'
  } finally {
    dialogBusy.value = false
  }
}

async function reopenStay(o: Occ) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Reopen stay',
    message: 'Clear the closed / externally-sold label and restore this stay to its original state?',
    confirmLabel: 'Reopen',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/occupancies/${o.id}/reopen`, { method: 'POST' })
    success.value = 'Stay reopened.'
    if (tab.value === 'list') await loadList()
    else if (tab.value === 'calendar') await loadCalendar()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to reopen stay'
  }
}

function openSplitDialog(o: Occ, night = '') {
  const stayStart = o.start_at.slice(0, 10)
  const stayLastNight = addISODate(o.end_at.slice(0, 10), -1)
  const initialNight = night && night >= stayStart && night <= stayLastNight ? night : stayStart
  splitDialogTarget.value = o
  splitStartNight.value = initialNight
  splitEndNight.value = initialNight
  splitDialogError.value = ''
  splitDialogOpen.value = true
}

async function submitSplitDialog() {
  if (!pid.value || !splitDialogTarget.value) return
  error.value = ''
  success.value = ''
  splitDialogError.value = ''
  const o = splitDialogTarget.value
  const stayStart = o.start_at.slice(0, 10)
  const stayLastNight = addISODate(o.end_at.slice(0, 10), -1)
  if (!isISODate(splitStartNight.value) || !isISODate(splitEndNight.value)) {
    splitDialogError.value = 'Choose a valid first and last night.'
    return
  }
  if (
    splitStartNight.value < stayStart ||
    splitEndNight.value > stayLastNight ||
    splitEndNight.value < splitStartNight.value
  ) {
    splitDialogError.value = `Choose nights inside ${stayStart} → ${stayLastNight}.`
    return
  }
  const checkoutDate = addISODate(splitEndNight.value, 1)
  try {
    splitDialogBusy.value = true
    await api(`/api/properties/${pid.value}/occupancies/${o.id}/split-nights`, {
      method: 'POST',
      json: { start_date: splitStartNight.value, end_date: checkoutDate },
    })
    splitDialogOpen.value = false
    success.value = 'Stay split into nightly rows.'
    if (tab.value === 'list') await loadList()
    else if (tab.value === 'calendar') await loadCalendar()
  } catch (e) {
    splitDialogError.value = e instanceof Error ? e.message : 'Failed to split stay'
  } finally {
    splitDialogBusy.value = false
  }
}

function isISODate(v: string) {
  return /^\d{4}-\d{2}-\d{2}$/.test(v) && addISODate(v, 0) === v
}

function addISODate(v: string, days: number) {
  const [year, monthNumber, day] = v.split('-').map((part) => Number(part))
  if (!year || !monthNumber || !day) return ''
  const d = new Date(Date.UTC(year, monthNumber - 1, day + days))
  return d.toISOString().slice(0, 10)
}

function prevMonth() {
  const { year, month: m } = parseMonthKey(month.value)
  month.value = monthKey(new Date(year, m - 2, 1))
}
function nextMonth() {
  const { year, month: m } = parseMonthKey(month.value)
  month.value = monthKey(new Date(year, m, 1))
}
function goToCurrentMonth() {
  month.value = monthKey(new Date())
}

async function loadCalendar() {
  if (!pid.value) return
  error.value = ''
  try {
    const r = await api<{ occupancies: Occ[] }>(
      `/api/properties/${pid.value}/occupancies/calendar?month=${encodeURIComponent(month.value)}`,
    )
    occupancies.value = r.occupancies
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load occupancy calendar'
  }
}

async function loadList() {
  if (!pid.value) return
  error.value = ''
  try {
    let q = `/api/properties/${pid.value}/occupancies?limit=200`
    if (month.value) q += `&month=${encodeURIComponent(month.value)}`
    // 'superseded' is a client-side (audit) view, not a DB status, so we fetch
    // all rows and let the list component filter (PMS_19 §8).
    if (statusFilter.value && statusFilter.value !== 'superseded') q += `&status=${encodeURIComponent(statusFilter.value)}`
    const r = await api<{ occupancies: Occ[] }>(q)
    occupancies.value = r.occupancies
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load occupancy list'
  }
}

async function loadSyncPanel() {
  if (!pid.value) return
  error.value = ''
  try {
    const [r1, r2, r3] = await Promise.all([
      api<{ runs: Run[] }>(`/api/properties/${pid.value}/occupancy-sync/runs`),
      api<{ tokens: TokenRow[] }>(`/api/properties/${pid.value}/occupancy-api-tokens`),
      api<{ source: { active: boolean; source_type: string } }>(
        `/api/properties/${pid.value}/occupancy-source`,
      ),
    ])
    runs.value = r1.runs
    tokens.value = r2.tokens
    source.value = r3.source
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load sync and export settings'
  }
}

async function runManualSync() {
  if (!pid.value) return
  syncing.value = true
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(`/api/properties/${pid.value}/occupancy-sync/run`, {
      method: 'POST',
    })
    if (!r.ok) error.value = r.error || 'Occupancy sync failed'
    else success.value = 'Occupancy sync completed.'
    await loadSyncPanel()
    if (tab.value === 'calendar') await loadCalendar()
    if (tab.value === 'list') await loadList()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to run occupancy sync'
  } finally {
    syncing.value = false
  }
}

async function dryRunRepair() {
  if (!pid.value) return
  repairBusy.value = true
  error.value = ''
  success.value = ''
  try {
    repairReport.value = await api<OccupancyRepairReport>(
      `/api/properties/${pid.value}/occupancy-repair/ics-reconciliation/dry-run`,
      { method: 'POST' },
    )
    success.value = 'Repair dry-run completed.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Repair dry-run failed'
  } finally {
    repairBusy.value = false
  }
}

async function applyRepair() {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Apply ICS repair',
    message: 'Apply the dry-run repair plan now? This never hard-deletes occupancy rows, but it may supersede duplicates and mark disappeared rows deleted from source.',
    confirmLabel: 'Apply repair',
  })
  if (!ok) return
  repairBusy.value = true
  error.value = ''
  success.value = ''
  try {
    repairReport.value = await api<OccupancyRepairReport>(
      `/api/properties/${pid.value}/occupancy-repair/ics-reconciliation/apply`,
      { method: 'POST' },
    )
    success.value = 'ICS repair applied.'
    await loadSyncPanel()
    if (tab.value === 'calendar') await loadCalendar()
    if (tab.value === 'list') await loadList()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Repair apply failed'
  } finally {
    repairBusy.value = false
  }
}

async function toggleSourceActive() {
  if (!pid.value || !source.value) return
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/occupancy-source`, {
      method: 'PATCH',
      json: { active: !source.value.active },
    })
    await loadSyncPanel()
    success.value = source.value?.active ? 'ICS sync enabled.' : 'ICS sync paused.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update ICS sync state'
  }
}

async function createToken() {
  if (!pid.value) return
  error.value = ''
  success.value = ''
  newTokenPlain.value = ''
  try {
    const r = await api<{ id: number; token: string }>(`/api/properties/${pid.value}/occupancy-api-tokens`, {
      method: 'POST',
      json: {},
    })
    newTokenPlain.value = r.token
    await loadSyncPanel()
    success.value = 'Export token created. Save it now; it will not be shown again.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to create export token'
  }
}

async function removeToken(id: number) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Revoke export token',
    message: 'Revoke this export token? Any integration using it will stop working.',
    confirmLabel: 'Revoke',
    tone: 'danger',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/occupancy-api-tokens/${id}`, { method: 'DELETE' })
    await loadSyncPanel()
    success.value = 'Export token revoked.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to revoke export token'
  }
}

function exportUrl() {
  const base = typeof window !== 'undefined' ? window.location.origin : ''
  return `${base}/api/properties/${pid.value}/occupancy-export`
}

async function copyExportCurl() {
  if (!newTokenPlain.value || !pid.value) return
  const cmd = `curl -H "Authorization: Bearer ${newTokenPlain.value}" ${exportUrl()}`
  try {
    await navigator.clipboard.writeText(cmd)
    copiedExport.value = 'curl command copied.'
    setTimeout(() => (copiedExport.value = ''), 3000)
  } catch {
    copiedExport.value = cmd
  }
}

watch(
  [pid, month, tab],
  () => {
    if (!pid.value) return
    if (tab.value === 'calendar') loadCalendar()
    else if (tab.value === 'list') loadList()
    else loadSyncPanel()
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <UiPageHeader
      title="Occupancy"
      lede="Calendar view of nightly occupancy, list of stays, and sync settings."
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
          <OccupancyCalendar
            v-if="active === 'calendar'"
            :month="month"
            :occupancies="occupancies"
            @update:month="month = $event"
            @prev="prevMonth"
            @next="nextMonth"
            @current="goToCurrentMonth"
            @cell-click="onCalendarCellClick"
          />
          <OccupancyStayList
            v-else-if="active === 'list'"
            :month="month"
            :status-filter="statusFilter"
            :occupancies="occupancies"
            :busy="dialogBusy || splitDialogBusy || outcomeDialogBusy || cleaningDialogBusy"
            @update:month="month = $event"
            @update:status-filter="statusFilter = $event"
            @prev="prevMonth"
            @next="nextMonth"
            @refresh="loadList"
            @close="openCloseDialog"
            @external-sale="openExternalSaleDialog"
            @reopen="reopenStay"
            @mark-outcome="openOutcomeDialog"
            @clear-outcome="clearOutcome"
            @exclude-cleaning-calendar="openCleaningExcludeDialog"
            @include-cleaning-calendar="includeCleaningCalendar"
          />
          <OccupancySyncPanel
            v-else
            :source="source"
            :runs="runs"
            :tokens="tokens"
            :syncing="syncing"
            :repair-busy="repairBusy"
            :repair-report="repairReport"
            :new-token-plain="newTokenPlain"
            :copied-export="copiedExport"
            @toggle-source="toggleSourceActive"
            @run-sync="runManualSync"
            @repair-dry-run="dryRunRepair"
            @repair-apply="applyRepair"
            @create-token="createToken"
            @remove-token="removeToken"
            @copy-curl="copyExportCurl"
          />
        </template>
      </UiTabs>

      <OccupancyClosureDialog
        v-model:open="dialogOpen"
        :mode="dialogMode"
        :stay-label="dialogStayLabel"
        :check-in="dialogCheckIn"
        :check-out="dialogCheckOut"
        :min-date="dialogMinDate"
        :max-date="dialogMaxDate"
        :busy="dialogBusy"
        :error-message="dialogError"
        @submit="submitDialog"
      />

      <UiDialog v-model:open="splitDialogOpen" title="Split nights" size="sm">
        <form class="split-dialog" @submit.prevent="submitSplitDialog">
          <p class="split-dialog__copy">
            Choose the nights to split out for {{ splitDialogStayLabel }}. Dates are nights, so a one-night
            maintenance split uses the same first and last night.
          </p>
          <div class="split-dialog__grid">
            <UiInput
              v-model="splitStartNight"
              type="date"
              label="First night"
              required
              :disabled="splitDialogBusy"
            />
            <UiInput
              v-model="splitEndNight"
              type="date"
              label="Last night"
              required
              :disabled="splitDialogBusy"
            />
          </div>
          <p v-if="splitDialogError" class="split-dialog__error">{{ splitDialogError }}</p>
        </form>
        <template #footer>
          <UiButton variant="ghost" :disabled="splitDialogBusy" @click="splitDialogOpen = false"
            >Cancel</UiButton
          >
          <UiButton variant="primary" :loading="splitDialogBusy" @click="submitSplitDialog"
            >Split selected nights</UiButton
          >
        </template>
      </UiDialog>

      <UiDialog v-model:open="outcomeDialogOpen" :title="outcomeDialogTitle" size="sm">
        <form class="outcome-dialog" @submit.prevent="submitOutcomeDialog">
          <p class="outcome-dialog__copy">{{ outcomeDialogCopy }}</p>
          <p class="outcome-dialog__stay">{{ outcomeDialogStayLabel }}</p>
          <UiInput
            v-model="outcomeReason"
            label="Reason (optional)"
            maxlength="500"
            :disabled="outcomeDialogBusy"
            placeholder="Add an operator note"
          />
          <p v-if="outcomeDialogError" class="outcome-dialog__error">{{ outcomeDialogError }}</p>
        </form>
        <template #footer>
          <UiButton variant="ghost" :disabled="outcomeDialogBusy" @click="outcomeDialogOpen = false">
            Cancel
          </UiButton>
          <UiButton variant="primary" :loading="outcomeDialogBusy" @click="submitOutcomeDialog">
            {{ outcomeDialogTitle }}
          </UiButton>
        </template>
      </UiDialog>

      <UiDialog v-model:open="cleaningDialogOpen" title="Do not send cleaning event" size="sm">
        <form class="cleaning-dialog" @submit.prevent="submitCleaningExcludeDialog">
          <p class="cleaning-dialog__copy">
            This removes the PMS-created Google Calendar cleaning event for this checkout. The stay will remain a normal occupied guest stay, so occupancy, finance, Nuki access codes, and guest messaging are not changed.
          </p>
          <p class="cleaning-dialog__stay">{{ cleaningDialogStayLabel }}</p>
          <UiInput
            v-model="cleaningReason"
            label="Reason (optional)"
            maxlength="500"
            :disabled="cleaningDialogBusy"
            placeholder="Cleaner unavailable; owner will clean"
          />
          <p v-if="cleaningDialogError" class="cleaning-dialog__error">{{ cleaningDialogError }}</p>
        </form>
        <template #footer>
          <UiButton variant="ghost" :disabled="cleaningDialogBusy" @click="cleaningDialogOpen = false">
            Cancel
          </UiButton>
          <UiButton variant="primary" :loading="cleaningDialogBusy" @click="submitCleaningExcludeDialog">
            Do not send cleaning event
          </UiButton>
        </template>
      </UiDialog>

      <UiDialog v-model:open="nameStayDialogOpen" :title="nameStayDialogTitle" size="sm">
        <form class="split-dialog" @submit.prevent="submitNameStayDialog">
          <p class="split-dialog__copy">
            {{ nameStayMode === 'edit' ? 'Update' : 'Create' }} a named guest stay inside {{ nameStayLabel }}.
            Dates must remain inside this Booking.com block. A named stay is required before Nuki code generation.
          </p>
          <UiInput
            v-model="nameStayGuest"
            label="Guest / stay name"
            required
            :disabled="nameStayBusy"
            placeholder="e.g. Koilpitchai"
          />
          <div class="split-dialog__grid">
            <UiInput v-model="nameStayCheckIn" type="date" label="Check-in" required :disabled="nameStayBusy" :min="nameStayBlockStart" :max="nameStayCheckInMax" />
            <UiInput v-model="nameStayCheckOut" type="date" label="Check-out" required :disabled="nameStayBusy" :min="nameStayCheckIn" :max="nameStayBlockEnd" />
          </div>
          <p v-if="nameStayError" class="split-dialog__error">{{ nameStayError }}</p>
        </form>
        <template #footer>
          <UiButton variant="ghost" :disabled="nameStayBusy" @click="nameStayDialogOpen = false">Cancel</UiButton>
          <UiButton variant="primary" :loading="nameStayBusy" @click="submitNameStayDialog">{{ nameStaySubmitLabel }}</UiButton>
        </template>
      </UiDialog>

      <UiDialog
        v-model:open="dayDialogOpen"
        :title="dayDialogDate ? `Stays on ${dayDialogDate}` : 'Stays'"
        size="md"
      >
        <p v-if="!dayDialogStays.length" class="day-dialog__empty">No stays on this day.</p>
        <ul v-else class="day-dialog__list">
          <li v-for="o in dayDialogStays" :key="o.id" class="day-dialog__item">
            <div class="day-dialog__row">
              <div class="day-dialog__meta">
                <div class="day-dialog__title">{{ stayLabel(o) }}</div>
                <div class="day-dialog__sub">
                  <UiBadge v-if="isLabelled(o)" :tone="closureTone(o.closure_state)">
                    {{ closureLabel(o.closure_state) }}
                  </UiBadge>
                  <UiBadge v-if="hasStayOutcome(o)" :tone="stayOutcomeTone(o.stay_outcome)">
                    {{ stayOutcomeLabel(o.stay_outcome) }}
                  </UiBadge>
                  <UiBadge :tone="hasCleaningCalendarExclusion(o) ? 'warning' : 'success'">
                    {{ cleaningCalendarStatusLabel(o) }}
                  </UiBadge>
                  <UiBadge v-if="isProvisionalBlock(o) && !isLabelled(o)" tone="info">
                    Provisional cleaning (from Booking block)
                  </UiBadge>
                  <span v-if="o.closure_state === 'external_sale'" class="day-dialog__amount">
                    {{ formatExternalAmount(o) }}
                  </span>
                  <span v-if="o.cleaning_calendar_exclusion_reason" class="day-dialog__amount">
                    {{ o.cleaning_calendar_exclusion_reason }}
                  </span>
                </div>
                <dl class="day-dialog__debug">
                  <div v-for="(row, idx) in metadataRows(o)" :key="idx" class="day-dialog__debug-row">
                    <dt>{{ row[0] }}</dt>
                    <dd>{{ row[1] }}</dd>
                  </div>
                </dl>
              </div>
              <div class="day-dialog__actions">
                <template v-if="!isLabelled(o) && !hasStayOutcome(o) && !canEditNamedStay(o)">
                  <UiButton
                    v-if="canNameStay(o)"
                    size="sm"
                    variant="primary"
                    :disabled="dialogBusy || splitDialogBusy || nameStayBusy"
                    @click="nameStayFromDay(o)"
                  >
                    Name stay
                  </UiButton>
                  <UiButton
                    size="sm"
                    variant="ghost"
                    :disabled="dialogBusy || splitDialogBusy"
                    @click="openCloseFromDay(o)"
                  >
                    Close / no guest
                  </UiButton>
                  <UiButton
                    size="sm"
                    variant="ghost"
                    :disabled="dialogBusy || splitDialogBusy"
                    @click="openExternalSaleFromDay(o)"
                  >
                    Externally sold
                  </UiButton>
                  <UiButton
                    v-if="canSplitNights(o)"
                    size="sm"
                    variant="ghost"
                    :disabled="dialogBusy || splitDialogBusy"
                    @click="splitNightsFromDay(o)"
                  >
                    Split nights
                  </UiButton>
                  <UiButton
                    v-if="canMarkStayOutcome(o)"
                    size="sm"
                    variant="ghost"
                    :disabled="dialogBusy || splitDialogBusy || outcomeDialogBusy"
                    @click="markOutcomeFromDay(o, 'cancelled_non_refundable')"
                  >
                    Non-refundable cancellation
                  </UiButton>
                  <UiButton
                    v-if="canMarkStayOutcome(o)"
                    size="sm"
                    variant="ghost"
                    :disabled="dialogBusy || splitDialogBusy || outcomeDialogBusy"
                    @click="markOutcomeFromDay(o, 'no_show')"
                  >
                    No-show
                  </UiButton>
                </template>
                <template v-else-if="canEditNamedStay(o)">
                  <UiButton
                    size="sm"
                    variant="primary"
                    :disabled="dialogBusy || nameStayBusy"
                    @click="openEditNameStayDialog(o)"
                  >
                    Edit named stay
                  </UiButton>
                  <UiButton
                    size="sm"
                    variant="ghost"
                    :disabled="dialogBusy || nameStayBusy"
                    @click="deleteNamedStay(o)"
                  >
                    Delete named stay
                  </UiButton>
                </template>
                <UiButton
                  v-else-if="isLabelled(o)"
                  size="sm"
                  variant="ghost"
                  :disabled="dialogBusy || splitDialogBusy"
                  @click="reopenFromDay(o)"
                >
                  Reopen
                </UiButton>
                <UiButton
                  v-else
                  size="sm"
                  variant="ghost"
                  :disabled="dialogBusy || splitDialogBusy || outcomeDialogBusy"
                  @click="clearOutcomeFromDay(o)"
                >
                  Clear outcome
                </UiButton>
                <UiButton
                  v-if="canExcludeCleaningCalendar(o)"
                  size="sm"
                  variant="ghost"
                  :disabled="dialogBusy || splitDialogBusy || outcomeDialogBusy || cleaningDialogBusy"
                  @click="excludeCleaningCalendarFromDay(o)"
                >
                  Do not send cleaning event
                </UiButton>
                <UiButton
                  v-else-if="hasCleaningCalendarExclusion(o)"
                  size="sm"
                  variant="ghost"
                  :disabled="dialogBusy || splitDialogBusy || outcomeDialogBusy || cleaningDialogBusy"
                  @click="includeCleaningCalendarFromDay(o)"
                >
                  Mark as cleaned by cleaning lady
                </UiButton>
              </div>
            </div>
          </li>
        </ul>
        <template #footer>
          <UiButton variant="ghost" @click="dayDialogOpen = false">Close</UiButton>
        </template>
      </UiDialog>
    </template>
  </div>
</template>

<style scoped>
.split-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.outcome-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.cleaning-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.split-dialog__copy {
  margin: 0;
  color: var(--color-text-muted);
}
.outcome-dialog__copy,
.outcome-dialog__stay,
.cleaning-dialog__copy,
.cleaning-dialog__stay {
  margin: 0;
  color: var(--color-text-muted);
}
.outcome-dialog__stay,
.cleaning-dialog__stay {
  font-weight: 500;
  color: var(--color-text);
}
.split-dialog__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
}
.split-dialog__error {
  margin: 0;
  color: var(--danger-fg);
  font-size: var(--font-size-sm);
}
.outcome-dialog__error {
  margin: 0;
  color: var(--danger-fg);
  font-size: var(--font-size-sm);
}
.cleaning-dialog__error {
  margin: 0;
  color: var(--danger-fg);
  font-size: var(--font-size-sm);
}
.day-dialog__empty {
  margin: 0;
  color: var(--color-text-muted);
}
.day-dialog__list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.day-dialog__item {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
}
.day-dialog__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  flex-wrap: wrap;
}
.day-dialog__meta {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.day-dialog__title {
  font-weight: 500;
}
.day-dialog__sub {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}
.day-dialog__debug {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 2px var(--space-3);
  margin: var(--space-1) 0 0;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.day-dialog__debug-row {
  display: flex;
  gap: var(--space-1);
  min-width: 0;
}
.day-dialog__debug dt {
  font-weight: 600;
}
.day-dialog__debug dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.day-dialog__actions {
  display: flex;
  gap: var(--space-1);
  flex-shrink: 0;
}
@media (max-width: 767.98px) {
  .split-dialog__grid {
    grid-template-columns: 1fr;
  }
}
</style>
