<script setup lang="ts">
import { computed } from 'vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import { formatEuros } from '@/utils/format'
import type { FinanceSummary } from '@/api/types/finance'

const props = defineProps<{ summary: FinanceSummary | null }>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)
const monthlyNetPositive = computed(() => (props.summary?.monthly_net_cents || 0) >= 0)
const monthlyNetLabel = computed(() =>
  (props.summary?.monthly_net_cents || 0) >= 0 ? 'Profitable' : 'Loss',
)
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
</style>
