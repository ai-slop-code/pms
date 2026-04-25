<script setup lang="ts">
import { ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import { formatEuros, formatShortDate, isoTitle } from '@/utils/format'
import { monthKey, shiftMonth } from '@/utils/month'
import type {
  BookingPayoutRow,
  BookingPayoutOccupancyOption as OccupancyOption,
} from '@/api/types/bookingPayouts'

const eur = (cents?: number | null) => formatEuros(cents ?? 0)

const { pid } = useCurrentProperty()

const month = ref(monthKey(new Date()))
const mappedOnly = ref<'all' | 'mapped' | 'unmapped'>('all')
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const success = ref('')
const payouts = ref<BookingPayoutRow[]>([])
const occupancyOptions = ref<OccupancyOption[]>([])
const mapInputByRef = ref<Record<string, string>>({})

function prevMonth() { month.value = shiftMonth(month.value, -1) }
function nextMonth() { month.value = shiftMonth(month.value, 1) }

async function load() {
  if (!pid.value) return
  loading.value = true
  error.value = ''
  try {
    let q = `/api/properties/${pid.value}/finance/booking-payouts?month=${encodeURIComponent(month.value)}`
    if (mappedOnly.value === 'mapped') q += '&mapped_only=true'
    if (mappedOnly.value === 'unmapped') q += '&mapped_only=false'
    const r = await api<{ payouts: BookingPayoutRow[] }>(q)
    payouts.value = r.payouts || []
    const next: Record<string, string> = { ...mapInputByRef.value }
    for (const p of payouts.value) {
      if (p.occupancy_id) next[p.reference_number] = String(p.occupancy_id)
      else if (!next[p.reference_number]) next[p.reference_number] = ''
    }
    mapInputByRef.value = next
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load booking payouts'
  } finally {
    loading.value = false
  }
}

async function loadOccupancyOptions() {
  if (!pid.value) return
  const months = [shiftMonth(month.value, -2), shiftMonth(month.value, -1), month.value, shiftMonth(month.value, 1)]
  const uniq = new Map<number, OccupancyOption>()
  for (const m of months) {
    try {
      const r = await api<{ occupancies: OccupancyOption[] }>(
        `/api/properties/${pid.value}/occupancies?month=${encodeURIComponent(m)}&limit=500`
      )
      for (const o of r.occupancies || []) {
        if (!uniq.has(o.id)) uniq.set(o.id, o)
      }
    } catch {
      // best-effort
    }
  }
  occupancyOptions.value = Array.from(uniq.values()).sort((a, b) => a.start_at.localeCompare(b.start_at))
}

function occLabel(o: OccupancyOption) {
  const name = o.raw_summary || o.source_event_uid
  return `${o.start_at.slice(0, 10)} to ${o.end_at.slice(0, 10)} | ${name} [#${o.id}]`
}

function suggestionsForPayout(p: BookingPayoutRow) {
  const inDate = (p.check_in_date || '').trim()
  const outDate = (p.check_out_date || '').trim()
  if (!inDate || !outDate) return occupancyOptions.value.slice(0, 20)
  const exact = occupancyOptions.value.filter(
    (o) => o.start_at.slice(0, 10) === inDate && o.end_at.slice(0, 10) === outDate
  )
  if (exact.length) return exact
  const q = (mapInputByRef.value[p.reference_number] || '').toLowerCase().trim()
  if (!q) return occupancyOptions.value.slice(0, 20)
  return occupancyOptions.value
    .filter((o) => occLabel(o).toLowerCase().includes(q) || o.source_event_uid.toLowerCase().includes(q))
    .slice(0, 20)
}

async function saveMapping(referenceNumber: string, occupancyIdRaw: string) {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  success.value = ''
  try {
    const n = Number(occupancyIdRaw)
    const occupancyID = Number.isFinite(n) && n > 0 ? n : null
    await api(`/api/properties/${pid.value}/finance/booking-payouts/${encodeURIComponent(referenceNumber)}/map`, {
      method: 'PATCH',
      json: { occupancy_id: occupancyID },
    })
    success.value = occupancyID
      ? `Stay mapping saved for ${referenceNumber}.`
      : `Stay mapping cleared for ${referenceNumber}.`
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update stay mapping'
  } finally {
    busy.value = false
  }
}

async function rematchUnmapped() {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; scanned: number; matched: number; updated: number; already_mapped: number; failed: number }>(
      `/api/properties/${pid.value}/finance/booking-payouts/rematch?month=${encodeURIComponent(month.value)}&only_unmapped=true`,
      { method: 'POST' }
    )
    success.value = `Auto-match completed: scanned ${r.scanned}, matched ${r.matched}, updated ${r.updated}, failed ${r.failed}.`
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to run payout auto-match'
  } finally {
    busy.value = false
  }
}

function canCreateStay(p: BookingPayoutRow) {
  return Boolean((p.check_in_date || '').trim() && (p.check_out_date || '').trim())
}

async function createStayFromPayout(referenceNumber: string) {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; occupancy_id: number; created: boolean }>(
      `/api/properties/${pid.value}/finance/booking-payouts/${encodeURIComponent(referenceNumber)}/create-stay`,
      { method: 'POST' }
    )
    success.value = r.created
      ? `Stay created and mapped for ${referenceNumber}.`
      : `Existing stay mapped for ${referenceNumber}.`
    mapInputByRef.value[referenceNumber] = String(r.occupancy_id)
    await load()
    await loadOccupancyOptions()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to create stay'
  } finally {
    busy.value = false
  }
}

