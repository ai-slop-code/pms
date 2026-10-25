<script setup lang="ts">
import { computed } from 'vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import { formatEuros } from '@/utils/format'
import type { FinanceSummary } from '@/api/types/finance'
import type { FinanceLongTermComparison } from '@/api/types/finance'
import UiButton from '@/components/ui/UiButton.vue'

const props = defineProps<{
  summary: FinanceSummary | null
  recognizedIncomingCents: number
  recognizedNetCents: number | null
  recognizedNetAvailable: boolean
  longTermComparison: FinanceLongTermComparison | null
  month: string
  canManageRent: boolean
  propertyTimezone?: string
}>()

const emit = defineEmits<{ (e: 'manage-long-term-rent'): void }>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)
const monthlyNetPositive = computed(() => (props.summary?.monthly_net_cents || 0) >= 0)
const monthlyNetLabel = computed(() => ((props.summary?.monthly_net_cents || 0) >= 0 ? 'Profitable' : 'Loss'))
const recognizedNetProvisional = computed(() => props.summary?.generated_entry_sync.status === 'not_synced')
const comparison = computed(() => props.longTermComparison)
const periodLabel = computed(() => {
  if (!props.propertyTimezone) return ''
  const now = new Intl.DateTimeFormat('en-CA', {
    timeZone: props.propertyTimezone,
    year: 'numeric',
    month: '2-digit',
  })
    .format(new Date())
    .replace('/', '-')
  return props.month === now
    ? 'Month in progress'
    : props.month > now
      ? 'Future month — based on currently recorded data'
      : ''
})
type LongTermNetPresentation = {
  value: string
  hint: string
  tone: 'default' | 'success' | 'danger'
}

const longTermNetPresentation = computed<LongTermNetPresentation>(() => {
  const c = comparison.value
  if (c?.status === 'not_configured') {
    return {
      value: 'Long-term rent not configured',
      hint: 'Configure a monthly benchmark rent to compare results.',
      tone: 'default',
    }
  }

  const longTermNetCents = c?.status === 'configured' ? c.long_term_net_cents : null
  const recognizedNetCents = props.recognizedNetCents
  if (
    c?.status !== 'configured' ||
    !props.recognizedNetAvailable ||
    typeof longTermNetCents !== 'number' ||
    !Number.isInteger(longTermNetCents) ||
    typeof recognizedNetCents !== 'number' ||
    !Number.isInteger(recognizedNetCents)
  ) {
    return { value: 'Unavailable', hint: 'Not available for the current selection', tone: 'default' }
  }

  const displayCents = -Math.abs(longTermNetCents) + Math.max(recognizedNetCents, 0)
  const normalizedDisplayCents = displayCents === 0 ? 0 : displayCents
  return {
    value: eur(normalizedDisplayCents),
    hint: `Long term rent: ${eur(longTermNetCents)}`,
    tone: normalizedDisplayCents < 0 ? 'danger' : normalizedDisplayCents > 0 ? 'success' : 'default',
  }
})
</script>

