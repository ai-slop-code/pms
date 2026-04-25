<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { sleep } from '@/utils/async'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import NukiUpcomingStays from '@/views/nuki/NukiUpcomingStays.vue'
import NukiCodeTable from '@/views/nuki/NukiCodeTable.vue'
import NukiRunsTimeline from '@/views/nuki/NukiRunsTimeline.vue'
import NukiPinRevealDialog from '@/views/nuki/NukiPinRevealDialog.vue'
import type {
  NukiKeypadCode as KeypadCode,
  NukiUpcomingStay as UpcomingStay,
  NukiRun,
  NukiPinReveal as PinReveal,
} from '@/api/types/nuki'

const { pid } = useCurrentProperty()
const loading = ref(false)
const syncing = ref(false)
const generatingOccupancyId = ref<number | null>(null)
const error = ref('')
const success = ref('')
const keypadCodes = ref<KeypadCode[]>([])
const upcomingStays = ref<UpcomingStay[]>([])
const runs = ref<NukiRun[]>([])
const pinNames = ref<Record<number, string>>({})
const savingStayName = ref<Record<number, boolean>>({})
const runsPage = ref(1)
const runsHasMore = ref(false)
const runsPageSize = 10
const revealingCodeId = ref<number | null>(null)

const toast = useToast()
const { confirm } = useConfirm()

const REVEAL_WINDOW_SECONDS = 30
const activeReveal = ref<PinReveal | null>(null)
const revealSecondsLeft = ref(0)
let revealTimer: ReturnType<typeof setInterval> | null = null

function stopRevealTimer() {
  if (revealTimer) {
    clearInterval(revealTimer)
    revealTimer = null
  }
}

function closePinDialog() {
  stopRevealTimer()
  activeReveal.value = null
  revealSecondsLeft.value = 0
}

function startRevealCountdown() {
  stopRevealTimer()
  revealSecondsLeft.value = REVEAL_WINDOW_SECONDS
  revealTimer = setInterval(() => {
    revealSecondsLeft.value -= 1
    if (revealSecondsLeft.value <= 0) {
      closePinDialog()
    }
  }, 1000)
}

async function copyPinToClipboard() {
  const reveal = activeReveal.value
  if (!reveal) return
  try {
    await navigator.clipboard.writeText(reveal.pin)
    toast.success('PIN copied. It will clear when the dialog closes.')
  } catch {
    toast.warning('Could not copy automatically. Select and copy manually.')
  }
}

onBeforeUnmount(stopRevealTimer)

async function loadAll(clearMessages = false) {
  if (!pid.value) return
  loading.value = true
  if (clearMessages) {
    error.value = ''
    success.value = ''
  }
  try {
    const [codesRes, staysRes, runsRes] = await Promise.all([
      api<{ codes: KeypadCode[] }>(`/api/properties/${pid.value}/nuki/codes?pins_only=1`),
      api<{ stays: UpcomingStay[] }>(`/api/properties/${pid.value}/nuki/upcoming-stays?limit=120`),
      api<{ runs: NukiRun[]; has_more: boolean }>(
        `/api/properties/${pid.value}/nuki/runs?limit=${runsPageSize}&page=${runsPage.value}`,
      ),
    ])
    keypadCodes.value = codesRes.codes
    upcomingStays.value = staysRes.stays
    runs.value = runsRes.runs
    runsHasMore.value = !!runsRes.has_more
    const next: Record<number, string> = {}
    for (const s of staysRes.stays) {
      const existing = pinNames.value[s.occupancy_id]
      if (existing && existing.trim() !== '') {
        next[s.occupancy_id] = existing
        continue
      }
      if (s.saved_pin_name && s.saved_pin_name.trim() !== '') {
        next[s.occupancy_id] = s.saved_pin_name.trim()
        continue
      }
      const label = s.generated_label || ''
      if (label.toLowerCase().startsWith('booking-')) {
        next[s.occupancy_id] = label.slice(8)
      }
    }
    pinNames.value = next
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load Nuki data'
  } finally {
    loading.value = false
  }
}

async function saveStayNameForOccupancy(occupancyId: number) {
  if (!pid.value) return
  const raw = pinNames.value[occupancyId] || ''
  const pinName = raw.trim()
  savingStayName.value[occupancyId] = true
  error.value = ''
  try {
    const r = await api<{ ok: boolean; saved_pin_name?: string }>(
      `/api/properties/${pid.value}/nuki/upcoming-stays/${occupancyId}`,
      { method: 'PATCH', json: { pin_name: pinName } },
    )
    if (!r.ok) {
      error.value = 'Failed to save stay name.'
      return
    }
    pinNames.value[occupancyId] = r.saved_pin_name || ''
    success.value = 'Stay name saved.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save stay name.'
  } finally {
    savingStayName.value[occupancyId] = false
  }
}

async function runSync() {
  if (!pid.value) return
  syncing.value = true
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(`/api/properties/${pid.value}/nuki/sync/run`, {
      method: 'POST',
    })
    if (!r.ok) error.value = r.error || 'Nuki access sync failed.'
    else success.value = 'Nuki access sync completed.'
    await loadAll()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to run Nuki access sync.'
  } finally {
    syncing.value = false
  }
}

async function nextRunsPage() {
  if (!runsHasMore.value) return
  runsPage.value += 1
  await loadAll()
}

