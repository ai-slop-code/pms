<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { api } from '@/api/http'
import { APP_NAV_ITEMS } from '@/constants/navigation'
import { useAuthStore } from '@/stores/auth'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiSkeleton from '@/components/ui/UiSkeleton.vue'
import DashboardHeroKpis from '@/views/dashboard/DashboardHeroKpis.vue'
import DashboardAlertsCard from '@/views/dashboard/DashboardAlertsCard.vue'
import type { DashboardAlert } from '@/views/dashboard/alerts'
import DashboardUpcomingStays from '@/views/dashboard/DashboardUpcomingStays.vue'
import DashboardNukiCodes from '@/views/dashboard/DashboardNukiCodes.vue'
import DashboardRecentInvoices from '@/views/dashboard/DashboardRecentInvoices.vue'
import DashboardQuickActions from '@/views/dashboard/DashboardQuickActions.vue'
import { widgetTitle } from '@/views/dashboard/status'

import type { DashboardWidgets } from '@/api/types/dashboard'

const auth = useAuthStore()
const { pid, currentProperty } = useCurrentProperty()
const summary = ref<DashboardWidgets | null>(null)
const error = ref('')
const loading = ref(false)

const hasAnyWidgets = computed(() => {
  const widgets = summary.value
  return !!(
    widgets?.sync_status ||
    widgets?.upcoming_stays ||
    widgets?.active_nuki_codes ||
    widgets?.cleaning_month ||
    widgets?.finance_month ||
    widgets?.recent_invoices
  )
})

const alerts = computed<DashboardAlert[]>(() => {
  const widgets = summary.value
  const out: DashboardAlert[] = []
  if (!widgets) return out
  const sync = widgets.sync_status
  if (sync?.occupancy && ['error', 'partial'].includes(sync.occupancy)) {
    out.push({
      id: 'occ-sync',
      tone: sync.occupancy === 'error' ? 'danger' : 'warning',
      title: 'Occupancy sync issue',
      detail: widgetTitle(sync.occupancy),
      to: '/occupancy',
    })
  }
  if (sync?.nuki && ['error', 'partial'].includes(sync.nuki)) {
    out.push({
      id: 'nuki-sync',
      tone: sync.nuki === 'error' ? 'danger' : 'warning',
      title: 'Nuki access issue',
      detail: widgetTitle(sync.nuki),
      to: '/nuki',
    })
  }
  const pendingCodes = (widgets.active_nuki_codes ?? []).filter(
    (c) => c.status === 'not_generated' || c.status === 'revoked' || !!c.error_message,
  )
  if (pendingCodes.length) {
    out.push({
      id: 'nuki-pending',
      tone: 'warning',
      title: 'Pin-reveal attention',
      detail: `${pendingCodes.length} code${pendingCodes.length === 1 ? '' : 's'} need review`,
      to: '/nuki',
    })
  }
  return out
})

const quickActions = computed(() => {
  const property = currentProperty.value
  return APP_NAV_ITEMS.filter(
    (item) =>
      item.to !== '/' &&
      item.to !== '/finance/booking-payouts' &&
      (!item.module || auth.canAccessPropertyModule(property, item.module)),
  )
})

async function load() {
  error.value = ''
  const id = pid.value
  if (!id) {
    summary.value = null
    return
  }
  loading.value = true
  try {
    const r = await api<{ widgets: DashboardWidgets }>(`/api/properties/${id}/dashboard`)
    summary.value = r.widgets
  } catch (e) {
    summary.value = null
    error.value = e instanceof Error ? e.message : 'Failed to load'
  } finally {
    loading.value = false
  }
}

watch(pid, () => load(), { immediate: true })
</script>

<template>
  <div>
    <UiPageHeader
      title="Dashboard"
      :lede="pid ? 'At-a-glance view for the selected property.' : 'Select a property to load the dashboard.'"
    >
      <template #actions>
        <UiButton variant="secondary" :disabled="!pid" :loading="loading" @click="load">
          <template #iconLeft><RefreshCw :size="16" aria-hidden="true" /></template>
          Refresh
        </UiButton>
      </template>
    </UiPageHeader>

    <UiInlineBanner
      v-if="!pid"
      tone="info"
      title="Select or create a property to see the dashboard."
    />
    <UiInlineBanner v-else-if="error" tone="danger" :title="error" />

    <div v-else-if="loading && !summary" class="dashboard-grid">
      <UiSkeleton variant="kpi" />
      <UiSkeleton variant="kpi" />
      <UiSkeleton variant="kpi" />
    </div>

    <template v-else-if="summary">
      <DashboardHeroKpis
        :finance="summary.finance_month"
        :cleaning="summary.cleaning_month"
        :upcoming-stays="summary.upcoming_stays"
      />

      <div class="dashboard-grid">
        <DashboardAlertsCard :alerts="alerts" :sync-status="summary.sync_status" />

        <DashboardUpcomingStays v-if="summary.upcoming_stays" :stays="summary.upcoming_stays" />

        <DashboardNukiCodes v-if="summary.active_nuki_codes" :codes="summary.active_nuki_codes" />

        <DashboardRecentInvoices v-if="summary.recent_invoices" :invoices="summary.recent_invoices" />
      </div>

      <UiEmptyState
        v-if="!hasAnyWidgets"
        illustration="dashboard"
        title="No widgets available"
        description="Nothing to show for this property and your current access level."
      />

      <DashboardQuickActions :actions="quickActions" />
    </template>
  </div>
</template>

<style scoped>
.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: var(--space-4);
}
</style>
