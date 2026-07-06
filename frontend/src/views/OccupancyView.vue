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
import { closureLabel, closureTone, isLabelled, formatExternalAmount } from '@/views/occupancy/closure'
import { monthKey, parseMonthKey } from '@/utils/month'
import type {
  Occupancy as Occ,
  OccupancySyncRun as Run,
  OccupancyApiToken as TokenRow,
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

// PMS_14 manual labelling state.
const dialogOpen = ref(false)
const dialogMode = ref<'close' | 'external_sale'>('close')
const dialogBusy = ref(false)
const dialogError = ref('')
const dialogTarget = ref<Occ | null>(null)
const dialogTargetNight = ref('')
const dialogStayLabel = computed(() => {
  const o = dialogTarget.value
  if (!o) return ''
  const prefix = dialogTargetNight.value
    ? `${dialogTargetNight.value} night from `
    : ''
  return `${prefix}${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} • ${o.raw_summary || o.source_event_uid}`
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

function stayNights(o: Occ) {
  const start = Date.parse(o.start_at)
  const end = Date.parse(o.end_at)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 0
  return Math.round((end - start) / 86_400_000)
}

function canSplitNights(o: Occ) {
  return !isLabelled(o) && stayNights(o) > 1
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
async function splitNightsFromDay(o: Occ) {
  dayDialogOpen.value = false
  await splitStayNights(o)
}
function openCloseDialog(o: Occ, night = '') {
  dialogTarget.value = o
  dialogTargetNight.value = night
  dialogMode.value = 'close'
  dialogError.value = ''
  dialogOpen.value = true
}

function openExternalSaleDialog(o: Occ) {
  dialogTarget.value = o
  dialogTargetNight.value = ''
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
      dialogMode.value === 'close' && dialogTargetNight.value
        ? { ...payload, night: dialogTargetNight.value }
        : payload
    await api(path, { method: 'POST', json })
    dialogOpen.value = false
    dialogTargetNight.value = ''
    success.value = dialogMode.value === 'close' ? 'Stay marked as closed.' : 'Stay marked as externally sold.'
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

async function splitStayNights(o: Occ) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Split into nightly stays',
    message: `Split ${o.start_at.slice(0, 10)} → ${o.end_at.slice(0, 10)} into one row per night? Future ICS syncs will preserve this manual split.`,
    confirmLabel: 'Split nights',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/occupancies/${o.id}/split-nights`, { method: 'POST' })
    success.value = 'Stay split into nightly rows.'
    if (tab.value === 'list') await loadList()
    else if (tab.value === 'calendar') await loadCalendar()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to split stay'
  }
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
    if (statusFilter.value) q += `&status=${encodeURIComponent(statusFilter.value)}`
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
    const r = await api<{ ok: boolean; error?: string }>(
      `/api/properties/${pid.value}/occupancy-sync/run`,
      { method: 'POST' },
    )
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
    const r = await api<{ id: number; token: string }>(
      `/api/properties/${pid.value}/occupancy-api-tokens`,
      { method: 'POST', json: {} },
    )
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
            :busy="dialogBusy"
            @update:month="month = $event"
            @update:status-filter="statusFilter = $event"
            @prev="prevMonth"
            @next="nextMonth"
            @refresh="loadList"
            @close="openCloseDialog"
            @external-sale="openExternalSaleDialog"
            @reopen="reopenStay"
          />
          <OccupancySyncPanel
            v-else
            :source="source"
            :runs="runs"
            :tokens="tokens"
            :syncing="syncing"
            :new-token-plain="newTokenPlain"
            :copied-export="copiedExport"
            @toggle-source="toggleSourceActive"
            @run-sync="runManualSync"
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
        :busy="dialogBusy"
        :error-message="dialogError"
        @submit="submitDialog"
      />

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
                  <span v-if="o.closure_state === 'external_sale'" class="day-dialog__amount">
                    {{ formatExternalAmount(o) }}
                  </span>
                </div>
              </div>
              <div class="day-dialog__actions">
                <template v-if="!isLabelled(o)">
                  <UiButton size="sm" variant="ghost" :disabled="dialogBusy" @click="openCloseFromDay(o)">
                    Close
                  </UiButton>
                  <UiButton size="sm" variant="ghost" :disabled="dialogBusy" @click="openExternalSaleFromDay(o)">
                    Externally sold
                  </UiButton>
                  <UiButton v-if="canSplitNights(o)" size="sm" variant="ghost" :disabled="dialogBusy" @click="splitNightsFromDay(o)">
                    Split nights
                  </UiButton>
                </template>
                <UiButton v-else size="sm" variant="ghost" :disabled="dialogBusy" @click="reopenFromDay(o)">
                  Reopen
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
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}
.day-dialog__actions {
  display: flex;
  gap: var(--space-1);
  flex-shrink: 0;
}
</style>
