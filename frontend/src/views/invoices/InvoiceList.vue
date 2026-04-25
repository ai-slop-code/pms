<script setup lang="ts">
import UiCard from '@/components/ui/UiCard.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import { eur, fmtDay } from './format'
import type { Invoice, InvoicePreview } from '@/api/types/invoice'

defineProps<{
  invoices: Invoice[]
  selectedId: number | null
  isEditing: boolean
  loading: boolean
  preview: InvoicePreview | null
}>()

const emit = defineEmits<{ select: [invoice: Invoice] }>()
</script>

<template>
  <UiCard class="invoice-list-card">
    <div class="card-head">
      <h2 class="card-head__title">Invoices</h2>
      <UiBadge tone="neutral">{{ invoices.length }} total</UiBadge>
    </div>
    <p v-if="preview && !isEditing" class="muted">
      Next number: <strong>{{ preview.invoice_number }}</strong>
    </p>
    <div v-if="loading && !invoices.length" class="muted">Loading…</div>
    <UiEmptyState
      v-else-if="!invoices.length"
      illustration="invoice"
      title="No invoices yet"
      description="Generate the first invoice for this property to see it here."
    />
    <div v-else class="invoice-list">
      <button
        v-for="invoice in invoices"
        :key="invoice.id"
        type="button"
        class="invoice-list-item"
        :class="{ active: selectedId === invoice.id }"
        @click="emit('select', invoice)"
      >
        <div class="ili-row">
          <span class="ili-number">{{ invoice.invoice_number }}</span>
          <span class="ili-amount num">{{ eur(invoice.amount_total_cents) }}</span>
        </div>
        <div class="ili-row ili-sub">
          <span>{{ invoice.customer.company_name || invoice.customer.name || 'Customer' }}</span>
          <UiBadge tone="neutral" size="sm">v{{ invoice.version }}</UiBadge>
        </div>
        <div class="ili-row ili-sub muted">
          <span>{{ fmtDay(invoice.stay_start_date) }} – {{ fmtDay(invoice.stay_end_date) }}</span>
        </div>
      </button>
    </div>
  </UiCard>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
  flex-wrap: wrap;
}
.card-head__title {
  margin: 0;
  font-size: var(--font-size-h2);
  font-weight: 600;
}
.invoice-list {
  display: flex;
  flex-direction: column;
}
.invoice-list-item {
  width: 100%;
  border: none;
  border-top: 1px solid var(--color-border);
  border-radius: 0;
  background: transparent;
  padding: var(--space-3) var(--space-2);
  text-align: left;
  display: grid;
  gap: var(--space-1);
  cursor: pointer;
  transition: background var(--motion-1) var(--ease-standard);
  color: var(--color-text);
}
.invoice-list-item:first-child {
  border-top: none;
}
.invoice-list-item:hover {
  background: var(--color-sunken);
}
.invoice-list-item.active {
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  border-left: 2px solid var(--color-primary);
  padding-left: calc(var(--space-2) - 2px);
}
.ili-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.ili-number {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
.ili-amount {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text);
  white-space: nowrap;
}
.ili-sub {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
</style>