async function prevRunsPage() {
  if (runsPage.value <= 1) return
  runsPage.value -= 1
  await loadAll()
}

async function syncCodesQuietly() {
  if (!pid.value) return
  try {
    await api<{ ok: boolean }>(`/api/properties/${pid.value}/nuki/sync/run`, { method: 'POST' })
  } catch {
    // best-effort
  }
}

async function refreshAfterGenerate(occupancyId: number) {
  const attempts = 5
  for (let i = 0; i < attempts; i++) {
    await syncCodesQuietly()
    await loadAll()
    const row = upcomingStays.value.find((s) => s.occupancy_id === occupancyId)
    if (row && row.generated_status === 'generated') return true
    await sleep(1200)
  }
  return false
}

async function revealPin(codeId: number, options: { stayName?: string; label?: string } = {}) {
  if (!pid.value) return
  revealingCodeId.value = codeId
  error.value = ''
  try {
    const r = await api<{ pin: string }>(`/api/properties/${pid.value}/nuki/codes/${codeId}/reveal-pin`)
    if (r && r.pin) {
      activeReveal.value = {
        codeId,
        pin: r.pin,
        label: options.label || `Code #${codeId}`,
        stayName: options.stayName,
      }
      startRevealCountdown()
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to reveal PIN.'
    toast.error(error.value)
  } finally {
    revealingCodeId.value = null
  }
}

function revealStayPin(stay: UpcomingStay) {
  if (!stay.generated_code_id) return
  revealPin(stay.generated_code_id, {
    stayName: stay.summary || pinNames.value[stay.occupancy_id] || stay.source_event_uid,
    label: stay.generated_label,
  }).catch(() => {})
}

function revealCode(code: KeypadCode) {
  revealPin(code.id, { stayName: code.name, label: code.name || code.external_nuki_id }).catch(() => {})
}

async function generateForStay(occupancyId: number) {
  if (!pid.value) return
  const pinName = (pinNames.value[occupancyId] || '').trim()
  if (!pinName) {
    error.value = 'Enter a stay name before generating a PIN.'
    return
  }
  generatingOccupancyId.value = occupancyId
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string }>(`/api/properties/${pid.value}/nuki/codes/generate`, {
      method: 'POST',
      json: { occupancy_id: occupancyId, pin_name: pinName },
    })
    if (!r.ok) error.value = r.error || 'PIN generation failed.'
    else {
      const settled = await refreshAfterGenerate(occupancyId)
      if (!settled) await loadAll()
      const row = upcomingStays.value.find((s) => s.occupancy_id === occupancyId)
      if (row && row.generated_code_id) {
        await revealPin(row.generated_code_id, {
          stayName: row.summary || pinNames.value[row.occupancy_id] || row.source_event_uid,
          label: row.generated_label,
        })
      }
      success.value = 'Guest PIN generated.'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to generate guest PIN.'
  } finally {
    generatingOccupancyId.value = null
  }
}

async function deleteKeypadCode(externalId: string) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Delete access code',
    message: 'Delete this keypad code from Nuki and PMS?',
    confirmLabel: 'Delete',
    tone: 'danger',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/nuki/keypad-codes/${encodeURIComponent(externalId)}`, {
      method: 'DELETE',
    })
    await syncCodesQuietly()
    await loadAll()
    success.value = 'Access code deleted.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete access code.'
  }
}

const enabledCodes = computed(() => keypadCodes.value.filter((c) => c.enabled))

function onUpdatePinName(occupancyId: number, value: string) {
  pinNames.value[occupancyId] = value
}

watch(
  () => pid.value,
  () => {
    runsPage.value = 1
    loadAll(true).catch(() => {})
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <UiPageHeader
      title="Nuki access"
      lede="Sync Nuki access codes, then generate guest PINs for upcoming stays."
    />

    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to load Nuki data."
    />

    <template v-else>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />
      <UiInlineBanner v-if="success" tone="success" :title="success" />

      <UiToolbar>
        <UiButton variant="primary" :loading="syncing" @click="runSync">Run Nuki access sync</UiButton>
        <UiButton variant="secondary" :disabled="loading || syncing" @click="loadAll">Refresh data</UiButton>
      </UiToolbar>

      <NukiUpcomingStays
        :stays="upcomingStays"
        :pin-names="pinNames"
        :saving-stay-name="savingStayName"
        :generating-occupancy-id="generatingOccupancyId"
        :revealing-code-id="revealingCodeId"
        @update:pin-name="onUpdatePinName"
        @save-pin-name="saveStayNameForOccupancy"
        @generate="generateForStay"
        @reveal="revealStayPin"
      />

      <NukiCodeTable
        :codes="enabledCodes"
        :revealing-code-id="revealingCodeId"
        @reveal="revealCode"
        @delete="deleteKeypadCode"
      />

      <NukiRunsTimeline
        :runs="runs"
        :page="runsPage"
        :has-more="runsHasMore"
        :loading="loading"
        @prev="prevRunsPage"
        @next="nextRunsPage"
      />
    </template>

    <NukiPinRevealDialog
      :reveal="activeReveal"
      :seconds-left="revealSecondsLeft"
      @copy="copyPinToClipboard"
      @close="closePinDialog"
    />
  </div>
</template>
