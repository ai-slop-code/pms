<script setup lang="ts">
import { ref, watch } from 'vue'
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
          />
          <OccupancyStayList
            v-else-if="active === 'list'"
            :month="month"
            :status-filter="statusFilter"
            :occupancies="occupancies"
            @update:month="month = $event"
            @update:status-filter="statusFilter = $event"
            @prev="prevMonth"
            @next="nextMonth"
            @refresh="loadList"
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
    </template>
  </div>
</template>
