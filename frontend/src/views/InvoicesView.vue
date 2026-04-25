<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Plus, RefreshCw } from 'lucide-vue-next'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import InvoiceList from '@/views/invoices/InvoiceList.vue'
import InvoiceEditorForm from '@/views/invoices/InvoiceEditorForm.vue'
import InvoiceFilesTable from '@/views/invoices/InvoiceFilesTable.vue'
import { today, payoutBillableCents } from '@/views/invoices/format'
import type {
  Invoice,
  InvoicePreview,
  InvoiceOccupancyOption as OccupancyOption,
  InvoiceBookingPayoutOption as BookingPayoutOption,
} from '@/api/types/invoice'

const { pid, currentProperty } = useCurrentProperty()

const invoices = ref<Invoice[]>([])
const occupancyOptions = ref<OccupancyOption[]>([])
const payoutOptions = ref<BookingPayoutOption[]>([])
const selectedId = ref<number | null>(null)
const selectedInvoice = ref<Invoice | null>(null)
const preview = ref<InvoicePreview | null>(null)
const loading = ref(false)
const saving = ref(false)
const regenerating = ref(false)
const error = ref('')
const success = ref('')

const form = ref({
  occupancy_id: '',
  booking_payout_id: '',
  language: 'sk' as 'sk' | 'en',
  issue_date: today(),
  taxable_supply_date: today(),
  due_date: today(),
  stay_start_date: today(),
  stay_end_date: today(),
  amount_eur: 0,
  payment_note: 'Already paid via Booking.com.',
  customer: {
    name: '',
    company_name: '',
    address_line_1: '',
    city: '',
    postal_code: '',
    country: '',
    vat_id: '',
  },
})

const isEditing = computed(() => selectedId.value !== null)

function resetForm() {
  const defaultLanguage = currentProperty.value?.default_language === 'en' ? 'en' : 'sk'
  form.value = {
    occupancy_id: '',
    booking_payout_id: '',
    language: defaultLanguage,
    issue_date: today(),
    taxable_supply_date: today(),
    due_date: today(),
    stay_start_date: today(),
    stay_end_date: today(),
    amount_eur: 0,
    payment_note: 'Already paid via Booking.com.',
    customer: {
      name: '', company_name: '', address_line_1: '', city: '',
      postal_code: '', country: '', vat_id: '',
    },
  }
}

function applyInvoiceToForm(invoice: Invoice) {
  form.value = {
    occupancy_id: invoice.occupancy_id ? String(invoice.occupancy_id) : '',
    booking_payout_id: invoice.booking_payout_id ? String(invoice.booking_payout_id) : '',
    language: invoice.language,
    issue_date: invoice.issue_date.slice(0, 10),
    taxable_supply_date: invoice.taxable_supply_date.slice(0, 10),
    due_date: invoice.due_date.slice(0, 10),
    stay_start_date: invoice.stay_start_date.slice(0, 10),
    stay_end_date: invoice.stay_end_date.slice(0, 10),
    amount_eur: invoice.amount_total_cents / 100,
    payment_note: invoice.payment_note,
    customer: {
      name: invoice.customer.name || '',
      company_name: invoice.customer.company_name || '',
      address_line_1: invoice.customer.address_line_1 || '',
      city: invoice.customer.city || '',
      postal_code: invoice.customer.postal_code || '',
      country: invoice.customer.country || '',
      vat_id: invoice.customer.vat_id || '',
    },
  }
}

