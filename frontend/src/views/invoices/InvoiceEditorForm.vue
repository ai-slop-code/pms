<script setup lang="ts">
import { Download } from 'lucide-vue-next'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { occupancyOptionLabel, payoutOptionLabel } from './format'
import type {
  Invoice,
  InvoiceOccupancyOption as OccupancyOption,
  InvoiceBookingPayoutOption as BookingPayoutOption,
} from '@/api/types/invoice'

export interface InvoiceFormState {
  occupancy_id: string
  booking_payout_id: string
  language: 'sk' | 'en'
  issue_date: string
  taxable_supply_date: string
  due_date: string
  stay_start_date: string
  stay_end_date: string
  amount_eur: number
  payment_note: string
  customer: {
    name: string
    company_name: string
    address_line_1: string
    city: string
    postal_code: string
    country: string
    vat_id: string
  }
}

defineProps<{
  isEditing: boolean
  saving: boolean
  regenerating: boolean
  selectedId: number | null
  selectedInvoice: Invoice | null
  occupancyOptions: OccupancyOption[]
  payoutOptions: BookingPayoutOption[]
}>()

const form = defineModel<InvoiceFormState>('form', { required: true })

const emit = defineEmits<{
  submit: []
  selectStay: [value: string]
  selectPayout: [value: string]
  download: [invoice: Invoice | null]
  regenerate: []
}>()
</script>

<template>
  <UiCard class="invoice-editor-card">
    <div class="card-head">
      <h2 class="card-head__title">{{ isEditing ? 'Edit invoice' : 'Create invoice' }}</h2>
      <div class="card-head__actions">
        <UiButton
          v-if="selectedInvoice"
          variant="secondary"
          size="sm"
          :disabled="regenerating"
          @click="emit('download', selectedInvoice)"
        >
          <template #iconLeft><Download :size="14" aria-hidden="true" /></template>
          Download PDF
        </UiButton>
        <UiButton
          v-if="selectedInvoice"
          variant="ghost"
          size="sm"
          :loading="regenerating"
          @click="emit('regenerate')"
        >Regenerate PDF</UiButton>
      </div>
    </div>

    <form @submit.prevent="emit('submit')">
      <div class="form-grid">
        <UiSelect
          :model-value="form.occupancy_id"
          label="Stay (optional)"
          class="form-grid__full"
          @update:model-value="emit('selectStay', String($event))"
        >
          <option value="">— Select stay —</option>
          <option v-for="o in occupancyOptions" :key="o.id" :value="String(o.id)">
            {{ occupancyOptionLabel(o) }}
          </option>
        </UiSelect>
        <UiInput v-model="form.occupancy_id" label="Occupancy ID" inputmode="numeric" placeholder="or type id" />
        <UiSelect
          :model-value="form.booking_payout_id"
          label="Mapped Booking.com payout (optional)"
          class="form-grid__full"
          @update:model-value="emit('selectPayout', String($event))"
        >
          <option value="">— None —</option>
          <option
            v-for="p in payoutOptions"
            :key="p.id"
            :value="String(p.id)"
            :disabled="!!p.linked_invoice_id && p.linked_invoice_id !== selectedId"
          >
            {{ payoutOptionLabel(p) }}
          </option>
        </UiSelect>
        <UiSelect v-model="form.language" label="Language">
          <option value="sk">Slovak</option>
          <option value="en">English</option>
        </UiSelect>
        <UiInput v-model="form.issue_date" type="date" label="Issue date" />
        <UiInput v-model="form.taxable_supply_date" type="date" label="Taxable supply date" />
        <UiInput v-model="form.due_date" type="date" label="Due date" />
        <UiInput
          v-model.number="form.amount_eur"
          type="number"
          min="0"
          step="0.01"
          label="Amount billed (EUR)"
          help="Full guest price"
        />
        <UiInput v-model="form.stay_start_date" type="date" label="Stay start" />
        <UiInput v-model="form.stay_end_date" type="date" label="Stay end" />
        <label class="textarea-field form-grid__full">
          <span class="textarea-field__label">Payment note</span>
          <textarea v-model="form.payment_note" rows="3" />
        </label>
      </div>

      <UiSection title="Customer details">
        <div class="form-grid">
          <UiInput v-model="form.customer.name" label="Name" />
          <UiInput v-model="form.customer.company_name" label="Company name" />
          <UiInput v-model="form.customer.address_line_1" label="Address line 1" />
          <UiInput v-model="form.customer.city" label="City" />
          <UiInput v-model="form.customer.postal_code" label="Postal code" />
          <UiInput v-model="form.customer.country" label="Country" />
          <UiInput v-model="form.customer.vat_id" label="VAT number" />
        </div>
      </UiSection>

      <p v-if="selectedInvoice?.booking_payout_id" class="muted linked-payout">
        Linked Booking.com payout row ID:
        <strong>{{ selectedInvoice.booking_payout_id }}</strong>
      </p>

      <div v-if="selectedInvoice" class="snapshot-card">
        <h3 class="snapshot-card__title">Supplier snapshot</h3>
        <p>
          <strong>{{ selectedInvoice.supplier.company_name || selectedInvoice.supplier.name }}</strong>
        </p>
        <p>{{ selectedInvoice.supplier.address_line_1 || '—' }}</p>
        <p>
          {{ [selectedInvoice.supplier.postal_code, selectedInvoice.supplier.city].filter(Boolean).join(' ') || '—' }}
        </p>
        <p>{{ selectedInvoice.supplier.country || '—' }}</p>
        <p class="muted">
          ICO: {{ selectedInvoice.supplier.ico || '—' }} ·
          DIC: {{ selectedInvoice.supplier.dic || '—' }} ·
          VAT ID: {{ selectedInvoice.supplier.vat_id || '—' }}
        </p>
      </div>

      <div class="form-actions">
        <UiButton type="submit" variant="primary" :loading="saving">
          {{ isEditing ? 'Save invoice' : 'Create invoice' }}
        </UiButton>
      </div>
    </form>
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
.card-head__actions {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}
.form-grid {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
}
.form-grid__full {
  grid-column: 1 / -1;
}
.textarea-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
}
.textarea-field__label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-weight: 500;
}
.textarea-field textarea {
  width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  font: var(--font-size-md) / 1.5 var(--font-family-sans);
  color: var(--color-text);
  background: var(--color-surface);
  resize: vertical;
}
.textarea-field textarea:focus {
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
  outline: none;
}
.snapshot-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  background: var(--color-sunken);
  margin-top: var(--space-4);
}
.snapshot-card__title {
  margin: 0 0 var(--space-2);
  font-size: var(--font-size-md);
  font-weight: 600;
}
.snapshot-card p {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
.linked-payout {
  margin-top: var(--space-3);
}
.form-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-4);
}
</style>
