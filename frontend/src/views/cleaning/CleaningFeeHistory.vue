<script setup lang="ts">
import { ref } from 'vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { formatEuros } from '@/utils/format'
import type { CleaningFeeRow } from '@/api/types/cleaning'

defineProps<{ fees: CleaningFeeRow[]; saving: boolean }>()
const emit = defineEmits<{
  submit: [payload: { cleaning_fee_amount_cents: number; washing_fee_amount_cents: number; effective_from: string }]
}>()

const form = ref({
  cleaning_fee_amount_eur: 0,
  washing_fee_amount_eur: 0,
  effective_from: new Date().toISOString().slice(0, 16),
})

const eur = (cents?: number | null) => formatEuros(cents ?? 0)

function onSubmit() {
  emit('submit', {
    cleaning_fee_amount_cents: Math.round((form.value.cleaning_fee_amount_eur || 0) * 100),
    washing_fee_amount_cents: Math.round((form.value.washing_fee_amount_eur || 0) * 100),
    effective_from: new Date(form.value.effective_from).toISOString(),
  })
}
</script>

<template>
  <UiSection title="Fee history">
    <UiCard>
      <form class="fee-form" @submit.prevent="onSubmit">
        <UiInput
          v-model.number="form.cleaning_fee_amount_eur"
          label="Cleaning fee (EUR)"
          type="number"
          min="0"
          step="0.01"
        />
        <UiInput
          v-model.number="form.washing_fee_amount_eur"
          label="Washing fee (EUR)"
          type="number"
          min="0"
          step="0.01"
        />
        <UiInput v-model="form.effective_from" label="Effective from" type="datetime-local" />
        <div class="fee-form__actions">
          <UiButton type="submit" variant="primary" :loading="saving">Add fee</UiButton>
        </div>
      </form>
    </UiCard>

    <UiTable :empty="!fees.length" empty-text="No fee history yet.">
      <template #head>
        <tr>
          <th>Effective from</th>
          <th class="num">Cleaning</th>
          <th class="num">Washing</th>
          <th>Created</th>
        </tr>
      </template>
      <tr v-for="f in fees" :key="f.id">
        <td>{{ f.effective_from }}</td>
        <td class="num">{{ eur(f.cleaning_fee_amount_cents) }}</td>
        <td class="num">{{ eur(f.washing_fee_amount_cents) }}</td>
        <td class="muted">{{ f.created_at }}</td>
      </tr>
    </UiTable>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.fee-form {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  align-items: end;
}
.fee-form__actions {
  display: flex;
  justify-content: flex-end;
  grid-column: 1 / -1;
}
</style>