watch([pid, month, mappedOnly], () => {
  load().catch(() => {})
}, { immediate: true })

watch([pid, month], () => {
  loadOccupancyOptions().catch(() => {})
}, { immediate: true })
</script>

<template>
  <div>
    <UiPageHeader
      title="Booking payouts"
      lede="Detailed view of imported Booking.com payout rows with stay-mapping status."
    />

    <UiInlineBanner v-if="error" tone="danger" :title="error" />
    <UiInlineBanner v-if="success" tone="success" :title="success" />

    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to load booking payouts."
    />

    <template v-else>
      <UiToolbar sticky>
        <UiButton variant="ghost" :disabled="loading" aria-label="Previous month" @click="prevMonth">
          <template #iconLeft><ChevronLeft :size="16" aria-hidden="true" /></template>
        </UiButton>
        <UiInput v-model="month" type="month" />
        <UiButton variant="ghost" :disabled="loading" aria-label="Next month" @click="nextMonth">
          <template #iconLeft><ChevronRight :size="16" aria-hidden="true" /></template>
        </UiButton>
        <UiSelect v-model="mappedOnly" label="Mapping">
          <option value="all">All</option>
          <option value="mapped">Mapped only</option>
          <option value="unmapped">Unmapped only</option>
        </UiSelect>
        <template #trailing>
          <UiButton variant="secondary" :loading="loading" @click="load">Refresh</UiButton>
          <UiButton variant="primary" :loading="busy" @click="rematchUnmapped">Auto-match unmapped</UiButton>
        </template>
      </UiToolbar>

      <UiTable
        sticky-header
        :empty="!loading && !payouts.length"
        empty-text="No Booking.com payout rows found for the selected filters."
      >
        <template #head>
          <tr>
            <th>Payout</th>
            <th>Reference #</th>
            <th>Guest</th>
            <th>Stay</th>
            <th class="num">Net</th>
            <th class="num">Breakdown</th>
            <th>Mapping</th>
            <th>Invoice</th>
            <th>Map action</th>
          </tr>
        </template>
        <tr v-for="p in payouts" :key="p.id">
          <td :title="isoTitle(p.payout_date)">{{ formatShortDate(p.payout_date) }}</td>
          <td>{{ p.reference_number }}</td>
          <td>{{ p.guest_name || '—' }}</td>
          <td>
            <span v-if="p.check_in_date || p.check_out_date">
              {{ p.check_in_date || '?' }} → {{ p.check_out_date || '?' }}
            </span>
            <span v-else class="muted">—</span>
          </td>
          <td class="num"><strong>{{ eur(p.net_cents) }}</strong></td>
          <td class="num breakdown-cell">
            <div><span class="muted">Amt</span> {{ eur(p.amount_cents) }}</div>
            <div><span class="muted">Com</span> {{ eur(p.commission_cents) }}</div>
            <div><span class="muted">Fee</span> {{ eur(p.payment_service_fee_cents) }}</div>
          </td>
          <td>
            <UiBadge :tone="p.occupancy_id ? 'success' : 'warning'" dot>
              {{ p.occupancy_id ? 'Mapped' : 'Unmapped' }}
            </UiBadge>
          </td>
          <td>
            <span v-if="p.linked_invoice_id" class="invoice-link">#{{ p.linked_invoice_id }}</span>
            <span v-else class="muted">—</span>
          </td>
          <td>
            <div class="map-cell">
              <input
                v-model="mapInputByRef[p.reference_number]"
                :list="`occ-${p.reference_number}`"
                class="map-cell__input"
                type="text"
                placeholder="Search stay…"
              />
              <datalist :id="`occ-${p.reference_number}`">
                <option v-for="o in suggestionsForPayout(p)" :key="o.id" :value="String(o.id)">
                  {{ occLabel(o) }}
                </option>
              </datalist>
              <div class="map-cell__actions">
                <UiButton
                  size="sm"
                  variant="primary"
                  :disabled="busy"
                  @click="saveMapping(p.reference_number, mapInputByRef[p.reference_number] || '')"
                >Save</UiButton>
                <UiButton
                  size="sm"
                  variant="ghost"
                  :disabled="busy"
                  @click="saveMapping(p.reference_number, '')"
                >Clear</UiButton>
                <UiButton
                  size="sm"
                  variant="secondary"
                  :disabled="busy || !canCreateStay(p)"
                  @click="createStayFromPayout(p.reference_number)"
                >Create stay</UiButton>
              </div>
            </div>
          </td>
        </tr>
      </UiTable>
    </template>
  </div>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
}
.invoice-link {
  color: var(--success-fg, var(--color-primary));
  font-weight: 500;
}
.map-cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 12rem;
}
.breakdown-cell {
  font-size: var(--font-size-xs);
  line-height: 1.3;
  white-space: nowrap;
}
.breakdown-cell .muted {
  display: inline-block;
  width: 1.8rem;
  color: var(--color-text-muted);
}
.map-cell__input {
  width: 100%;
  min-height: 32px;
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: var(--font-size-sm) / 1.4 var(--font-family-sans);
  color: var(--color-text);
  background: var(--color-surface);
}
.map-cell__input:focus {
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
  outline: none;
}
.map-cell__actions {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}
</style>
