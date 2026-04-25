<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import CleaningHeatmap from '@/views/cleaning/CleaningHeatmap.vue'
import CleaningLogsTable from '@/views/cleaning/CleaningLogsTable.vue'
import CleaningFeeHistory from '@/views/cleaning/CleaningFeeHistory.vue'
import { formatEuros } from '@/utils/format'
import { monthKey, shiftMonth } from '@/utils/month'
import type {
  CleaningLogRow,
  CleaningFeeRow,
  CleaningAdjustmentRow,
  CleaningSummary,
  CleaningHeatBucket,
  CleaningNukiCodeRow,
  CleaningReconcileStats,
} from '@/api/types/cleaning'

// Preserve the "€0.00 for nullish" behaviour the inline helper used to have,
// so templates with optional amount fields don't flip to em-dash.
const eur = (cents?: number | null) => formatEuros(cents ?? 0)

const { pid } = useCurrentProperty()
const month = ref(monthKey(new Date()))
const loading = ref(false)
const savingFee = ref(false)
const savingAdjustment = ref(false)
const reconciling = ref(false)
const error = ref('')
const success = ref('')

const logs = ref<CleaningLogRow[]>([])
const fees = ref<CleaningFeeRow[]>([])
const adjustments = ref<CleaningAdjustmentRow[]>([])
const heatmap = ref<CleaningHeatBucket[]>([])
const summary = ref<CleaningSummary | null>(null)
const cleanerAuthID = ref('')

const keypadAuthCandidates = ref<Array<{ value: string; label: string }>>([])
const savingCleanerAuthID = ref(false)
const hasCleanerAuthID = computed(() => cleanerAuthID.value.trim().length > 0)
const showCleanerAuthConfig = ref(false)

const adjustmentForm = ref({
  adjustment_amount_cents: 0,
  reason: '',
})

async function loadAll() {
  if (!pid.value) return
  loading.value = true
  error.value = ''
  try {
    const [logsRes, summaryRes, heatmapRes, feesRes, adjustmentsRes, settingsRes, codesRes] = await Promise.all([
      api<{ logs: CleaningLogRow[] }>(`/api/properties/${pid.value}/cleaning/logs?month=${encodeURIComponent(month.value)}`),
      api<CleaningSummary>(`/api/properties/${pid.value}/cleaning/summary?month=${encodeURIComponent(month.value)}`),
      api<{ buckets: CleaningHeatBucket[] }>(`/api/properties/${pid.value}/cleaning/heatmap?month=${encodeURIComponent(month.value)}`),
      api<{ fees: CleaningFeeRow[] }>(`/api/properties/${pid.value}/cleaning/fees`),
      api<{ adjustments: CleaningAdjustmentRow[] }>(
        `/api/properties/${pid.value}/cleaning/adjustments?month=${encodeURIComponent(month.value)}`
      ),
      api<{ profile: { cleaner_nuki_auth_id?: string } }>(`/api/properties/${pid.value}/settings`),
      api<{ codes: CleaningNukiCodeRow[] }>(`/api/properties/${pid.value}/nuki/codes`),
    ])
    logs.value = logsRes.logs
    summary.value = summaryRes
    heatmap.value = heatmapRes.buckets
    fees.value = feesRes.fees
    adjustments.value = adjustmentsRes.adjustments
    cleanerAuthID.value = settingsRes.profile?.cleaner_nuki_auth_id || ''
    if (!cleanerAuthID.value.trim()) {
      showCleanerAuthConfig.value = true
    } else if (!showCleanerAuthConfig.value) {
      showCleanerAuthConfig.value = false
    }
    const seen = new Set<string>()
    const candidates: Array<{ value: string; label: string }> = []
    for (const c of codesRes.codes || []) {
      const name = (c.name || '-').trim()
      const externalID = (c.external_nuki_id || '').trim()
      const accountUserID = (c.account_user_id || '').trim()
      if (externalID && !seen.has(externalID)) {
        seen.add(externalID)
        candidates.push({ value: externalID, label: `${externalID} — ${name} [ExternalID]` })
      }
      if (accountUserID && !seen.has(accountUserID)) {
        seen.add(accountUserID)
        candidates.push({ value: accountUserID, label: `${accountUserID} — ${name} [accountUserId]` })
      }
    }
    keypadAuthCandidates.value = candidates
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load cleaning analytics'
  } finally {
    loading.value = false
  }
}

