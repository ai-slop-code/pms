<script setup lang="ts">
import { computed } from 'vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import { formatEuros } from '@/utils/format'
import type { CleaningMonthWidget, FinanceMonthWidget, UpcomingStayWidget } from '@/api/types/dashboard'

const props = defineProps<{
  finance?: FinanceMonthWidget
  cleaning?: CleaningMonthWidget
  upcomingStays?: UpcomingStayWidget[]
}>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)

const upcomingCount = computed(() => props.upcomingStays?.length ?? 0)
const upcoming7DayCount = computed(() => {
  const list = props.upcomingStays ?? []
  const now = Date.now()
  const limit = now + 7 * 24 * 60 * 60 * 1000
  return list.filter((s) => {
    const t = Date.parse(s.start_at)
    return !Number.isNaN(t) && t >= now && t <= limit
  }).length
})
</script>

<template>
  <div v-if="finance || cleaning || upcomingStays" class="dashboard-kpis">
    <UiKpiCard
      v-if="finance"
      label="Net cashflow (month)"
      :value="eur(finance.net)"
      :tone="finance.net < 0 ? 'danger' : 'success'"
      hint="Net = incoming − outgoing"
      hero
    />
    <UiKpiCard
      v-if="finance"
      label="Gross revenue (month)"
      :value="eur(finance.incoming)"
      :hint="`Outgoing ${eur(finance.outgoing)}`"
    />
    <UiKpiCard
      v-if="upcomingStays"
      label="Check-ins (next 7 days)"
      :value="String(upcoming7DayCount)"
      :hint="`${upcomingCount} upcoming total`"
    />
    <UiKpiCard
      v-if="cleaning"
      label="Cleaning days (month)"
      :value="String(cleaning.counted_days)"
      :hint="`Salary draft ${eur(cleaning.salary_draft)}`"
    />
  </div>
</template>

<style scoped>
.dashboard-kpis {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-4);
  margin-bottom: var(--space-5);
}
</style>
