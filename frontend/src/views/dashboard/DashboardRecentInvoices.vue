<script setup lang="ts">
import { RouterLink } from 'vue-router'
import UiCard from '@/components/ui/UiCard.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import { formatEuros, formatShortDate, isoTitle } from '@/utils/format'
import type { RecentInvoiceWidget } from '@/api/types/dashboard'
import './listRows.css'

defineProps<{ invoices: RecentInvoiceWidget[] }>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)
const fmtDay = formatShortDate
</script>

<template>
  <UiCard>
    <template #header>
      <div class="dashboard-widget-head">
        <h2 class="dashboard-widget-title">Recent invoices</h2>
        <RouterLink to="/invoices" class="dashboard-widget-link">Open</RouterLink>
      </div>
    </template>
    <UiEmptyState
      v-if="!invoices.length"
      illustration="invoice"
      title="No invoices yet"
      description="Generate the first invoice to see it here."
    />
    <ul v-else class="dashboard-list-rows">
      <li v-for="invoice in invoices" :key="invoice.invoice_id" class="dashboard-list-row">
        <div>
          <div class="dashboard-list-row__title">{{ invoice.invoice_number }}</div>
          <div class="dashboard-list-row__meta">
            {{ invoice.customer_name || 'Customer pending' }} ·
            <time :datetime="invoice.issue_date" :title="isoTitle(invoice.issue_date)">{{ fmtDay(invoice.issue_date) }}</time>
          </div>
        </div>
        <div class="dashboard-list-row__side">
          <div class="dashboard-list-row__emphasis dashboard-num">{{ eur(invoice.amount_total_cents) }}</div>
          <div class="dashboard-list-row__meta">v{{ invoice.version }}</div>
        </div>
      </li>
    </ul>
  </UiCard>
</template>