async function addFee(payload: {
  cleaning_fee_amount_cents: number
  washing_fee_amount_cents: number
  effective_from: string
}) {
  if (!pid.value) return
  savingFee.value = true
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/cleaning/fees`, {
      method: 'POST',
      json: payload,
    })
    success.value = 'Fee history updated.'
    await loadAll()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save fee history'
  } finally {
    savingFee.value = false
  }
}

async function addAdjustment() {
  if (!pid.value) return
  if (!adjustmentForm.value.reason.trim()) {
    error.value = 'Adjustment reason is required.'
    return
  }
  savingAdjustment.value = true
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/cleaning/adjustments`, {
      method: 'POST',
      json: {
        month: month.value,
        adjustment_amount_cents: adjustmentForm.value.adjustment_amount_cents,
        reason: adjustmentForm.value.reason.trim(),
      },
    })
    adjustmentForm.value.reason = ''
    adjustmentForm.value.adjustment_amount_cents = 0
    success.value = 'Adjustment added.'
    await loadAll()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save monthly adjustment'
  } finally {
    savingAdjustment.value = false
  }
}

async function runReconcileNow() {
  if (!pid.value) return
  reconciling.value = true
  error.value = ''
  success.value = ''
  try {
    const r = await api<{ ok: boolean; error?: string; stats?: CleaningReconcileStats }>(
      `/api/properties/${pid.value}/cleaning/reconcile/run?month=${encodeURIComponent(month.value)}`,
      { method: 'POST' }
    )
    if (!r.ok) {
      error.value = r.error || 'Cleaning reconciliation failed'
      return
    }
    if (r.stats) {
      const fallback = r.stats.fallback_any_event ? ' (fallback mode used)' : ''
      success.value = `Cleaning reconciliation completed: fetched ${r.stats.fetched_events}, auth-matched ${r.stats.auth_matched_events}, entry-like ${r.stats.entry_like_events}, updated days ${r.stats.upserted_days}${fallback}.`
    } else {
      success.value = 'Cleaning logs reconciled from Nuki.'
    }
    await loadAll()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to run cleaning reconciliation'
  } finally {
    reconciling.value = false
  }
}

function prevMonth() { month.value = shiftMonth(month.value, -1) }
function nextMonth() { month.value = shiftMonth(month.value, 1) }

async function saveCleanerAuthID() {
  if (!pid.value) return
  savingCleanerAuthID.value = true
  error.value = ''
  success.value = ''
  try {
    await api(`/api/properties/${pid.value}/settings`, {
      method: 'PATCH',
      json: { profile: { cleaner_nuki_auth_id: cleanerAuthID.value.trim() || null } },
    })
    success.value = 'Cleaner auth ID saved.'
    showCleanerAuthConfig.value = false
    await loadAll()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save cleaner auth ID'
  } finally {
    savingCleanerAuthID.value = false
  }
}

watch([pid, month], () => {
  loadAll().catch(() => {})
}, { immediate: true })
</script>