async function loadList() {
  if (!pid.value) {
    invoices.value = []
    selectedId.value = null
    selectedInvoice.value = null
    preview.value = null
    resetForm()
    return
  }
  loading.value = true
  error.value = ''
  try {
    const [list, nextPreview, occRes, payRes] = await Promise.all([
      api<{ invoices: Invoice[] }>(`/api/properties/${pid.value}/invoices`),
      api<InvoicePreview>(
        `/api/properties/${pid.value}/invoice-sequence/next-preview?year=${form.value.issue_date.slice(0, 4)}`,
      ),
      api<{ occupancies: OccupancyOption[] }>(
        `/api/properties/${pid.value}/invoices/occupancy-candidates?limit=120`,
      ).catch(() => ({ occupancies: [] as OccupancyOption[] })),
      api<{ payouts: BookingPayoutOption[] }>(
        `/api/properties/${pid.value}/invoices/payout-link-candidates`,
      ).catch(() => ({ payouts: [] as BookingPayoutOption[] })),
    ])
    invoices.value = list.invoices
    preview.value = nextPreview
    occupancyOptions.value = occRes.occupancies ?? []
    payoutOptions.value = payRes.payouts ?? []
    if (selectedId.value) {
      await loadInvoice(selectedId.value)
    } else {
      selectedInvoice.value = null
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load invoices'
  } finally {
    loading.value = false
  }
}

async function loadInvoice(id: number) {
  if (!pid.value) return
  try {
    const response = await api<{ invoice: Invoice }>(`/api/properties/${pid.value}/invoices/${id}`)
    selectedInvoice.value = response.invoice
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load invoice details'
  }
}

async function refreshPreview() {
  if (!pid.value) {
    preview.value = null
    return
  }
  try {
    preview.value = await api<InvoicePreview>(
      `/api/properties/${pid.value}/invoice-sequence/next-preview?year=${form.value.issue_date.slice(0, 4)}`,
    )
  } catch {
    preview.value = null
  }
}

function selectInvoice(invoice: Invoice) {
  selectedId.value = invoice.id
  applyInvoiceToForm(invoice)
  void loadInvoice(invoice.id)
  success.value = ''
  error.value = ''
}

function onStaySelect(value: string) {
  if (!value) return
  const o = occupancyOptions.value.find((x) => String(x.id) === value)
  if (!o) return
  form.value.occupancy_id = String(o.id)
}

function onPayoutSelect(value: string) {
  if (!value) {
    form.value.booking_payout_id = ''
    return
  }
  const row = payoutOptions.value.find((x) => String(x.id) === value)
  if (!row) return
  form.value.booking_payout_id = String(row.id)
  form.value.amount_eur = payoutBillableCents(row) / 100
  const cin = row.check_in_date?.slice(0, 10)
  const cout = row.check_out_date?.slice(0, 10)
  if (cin) form.value.stay_start_date = cin
  if (cout) form.value.stay_end_date = cout
  if (row.occupancy_id) form.value.occupancy_id = String(row.occupancy_id)
  const gn = row.guest_name?.trim()
  if (gn && !form.value.customer.name.trim()) form.value.customer.name = gn
}

function startNewInvoice() {
  selectedId.value = null
  selectedInvoice.value = null
  resetForm()
  success.value = ''
  error.value = ''
  void refreshPreview()
}

function invoicePayload() {
  const p: Record<string, unknown> = {
    language: form.value.language,
    issue_date: form.value.issue_date,
    taxable_supply_date: form.value.taxable_supply_date,
    due_date: form.value.due_date,
    stay_start_date: form.value.stay_start_date,
    stay_end_date: form.value.stay_end_date,
    amount_total_cents: Math.round((form.value.amount_eur || 0) * 100),
    payment_note: form.value.payment_note,
    customer: {
      name: form.value.customer.name.trim(),
      company_name: form.value.customer.company_name.trim(),
      address_line_1: form.value.customer.address_line_1.trim(),
      city: form.value.customer.city.trim(),
      postal_code: form.value.customer.postal_code.trim(),
      country: form.value.customer.country.trim(),
      vat_id: form.value.customer.vat_id.trim(),
    },
  }
  const oid = form.value.occupancy_id.trim()
  if (oid) p.occupancy_id = Number(oid)
  else if (selectedId.value && selectedInvoice.value?.occupancy_id) p.occupancy_id = 0
  const bid = form.value.booking_payout_id.trim()
  if (bid) p.booking_payout_id = Number(bid)
  else if (selectedId.value && selectedInvoice.value?.booking_payout_id) p.booking_payout_id = 0
  return p
}

async function saveInvoice() {
  if (!pid.value) return
  saving.value = true
  error.value = ''
  success.value = ''
  try {
    const payload = invoicePayload()
    if (selectedId.value) {
      const response = await api<{ invoice: Invoice }>(
        `/api/properties/${pid.value}/invoices/${selectedId.value}`,
        { method: 'PATCH', json: payload },
      )
      selectedInvoice.value = response.invoice
      success.value = 'Invoice updated and regenerated.'
    } else {
      const response = await api<{ invoice: Invoice }>(`/api/properties/${pid.value}/invoices`, {
        method: 'POST',
        json: payload,
      })
      selectedId.value = response.invoice.id
      selectedInvoice.value = response.invoice
      success.value = 'Invoice created.'
    }
    await loadList()
    if (selectedInvoice.value) applyInvoiceToForm(selectedInvoice.value)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save invoice'
  } finally {
    saving.value = false
  }
}

async function regenerateSelectedInvoice() {
  if (!pid.value || !selectedId.value) return
  regenerating.value = true
  error.value = ''
  success.value = ''
  try {
    const response = await api<{ invoice: Invoice }>(
      `/api/properties/${pid.value}/invoices/${selectedId.value}/regenerate`,
      { method: 'POST' },
    )
    selectedInvoice.value = response.invoice
    success.value = 'Invoice PDF regenerated.'
    await loadList()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to regenerate invoice'
  } finally {
    regenerating.value = false
  }
}

function downloadInvoice(invoice?: Invoice | null) {
  if (!invoice) return
  const base = typeof window !== 'undefined' ? window.location.origin : ''
  window.location.assign(`${base}${invoice.download_url}`)
}

watch(
  pid,
  () => {
    startNewInvoice()
    void loadList()
  },
  { immediate: true },
)

watch(
  () => form.value.issue_date,
  () => {
    if (!selectedId.value) void refreshPreview()
  },
)
</script>

<template>
  <div>
    <UiPageHeader
      title="Invoices"
      lede="Create manual stay invoices, download PDFs, and keep version history per property."
    >
      <template #actions>
        <UiButton variant="secondary" :loading="loading" @click="loadList">
          <template #iconLeft><RefreshCw :size="14" aria-hidden="true" /></template>
          Refresh
        </UiButton>
        <UiButton variant="primary" @click="startNewInvoice">
          <template #iconLeft><Plus :size="14" aria-hidden="true" /></template>
          New invoice
        </UiButton>
      </template>
    </UiPageHeader>

    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to manage invoices."
    />

    <template v-else>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />
      <UiInlineBanner v-if="success" tone="success" :title="success" />

      <div class="invoice-layout">
        <InvoiceList
          :invoices="invoices"
          :selected-id="selectedId"
          :is-editing="isEditing"
          :loading="loading"
          :preview="preview"
          @select="selectInvoice"
        />

        <InvoiceEditorForm
          v-model:form="form"
          :is-editing="isEditing"
          :saving="saving"
          :regenerating="regenerating"
          :selected-id="selectedId"
          :selected-invoice="selectedInvoice"
          :occupancy-options="occupancyOptions"
          :payout-options="payoutOptions"
          @submit="saveInvoice"
          @select-stay="onStaySelect"
          @select-payout="onPayoutSelect"
          @download="downloadInvoice"
          @regenerate="regenerateSelectedInvoice"
        />
      </div>

      <InvoiceFilesTable
        v-if="selectedInvoice?.files?.length"
        :files="selectedInvoice.files"
      />
    </template>
  </div>
</template>

<style scoped>
.invoice-layout {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
}
@media (max-width: 960px) {
  .invoice-layout {
    grid-template-columns: 1fr;
  }
}
</style>
