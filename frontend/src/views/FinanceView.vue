<script setup lang="ts">
import { ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { monthKey, shiftMonth } from '@/utils/month'
import FinanceOverviewTab from '@/views/finance/FinanceOverviewTab.vue'
import FinanceTransactionsTab from '@/views/finance/FinanceTransactionsTab.vue'
import FinanceRecurringTab from '@/views/finance/FinanceRecurringTab.vue'
import FinanceCategoriesTab from '@/views/finance/FinanceCategoriesTab.vue'
import FinanceBreakdownTab from '@/views/finance/FinanceBreakdownTab.vue'
import { FINANCE_TABS, type FinanceTab } from '@/views/finance/helpers'
import type {
  FinanceCategory,
  FinanceTransaction,
  FinanceRecurringRule as RecurringRule,
  FinanceSummary,
} from '@/api/types/finance'

const { pid } = useCurrentProperty()
const month = ref(monthKey(new Date()))
const tab = ref<FinanceTab>('overview')

const loading = ref(false)
const busy = ref(false)
const importingPayouts = ref(false)
const error = ref('')
const toast = useToast()
const { confirm } = useConfirm()

const editDialogOpen = ref(false)
const editingTx = ref<FinanceTransaction | null>(null)
const editAmountEur = ref('')
const editNote = ref('')
const editSubmitting = ref(false)
const editError = ref('')

const categories = ref<FinanceCategory[]>([])
const transactions = ref<FinanceTransaction[]>([])
const recurringRules = ref<RecurringRule[]>([])
const summary = ref<FinanceSummary | null>(null)

const txFilterDirection = ref<'' | 'incoming' | 'outgoing'>('')
const txFilterCategory = ref<number>(0)

const txForm = ref({
  transaction_date: new Date().toISOString().slice(0, 10),
  direction: 'incoming' as 'incoming' | 'outgoing',
  amount_eur: 0,
  category_id: 0,
  note: '',
  attachment: null as File | null,
})

const payoutImportFile = ref<File | null>(null)

const categoryForm = ref({
  code: '',
  title: '',
  direction: 'outgoing' as 'incoming' | 'outgoing' | 'both',
  counts_toward_property_income: false,
})

const recurringForm = ref({
  title: '',
  category_id: 0,
  amount_eur: 0,
  direction: 'outgoing' as 'incoming' | 'outgoing',
  start_month: monthKey(new Date()),
  end_month: '',
  effective_from: new Date().toISOString().slice(0, 16),
})

async function loadAll() {
  if (!pid.value) return
  loading.value = true
  error.value = ''
  try {
    const [cats, txs, sum, rules] = await Promise.all([
      api<{ categories: FinanceCategory[] }>(`/api/properties/${pid.value}/finance/categories`),
      api<{ transactions: FinanceTransaction[] }>(
        `/api/properties/${pid.value}/finance/transactions?month=${encodeURIComponent(month.value)}`,
      ),
      api<FinanceSummary>(
        `/api/properties/${pid.value}/finance/summary?month=${encodeURIComponent(month.value)}`,
      ),
      api<{ rules: RecurringRule[] }>(`/api/properties/${pid.value}/finance/recurring-rules`),
    ])
    categories.value = cats.categories
    transactions.value = txs.transactions
    summary.value = sum
    recurringRules.value = rules.rules
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load finance data'
  } finally {
    loading.value = false
  }
}

async function createTransaction() {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  try {
    const fd = new FormData()
    fd.append('transaction_date', txForm.value.transaction_date)
    fd.append('direction', txForm.value.direction)
    fd.append('amount_cents', String(Math.round((txForm.value.amount_eur || 0) * 100)))
    if (txForm.value.category_id > 0) fd.append('category_id', String(txForm.value.category_id))
    if (txForm.value.note.trim()) fd.append('note', txForm.value.note.trim())
    if (txForm.value.attachment) fd.append('attachment', txForm.value.attachment)
    await api(`/api/properties/${pid.value}/finance/transactions`, { method: 'POST', body: fd })
    txForm.value.note = ''
    txForm.value.amount_eur = 0
    txForm.value.attachment = null
    toast.success('Transaction created.')
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to create transaction'
    error.value = msg
    toast.error(msg)
  } finally {
    busy.value = false
  }
}

async function importBookingPayoutCSV() {
  if (!pid.value || !payoutImportFile.value) return
  importingPayouts.value = true
  error.value = ''
  try {
    const fd = new FormData()
    fd.append('file', payoutImportFile.value)
    const r = await api<{
      ok: boolean
      imported: number
      duplicates: number
      mapped: number
      failed: number
      warnings?: string[]
    }>(`/api/properties/${pid.value}/finance/booking-payouts/import`, { method: 'POST', body: fd })
    const warningNote = r.warnings && r.warnings.length ? ` Warnings: ${r.warnings.length}.` : ''
    toast.success(
      `Imported ${r.imported}, duplicates ${r.duplicates}, mapped ${r.mapped}, failed ${r.failed}.${warningNote}`,
      'CSV import done',
    )
    payoutImportFile.value = null
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to import Booking.com payout CSV'
    error.value = msg
    toast.error(msg)
  } finally {
    importingPayouts.value = false
  }
}

function editTransaction(tx: FinanceTransaction) {
  if (!pid.value || tx.is_auto_generated) return
  editingTx.value = tx
  editAmountEur.value = ((tx.amount_cents || 0) / 100).toFixed(2)
  editNote.value = tx.note || ''
  editError.value = ''
  editDialogOpen.value = true
}

async function submitEditTransaction() {
  const tx = editingTx.value
  if (!tx || !pid.value) return
  const amount = Number(editAmountEur.value)
  if (!Number.isFinite(amount)) {
    editError.value = 'Amount must be a number.'
    return
  }
  editSubmitting.value = true
  editError.value = ''
  try {
    await api(`/api/properties/${pid.value}/finance/transactions/${tx.id}`, {
      method: 'PATCH',
      json: { amount_cents: Math.round(amount * 100), note: editNote.value },
    })
    toast.success('Transaction updated.')
    editDialogOpen.value = false
    editingTx.value = null
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to update transaction'
    editError.value = msg
    toast.error(msg)
  } finally {
    editSubmitting.value = false
  }
}

async function deleteTransaction(tx: FinanceTransaction) {
  if (!pid.value || tx.is_auto_generated) return
  const ok = await confirm({
    title: 'Delete transaction',
    message: 'Delete this transaction? This cannot be undone.',
    confirmLabel: 'Delete',
    tone: 'danger',
  })
  if (!ok) return
  try {
    await api(`/api/properties/${pid.value}/finance/transactions/${tx.id}`, { method: 'DELETE' })
    toast.success('Transaction deleted.')
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to delete transaction'
    error.value = msg
    toast.error(msg)
  }
}

async function createCategory() {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  try {
    await api(`/api/properties/${pid.value}/finance/categories`, {
      method: 'POST',
      json: { ...categoryForm.value },
    })
    categoryForm.value.code = ''
    categoryForm.value.title = ''
    toast.success('Category created.')
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to create category'
    error.value = msg
    toast.error(msg)
  } finally {
    busy.value = false
  }
}

async function createRecurringRule() {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  try {
    await api(`/api/properties/${pid.value}/finance/recurring-rules`, {
      method: 'POST',
      json: {
        title: recurringForm.value.title,
        category_id: recurringForm.value.category_id || null,
        amount_cents: Math.round((recurringForm.value.amount_eur || 0) * 100),
        direction: recurringForm.value.direction,
        start_month: recurringForm.value.start_month,
        end_month: recurringForm.value.end_month || null,
        effective_from: new Date(recurringForm.value.effective_from).toISOString(),
      },
    })
    recurringForm.value.title = ''
    recurringForm.value.amount_eur = 0
    recurringForm.value.end_month = ''
    toast.success('Recurring rule created.')
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to create recurring rule'
    error.value = msg
    toast.error(msg)
  } finally {
    busy.value = false
  }
}

async function toggleRecurringRule(rule: RecurringRule) {
  if (!pid.value) return
  try {
    await api(`/api/properties/${pid.value}/finance/recurring-rules/${rule.id}`, {
      method: 'PATCH',
      json: { active: !rule.active },
    })
    toast.success(rule.active ? 'Recurring rule deactivated.' : 'Recurring rule activated.')
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to update recurring rule'
    error.value = msg
    toast.error(msg)
  }
}

async function openMonth() {
  if (!pid.value) return
  busy.value = true
  error.value = ''
  try {
    await api(`/api/properties/${pid.value}/finance/months/${month.value}/open`, { method: 'POST' })
    toast.success(`Month ${month.value} opened and recurring entries synchronized.`)
    await loadAll()
  } catch (e) {
    const msg = e instanceof Error ? e.message : 'Failed to open month'
    error.value = msg
    toast.error(msg)
  } finally {
    busy.value = false
  }
}

function prevMonth() {
  month.value = shiftMonth(month.value, -1)
}
function nextMonth() {
  month.value = shiftMonth(month.value, 1)
}

watch(
  [pid, month],
  () => {
    loadAll().catch(() => {})
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <UiPageHeader title="Finance" lede="Ledger, recurring rules, and monthly close." />

    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to load finance data."
    />

    <template v-else>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />

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
          <UiButton variant="primary" :loading="busy" @click="openMonth">Open month</UiButton>
        </template>
      </UiToolbar>

      <UiTabs
        :model-value="tab"
        :tabs="FINANCE_TABS"
        aria-label="Finance workspace"
        @update:model-value="(v) => (tab = v as FinanceTab)"
      />

      <FinanceOverviewTab v-if="tab === 'overview'" :summary="summary" />

      <FinanceTransactionsTab
        v-if="tab === 'transactions'"
        v-model:tx-form="txForm"
        :transactions="transactions"
        :categories="categories"
        :payout-import-file="payoutImportFile"
        :tx-filter-direction="txFilterDirection"
        :tx-filter-category="txFilterCategory"
        :busy="busy"
        :importing-payouts="importingPayouts"
        @update:payout-import-file="payoutImportFile = $event"
        @update:tx-filter-direction="txFilterDirection = $event"
        @update:tx-filter-category="txFilterCategory = $event"
        @submit="createTransaction"
        @import-payouts="importBookingPayoutCSV"
        @edit="editTransaction"
        @delete="deleteTransaction"
      />

      <FinanceRecurringTab
        v-if="tab === 'recurring'"
        v-model:recurring-form="recurringForm"
        :rules="recurringRules"
        :categories="categories"
        :busy="busy"
        @submit="createRecurringRule"
        @toggle="toggleRecurringRule"
      />

      <FinanceCategoriesTab
        v-if="tab === 'categories'"
        v-model:category-form="categoryForm"
        :categories="categories"
        :busy="busy"
        @submit="createCategory"
      />

      <FinanceBreakdownTab v-if="tab === 'breakdown'" :summary="summary" />
    </template>

    <UiDialog
      :open="editDialogOpen"
      title="Edit transaction"
      size="sm"
      @update:open="editDialogOpen = $event"
    >
      <form class="edit-tx-form" @submit.prevent="submitEditTransaction">
        <UiInput v-model="editAmountEur" type="number" label="Amount (EUR)" />
        <UiInput v-model="editNote" label="Note" />
        <UiInlineBanner v-if="editError" tone="danger" :message="editError" />
      </form>
      <template #footer>
        <UiButton variant="secondary" :disabled="editSubmitting" @click="editDialogOpen = false">Cancel</UiButton>
        <UiButton variant="primary" :loading="editSubmitting" @click="submitEditTransaction">Save</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<style scoped>
.edit-tx-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
</style>
