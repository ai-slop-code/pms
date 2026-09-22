<script setup lang="ts">
import { ref, watch } from 'vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import { api } from '@/api/http'
import { formatEuros } from '@/utils/format'
import { useConfirm } from '@/composables/useConfirm'
import type { FinanceLongTermRentRate } from '@/api/types/finance'

const props = defineProps<{
  open: boolean
  propertyId: number | null
  selectedMonth: string
  rates: FinanceLongTermRentRate[]
  canManage: boolean
}>()
const emit = defineEmits<{ (e: 'update:open', value: boolean): void; (e: 'changed'): void }>()
const { confirm } = useConfirm()
const editingID = ref<number | null>(null)
const amount = ref('')
const effectiveMonth = ref(props.selectedMonth)
const error = ref('')
const submitting = ref(false)

watch(() => props.selectedMonth, (value) => { if (editingID.value === null) effectiveMonth.value = value })

function resetForm() {
  editingID.value = null
  amount.value = ''
  effectiveMonth.value = props.selectedMonth
  error.value = ''
}

function edit(rate: FinanceLongTermRentRate) {
  editingID.value = rate.id
  amount.value = (rate.monthly_rent_cents / 100).toFixed(2)
  effectiveMonth.value = rate.effective_from_month
  error.value = ''
}

function cents(value: string): number | null {
  const normalized = value.trim().replace(',', '.')
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) return null
  const [whole, fraction = ''] = normalized.split('.')
  const result = Number(whole) * 100 + Number(fraction.padEnd(2, '0'))
  return Number.isSafeInteger(result) && result > 0 ? result : null
}

async function save() {
  error.value = ''
  if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(effectiveMonth.value)) { error.value = 'Effective month must be YYYY-MM.'; return }
  const rentCents = cents(amount.value)
  if (rentCents === null) { error.value = 'Enter a positive amount with at most two decimal places.'; return }
  const requestPropertyID = props.propertyId
  if (!requestPropertyID) return
  submitting.value = true
  try {
    const path = `/api/properties/${requestPropertyID}/finance/long-term-rent-rates${editingID.value ? `/${editingID.value}` : ''}`
    await api(path, { method: editingID.value ? 'PATCH' : 'POST', json: { effective_from_month: effectiveMonth.value, monthly_rent_cents: rentCents } })
    resetForm()
    emit('changed')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save rent rate.'
  } finally { submitting.value = false }
}

async function remove(rate: FinanceLongTermRentRate) {
  const requestPropertyID = props.propertyId
  if (!requestPropertyID) return
  const ok = await confirm({ title: 'Delete rent rate?', message: `Delete the ${formatEuros(rate.monthly_rent_cents)} rate from ${rate.effective_from_month}? The preceding rate may apply instead.`, confirmLabel: 'Delete', tone: 'danger' })
  if (!ok) return
  error.value = ''
  try {
    await api(`/api/properties/${requestPropertyID}/finance/long-term-rent-rates/${rate.id}`, { method: 'DELETE' })
    if (editingID.value === rate.id) resetForm()
    emit('changed')
  } catch (e) { error.value = e instanceof Error ? e.message : 'Failed to delete rent rate.' }
}
</script>

<template>
  <UiDialog :open="open" title="Manage long-term rent" size="md" @update:open="emit('update:open', $event)">
    <p class="intro">Configure the hypothetical monthly rent used for the Finance comparison. Rent includes utilities.</p>
    <div v-if="canManage" class="rate-form">
      <UiInput v-model="amount" label="Monthly rent (EUR)" placeholder="900.00" inputmode="decimal" required />
      <UiInput v-model="effectiveMonth" label="Effective from" type="month" required />
      <UiInlineBanner v-if="error" tone="danger" :title="error" />
      <div class="form-actions">
        <UiButton v-if="editingID" variant="ghost" @click="resetForm">Cancel edit</UiButton>
        <UiButton variant="primary" :loading="submitting" @click="save">{{ editingID ? 'Save changes' : 'Add rate' }}</UiButton>
      </div>
    </div>
    <UiInlineBanner v-else-if="error" tone="danger" :title="error" />
    <section class="history">
      <h3>Rate history</h3>
      <p v-if="!rates.length">No rent rates configured.</p>
      <div v-for="rate in rates" :key="rate.id" class="rate-row">
        <div><strong>{{ rate.effective_from_month }}</strong><span>{{ formatEuros(rate.monthly_rent_cents) }} / month</span></div>
        <div v-if="canManage" class="row-actions"><UiButton size="sm" @click="edit(rate)">Edit</UiButton><UiButton size="sm" variant="danger" @click="remove(rate)">Delete</UiButton></div>
      </div>
    </section>
    <template #footer><UiButton @click="emit('update:open', false)">Close</UiButton></template>
  </UiDialog>
</template>

<style scoped>
.intro, .history p { color: var(--text-muted); }
.rate-form, .history { display: grid; gap: var(--space-3); }
.form-actions, .row-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }
.history { margin-top: var(--space-5); }
.history h3 { margin: 0; font-size: var(--font-size-h4); }
.rate-row { display: flex; justify-content: space-between; align-items: center; gap: var(--space-3); padding: var(--space-3) 0; border-top: 1px solid var(--color-border); }
.rate-row span { display: block; color: var(--text-muted); font-size: var(--font-size-sm); }
@media (max-width: 480px) { .rate-row { align-items: flex-start; flex-direction: column; } .row-actions { justify-content: flex-start; } }
</style>
