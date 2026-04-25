<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ExternalLink } from 'lucide-vue-next'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiFileInput from '@/components/ui/UiFileInput.vue'
import { formatShortDate, isoTitle, formatEuros } from '@/utils/format'
import { displayDirection, displaySource, directionTone, type TxForm } from './helpers'
import type { FinanceCategory, FinanceTransaction } from '@/api/types/finance'

const props = defineProps<{
  transactions: FinanceTransaction[]
  categories: FinanceCategory[]
  payoutImportFile: File | null
  txFilterDirection: '' | 'incoming' | 'outgoing'
  txFilterCategory: number
  busy: boolean
  importingPayouts: boolean
}>()

const txForm = defineModel<TxForm>('txForm', { required: true })

const emit = defineEmits<{
  'update:payoutImportFile': [value: File | null]
  'update:txFilterDirection': [value: '' | 'incoming' | 'outgoing']
  'update:txFilterCategory': [value: number]
  submit: []
  importPayouts: []
  edit: [tx: FinanceTransaction]
  delete: [tx: FinanceTransaction]
}>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)

const filteredTransactions = computed(() => {
  return props.transactions.filter((t) => {
    if (props.txFilterDirection && t.direction !== props.txFilterDirection) return false
    if (props.txFilterCategory && t.category_id !== props.txFilterCategory) return false
    return true
  })
})
</script>

<template>
  <div>
    <UiSection
      title="Booking.com payout import"
      description="Upload a payout CSV. Each row imports the net amount as Booking.com income; reference numbers prevent duplicates."
    >
      <template #actions>
        <RouterLink to="/finance/booking-payouts" class="action-link">
          Open booking payouts
          <ExternalLink :size="14" aria-hidden="true" />
        </RouterLink>
      </template>
      <UiCard>
        <div class="payout-import">
          <UiFileInput
            :model-value="payoutImportFile"
            label="CSV file"
            accept=".csv,text/csv"
            button-label="Choose CSV"
            @update:model-value="emit('update:payoutImportFile', $event)"
          />
          <UiButton
            variant="primary"
            :loading="importingPayouts"
            :disabled="!payoutImportFile"
            @click="emit('importPayouts')"
          >Import payout CSV</UiButton>
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="Add transaction">
      <UiCard>
        <form class="form-grid" @submit.prevent="emit('submit')">
          <UiInput v-model="txForm.transaction_date" type="date" label="Date" />
          <UiSelect v-model="txForm.direction" label="Direction">
            <option value="incoming">Incoming</option>
            <option value="outgoing">Outgoing</option>
          </UiSelect>
          <UiInput v-model.number="txForm.amount_eur" type="number" label="Amount (EUR)" />
          <UiSelect v-model.number="txForm.category_id" label="Category">
            <option :value="0">Uncategorized</option>
            <option
              v-for="c in categories.filter((x) => x.direction === 'both' || x.direction === txForm.direction)"
              :key="c.id"
              :value="c.id"
            >{{ c.title }}</option>
          </UiSelect>
          <UiInput v-model="txForm.note" label="Note" class="form-grid__wide" />
          <UiFileInput v-model="txForm.attachment" label="Attachment" />
          <div class="form-grid__full form-actions">
            <UiButton type="submit" variant="primary" :loading="busy">Add transaction</UiButton>
          </div>
        </form>
      </UiCard>
    </UiSection>

    <UiSection title="Transactions">
      <template #actions>
        <UiSelect
          :model-value="txFilterDirection"
          aria-label="Filter by direction"
          @update:model-value="emit('update:txFilterDirection', $event as '' | 'incoming' | 'outgoing')"
        >
          <option value="">All directions</option>
          <option value="incoming">Incoming</option>
          <option value="outgoing">Outgoing</option>
        </UiSelect>
        <UiSelect
          :model-value="txFilterCategory"
          aria-label="Filter by category"
          @update:model-value="emit('update:txFilterCategory', Number($event))"
        >
          <option :value="0">All categories</option>
          <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.title }}</option>
        </UiSelect>
      </template>
      <UiTable
        sticky-header
        :empty="!filteredTransactions.length"
        empty-text="No transactions match these filters."
      >
        <template #head>
          <tr>
            <th>Date</th>
            <th>Direction</th>
            <th class="num">Amount</th>
            <th>Category</th>
            <th>Note</th>
            <th>Source</th>
            <th>Stay mapped</th>
            <th>Attachment</th>
            <th aria-label="Actions" />
          </tr>
        </template>
        <tr v-for="t in filteredTransactions" :key="t.id">
          <td>
            <time :datetime="t.transaction_date" :title="isoTitle(t.transaction_date)">{{ formatShortDate(t.transaction_date) }}</time>
          </td>
          <td>
            <UiBadge :tone="directionTone(t.direction)" dot>{{ displayDirection(t.direction) }}</UiBadge>
          </td>
          <td class="num">{{ eur(t.amount_cents) }}</td>
          <td>{{ t.category_title || '—' }}</td>
          <td class="note-cell" :title="t.note || ''">{{ t.note || '—' }}</td>
          <td class="muted">{{ displaySource(t.source_type) }}</td>
          <td>
            <UiBadge
              v-if="t.source_type === 'booking_payout'"
              :tone="t.mapped_to_stay ? 'success' : 'warning'"
              dot
            >{{ t.mapped_to_stay ? 'Yes' : 'No' }}</UiBadge>
            <span v-else class="muted">—</span>
          </td>
          <td class="muted">{{ t.attachment_path || '—' }}</td>
          <td class="row-actions">
            <UiButton v-if="!t.is_auto_generated" variant="ghost" size="sm" @click="emit('edit', t)">Edit</UiButton>
            <UiButton v-if="!t.is_auto_generated" variant="danger" size="sm" @click="emit('delete', t)">Delete</UiButton>
          </td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  align-items: end;
}
.form-grid__full { grid-column: 1 / -1; }
.form-grid__wide { grid-column: span 2; }
.form-actions { display: flex; justify-content: flex-end; }
.action-link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--font-size-sm);
  color: var(--color-primary);
}
.action-link:hover { text-decoration: none; }
.payout-import {
  display: flex;
  align-items: flex-end;
  gap: var(--space-3);
  flex-wrap: wrap;
}
.row-actions { display: flex; gap: var(--space-2); justify-content: flex-end; }
.note-cell {
  max-width: 28ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
