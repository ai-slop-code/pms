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
  recognizedGrossCents: number
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
  const now = new Intl.DateTimeFormat('en-CA', { timeZone: props.propertyTimezone, year: 'numeric', month: '2-digit' }).format(new Date()).replace('/', '-')
  return props.month === now ? 'Month in progress' : props.month > now ? 'Future month — based on currently recorded data' : ''
})
const outcomeText = computed(() => {
  const c = comparison.value
  if (!c || c.status !== 'configured' || c.outcome === null || c.short_term_difference_cents === null) return ''
  if (c.outcome === 'equal') return 'Same result as long-term rent'
  return `Short-term ${c.outcome} by ${eur(Math.abs(c.short_term_difference_cents))}`
})
</script>

<template>
  <div v-if="summary">
    <UiSection title="This month" description="Net position for the selected month.">
      <div class="kpi-grid">
        <UiKpiCard
          label="Monthly net"
          :value="eur(summary.monthly_net_cents)"
          :tone="monthlyNetPositive ? 'success' : 'danger'"
          :hint="monthlyNetLabel"
          hero
        />
        <UiKpiCard label="Monthly incoming" :value="eur(summary.monthly_incoming_cents)" tone="success" />
        <UiKpiCard label="Monthly outgoing" :value="eur(summary.monthly_outgoing_cents)" tone="warning" />
        <UiKpiCard label="Property income" :value="eur(summary.monthly_property_income_cents)" />
        <UiKpiCard label="Cleaner expense" :value="eur(summary.cleaner_expense_cents)" />
        <UiKpiCard
          label="Cleaner margin"
          :value="`${(summary.cleaner_margin * 100).toFixed(1)}%`"
          :tone="summary.cleaner_margin >= 0.5 ? 'success' : 'warning'"
        />
      </div>
    </UiSection>

    <UiSection
      title="Revenue recognition"
      description="Booking revenue follows stay-recognition rules. Recognized net also includes other Finance movements in the selected month."
    >
      <div class="kpi-grid">
        <UiKpiCard
          label="Recognized gross"
          :value="eur(recognizedGrossCents)"
          hint="Based on stay nights, not payout date"
          tone="success"
        />
        <UiKpiCard
          label="Recognized net"
          :value="recognizedNetAvailable ? eur(recognizedNetCents) : 'Unavailable'"
          :hint="
            recognizedNetAvailable
              ? 'Stay-based booking net + other incoming − other outgoing.'
              : 'Not available for the current selection'
          "
          :tone="!recognizedNetAvailable ? 'default' : (recognizedNetCents ?? 0) >= 0 ? 'success' : 'danger'"
        />
        <UiKpiCard
          label="Long-term net"
          :value="comparison?.status === 'configured' && comparison.long_term_net_cents !== null ? eur(comparison.long_term_net_cents) : comparison?.status === 'not_configured' ? 'Long-term rent not configured' : 'Unavailable'"
          :hint="comparison?.status === 'configured' ? outcomeText : comparison?.status === 'not_configured' ? 'Configure a monthly benchmark rent to compare results.' : 'Not available for the current selection'"
          :tone="comparison?.status !== 'configured' ? 'default' : comparison.outcome === 'ahead' ? 'success' : comparison.outcome === 'behind' ? 'danger' : 'default'"
        />
        <div class="long-term-details">
          <UiButton v-if="canManageRent" size="sm" variant="secondary" @click="emit('manage-long-term-rent')">Manage long-term rent</UiButton>
          <template v-if="comparison?.status === 'configured'">
            <span>{{ eur(comparison.monthly_rent_cents) }} rent − {{ eur(comparison.eligible_outgoing_cents) }} eligible expenses.</span>
            <span>Monthly rent less recorded outgoing, excluding booking payouts and cleaning salary/category expenses. Compared with Recognized net.</span>
            <strong v-if="periodLabel">{{ periodLabel }}</strong>
            <strong v-if="recognizedNetProvisional">Provisional — generated entries have not been synced.</strong>
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
    </UiSection>

    <UiSection title="Lifetime totals">
      <div class="kpi-grid">
        <UiKpiCard label="Total incoming" :value="eur(summary.total_incoming_cents)" />
        <UiKpiCard label="Total outgoing" :value="eur(summary.total_outgoing_cents)" />
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

.recognition-note span {
  display: block;
}
.long-term-details { grid-column: 1 / -1; display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); color: var(--text-muted); font-size: 0.875rem; }
.long-term-details span:last-of-type { flex-basis: 100%; }
.long-term-details strong { color: var(--text); }
</style>