<template>
  <div v-if="summary">
    <UiSection title="Monthly CashFlow" description="Payout-date cashflow for the selected month.">
      <div class="kpi-grid">
        <UiKpiCard
          label="Monthly net"
          :value="eur(summary.monthly_net_cents)"
          :tone="monthlyNetPositive ? 'success' : 'danger'"
          :hint="monthlyNetLabel"
          calculation-help="Monthly incoming minus Monthly outgoing. Payout deductions are counted once as reduced income."
          hero
        />
        <UiKpiCard
          label="Monthly incoming"
          :value="eur(summary.monthly_incoming_cents)"
          tone="success"
          calculation-help="Property income plus other incoming transactions dated in the selected month. Imported payouts are counted once."
        />
        <UiKpiCard
          label="Monthly outgoing"
          :value="eur(summary.monthly_outgoing_cents)"
          tone="warning"
          calculation-help="Outgoing transactions dated in the selected month, including stored recurring and cleaning expenses. Payout deductions reduce income instead and are excluded here."
        />
        <UiKpiCard
          label="Property income"
          :value="eur(summary.monthly_property_income_cents)"
          calculation-help="Signed Net from all payout-backed bookings with a payout date in the selected month, regardless of Type. Adjustments and refunds revise the original payout month and reduce income."
        />
        <UiKpiCard
          label="Cleaner expense"
          :value="eur(summary.cleaner_expense_cents)"
          calculation-help="Stored generated cleaning-salary entries for the selected cleaning month, including salary adjustments reflected by the existing sync process."
        />
        <UiKpiCard
          label="Cleaner margin"
          :value="`${(summary.cleaner_margin * 100).toFixed(1)}%`"
          :tone="summary.cleaner_margin >= 0.5 ? 'success' : 'warning'"
          calculation-help="Cleaner expense divided by Property income, shown as a percentage. Displays zero when Property income is zero or negative."
        />
      </div>
      <p class="section-footnote">
        Property income is based on payout date. Other incoming and outgoing transactions use their
        transaction date.
      </p>
    </UiSection>

    <UiSection
      title="Monthly Revenue"
      description="Income is allocated across stay nights, with late-checkout charges recognized in the checkout month."
    >
      <div class="kpi-grid">
        <UiKpiCard
          label="Recognized incoming"
          :value="eur(recognizedIncomingCents)"
          calculation-help="Payout Net is allocated over stay nights. Late-checkout charges are recognized in full in their checkout month; paid cancellations and no-shows count in the scheduled check-in month."
          tone="success"
        />
        <UiKpiCard
          label="Recognized net"
          :value="recognizedNetAvailable ? eur(recognizedNetCents) : 'Unavailable'"
          :hint="
            recognizedNetAvailable
              ? 'Recognized incoming − Monthly outgoing.'
              : 'Not available for the current selection'
          "
          :tone="!recognizedNetAvailable ? 'default' : (recognizedNetCents ?? 0) >= 0 ? 'success' : 'danger'"
          calculation-help="Recognized incoming minus Monthly outgoing. Other incoming transactions are not added; payout deductions are already included in recognized income."
        />
        <UiKpiCard
          label="Long-term net"
          :value="longTermNetPresentation.value"
          :hint="longTermNetPresentation.hint"
          :tone="longTermNetPresentation.tone"
          calculation-help="Start with the negative absolute value of monthly long-term rent minus eligible expenses, then add positive Recognized net. Eligible expenses exclude booking payouts and cleaning salary/source-category expenses."
        />
        <div class="long-term-details">
          <UiButton v-if="canManageRent" size="sm" variant="secondary" @click="emit('manage-long-term-rent')"
            >Manage long-term rent</UiButton
          >
          <template v-if="comparison?.status === 'configured'">
            <span
              >Monthly rent less recorded outgoing, excluding booking payouts and cleaning salary/category
              expenses. Compared with Recognized net.</span
            >
            <strong v-if="periodLabel">{{ periodLabel }}</strong>
            <strong v-if="recognizedNetProvisional"
              >Provisional — generated entries have not been synced.</strong
            >
          </template>
        </div>
        <p v-if="recognizedNetAvailable && recognizedNetProvisional" class="recognition-note">
          Provisional — generated entries have not been synced for this month.
          <span
            >Uses currently stored entries. Sync generated entries to update recurring and cleaning
            amounts.</span
          >
        </p>
      </div>
      <p class="section-footnote">
        Income is allocated across stay nights within the selected calendar month. Expenses use their
        transaction date.
      </p>
    </UiSection>

    <UiSection title="Lifetime totals">
      <div class="kpi-grid">
        <UiKpiCard
          label="Total incoming"
          :value="eur(summary.total_incoming_cents)"
          calculation-help="All-time signed payout Net plus all other incoming transactions. Payout deductions reduce income and payouts are counted once."
        />
        <UiKpiCard
          label="Total outgoing"
          :value="eur(summary.total_outgoing_cents)"
          calculation-help="All-time outgoing transactions excluding booking payouts, whose deductions reduce income instead."
        />
      </div>
    </UiSection>
  </div>
</template>

<style scoped>
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
}

.recognition-note {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--text-muted);
  font-size: 0.875rem;
}

.section-footnote {
  margin: var(--space-3) 0 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.recognition-note span {
  display: block;
}
.long-term-details {
  grid-column: 1 / -1;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-muted);
  font-size: 0.875rem;
}
.long-term-details span:last-of-type {
  flex-basis: 100%;
}
.long-term-details strong {
  color: var(--text);
}
</style>