<template>
  <div>
    <UiPageHeader
      title="Cleaning log & salary"
      lede="Monthly cleaning analytics based on first counted daily entries, with fee history and manual adjustments."
    />

    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to load cleaning analytics."
    />

    <template v-else>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />
      <UiInlineBanner v-if="success" tone="success" :title="success" />

      <UiToolbar sticky>
        <UiButton variant="ghost" :disabled="loading" aria-label="Previous month" @click="prevMonth">
          <template #iconLeft><ChevronLeft :size="16" aria-hidden="true" /></template>
        </UiButton>
        <UiInput v-model="month" type="month" />
        <UiButton variant="ghost" :disabled="loading" aria-label="Next month" @click="nextMonth">
          <template #iconLeft><ChevronRight :size="16" aria-hidden="true" /></template>
        </UiButton>
        <template #trailing>
          <UiButton variant="secondary" :disabled="loading" @click="loadAll">Refresh</UiButton>
          <UiButton variant="primary" :loading="reconciling" @click="runReconcileNow">
            Run reconciliation
          </UiButton>
          <UiButton
            v-if="hasCleanerAuthID && !showCleanerAuthConfig"
            variant="ghost"
            @click="showCleanerAuthConfig = true"
          >Change auth ID</UiButton>
        </template>
      </UiToolbar>

      <UiSection
        v-if="!hasCleanerAuthID || showCleanerAuthConfig"
        title="Cleaner Nuki auth ID"
        description="This auth ID identifies which Nuki entries count as cleaner arrivals."
      >
        <UiCard>
          <div class="auth-form">
            <UiInput
              v-model="cleanerAuthID"
              label="Cleaner auth ID"
              placeholder="e.g. 123456 or cleaner-user-id"
            />
            <UiSelect
              label="Pick from current Nuki codes"
              @update:model-value="(v) => (cleanerAuthID = v)"
            >
              <option value="">Select an auth ID from fetched codes…</option>
              <option v-for="c in keypadAuthCandidates" :key="c.value" :value="c.value">
                {{ c.label }}
              </option>
            </UiSelect>
            <div class="auth-form__actions">
              <UiButton variant="primary" :loading="savingCleanerAuthID" @click="saveCleanerAuthID">
                Save auth ID
              </UiButton>
            </div>
          </div>
        </UiCard>
      </UiSection>

      <div v-if="summary" class="kpi-grid">
        <UiKpiCard label="Counted days" :value="summary.counted_days" />
        <UiKpiCard label="Base salary" :value="eur(summary.base_salary_cents)" />
        <UiKpiCard
          label="Adjustments"
          :value="eur(summary.adjustments_total_cents)"
          :tone="summary.adjustments_total_cents < 0 ? 'danger' : 'default'"
        />
        <UiKpiCard label="Final salary" :value="eur(summary.final_salary_cents)" hero tone="success" />
      </div>

      <CleaningHeatmap :buckets="heatmap" />

      <CleaningLogsTable :logs="logs" />

      <CleaningFeeHistory :fees="fees" :saving="savingFee" @submit="addFee" />

      <UiSection title="Monthly adjustments">
        <UiCard>
          <form class="fee-form" @submit.prevent="addAdjustment">
            <UiInput
              v-model.number="adjustmentForm.adjustment_amount_cents"
              label="Amount (cents)"
              type="number"
              help="Negative for deduction"
            />
            <UiInput
              v-model="adjustmentForm.reason"
              label="Reason"
              placeholder="Bonus, correction…"
            />
            <div class="fee-form__actions">
              <UiButton type="submit" variant="primary" :loading="savingAdjustment">Add adjustment</UiButton>
            </div>
          </form>
        </UiCard>

        <UiTable :empty="!adjustments.length" empty-text="No adjustments recorded for this month.">
          <template #head>
            <tr>
              <th>Created</th>
              <th class="num">Amount</th>
              <th>Reason</th>
            </tr>
          </template>
          <tr v-for="a in adjustments" :key="a.id">
            <td>{{ a.created_at }}</td>
            <td class="num" :class="{ 'amount-negative': a.adjustment_amount_cents < 0 }">
              {{ eur(a.adjustment_amount_cents) }}
            </td>
            <td>{{ a.reason }}</td>
          </tr>
        </UiTable>
      </UiSection>
    </template>
  </div>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
  margin-top: var(--space-4);
}
.auth-form {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
  align-items: end;
}
.auth-form__actions {
  display: flex;
  justify-content: flex-end;
  grid-column: 1 / -1;
}
.fee-form {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  align-items: end;
}
.fee-form__actions {
  display: flex;
  justify-content: flex-end;
  grid-column: 1 / -1;
}
.amount-negative {
  color: var(--danger-fg);
}
.arrival-hbars {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.arrival-hbar-row {
  display: grid;
  grid-template-columns: 56px 1fr 48px;
  align-items: center;
  gap: var(--space-3);
}
.arrival-hbar-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  font-weight: 500;
}
.arrival-hbar-track {
  height: 18px;
  border-radius: 999px;
  background: var(--color-sunken);
  border: 1px solid var(--color-border);
  overflow: hidden;
}
.arrival-hbar-fill {
  height: 100%;
  min-width: 6%;
  border-radius: 999px;
  background: var(--color-primary);
}
.arrival-hbar-value {
  font-size: var(--font-size-xs);
  color: var(--color-text);
  font-weight: 600;
  text-align: right;
}
</style>
