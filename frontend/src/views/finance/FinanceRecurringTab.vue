<script setup lang="ts">
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { formatEuros } from '@/utils/format'
import { displayDirection, directionTone, type RecurringForm } from './helpers'
import type { FinanceCategory, FinanceRecurringRule as RecurringRule } from '@/api/types/finance'

defineProps<{
  rules: RecurringRule[]
  categories: FinanceCategory[]
  busy: boolean
}>()

const recurringForm = defineModel<RecurringForm>('recurringForm', { required: true })

const emit = defineEmits<{ submit: []; toggle: [rule: RecurringRule] }>()

const eur = (cents?: number | null) => formatEuros(cents ?? 0)
</script>

<template>
  <div>
    <UiSection title="Add recurring rule">
      <UiCard>
        <form class="form-grid" @submit.prevent="emit('submit')">
          <UiInput v-model="recurringForm.title" label="Title" class="form-grid__wide" />
          <UiInput v-model.number="recurringForm.amount_eur" type="number" label="Amount (EUR)" />
          <UiSelect v-model="recurringForm.direction" label="Direction">
            <option value="incoming">Incoming</option>
            <option value="outgoing">Outgoing</option>
          </UiSelect>
          <UiSelect v-model.number="recurringForm.category_id" label="Category">
            <option :value="0">Uncategorized</option>
            <option
              v-for="c in categories.filter((x) => x.direction === 'both' || x.direction === recurringForm.direction)"
              :key="c.id"
              :value="c.id"
            >{{ c.title }}</option>
          </UiSelect>
          <UiInput v-model="recurringForm.start_month" type="month" label="Start month" />
          <UiInput v-model="recurringForm.end_month" type="month" label="End month (optional)" />
          <UiInput v-model="recurringForm.effective_from" type="datetime-local" label="Effective from" />
          <div class="form-grid__full form-actions">
            <UiButton type="submit" variant="primary" :loading="busy">Add rule</UiButton>
          </div>
        </form>
      </UiCard>
    </UiSection>

    <UiSection title="Rules">
      <UiTable :empty="!rules.length" empty-text="No recurring rules yet.">
        <template #head>
          <tr>
            <th>Title</th>
            <th>Direction</th>
            <th class="num">Amount</th>
            <th>Start</th>
            <th>End</th>
            <th>Status</th>
            <th aria-label="Actions" />
          </tr>
        </template>
        <tr v-for="r in rules" :key="r.id">
          <td>{{ r.title }}</td>
          <td><UiBadge :tone="directionTone(r.direction)">{{ displayDirection(r.direction) }}</UiBadge></td>
          <td class="num">{{ eur(r.amount_cents) }}</td>
          <td>{{ r.start_month }}</td>
          <td>{{ r.end_month || '—' }}</td>
          <td>
            <UiBadge :tone="r.active ? 'success' : 'neutral'" dot>{{ r.active ? 'Active' : 'Inactive' }}</UiBadge>
          </td>
          <td class="row-actions">
            <UiButton variant="ghost" size="sm" @click="emit('toggle', r)">
              {{ r.active ? 'Deactivate' : 'Activate' }}
            </UiButton>
          </td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  align-items: end;
}
.form-grid__full { grid-column: 1 / -1; }
.form-grid__wide { grid-column: span 2; }
.form-actions { display: flex; justify-content: flex-end; }
.row-actions { display: flex; gap: var(--space-2); justify-content: flex-end; }
</style>
