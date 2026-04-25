<script setup lang="ts">
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { displayDirection, type CategoryForm } from './helpers'
import type { FinanceCategory } from '@/api/types/finance'

defineProps<{
  categories: FinanceCategory[]
  busy: boolean
}>()

const categoryForm = defineModel<CategoryForm>('categoryForm', { required: true })

const emit = defineEmits<{ submit: [] }>()
</script>

<template>
  <div>
    <UiSection title="Add category">
      <UiCard>
        <form class="form-grid" @submit.prevent="emit('submit')">
          <UiInput v-model="categoryForm.code" label="Code" placeholder="e.g. parking_income" />
          <UiInput v-model="categoryForm.title" label="Title" />
          <UiSelect v-model="categoryForm.direction" label="Direction">
            <option value="incoming">Incoming</option>
            <option value="outgoing">Outgoing</option>
            <option value="both">Both</option>
          </UiSelect>
          <label class="checkbox-field">
            <input v-model="categoryForm.counts_toward_property_income" type="checkbox" />
            <span>Counts toward property income</span>
          </label>
          <div class="form-grid__full form-actions">
            <UiButton type="submit" variant="primary" :loading="busy">Add category</UiButton>
          </div>
        </form>
      </UiCard>
    </UiSection>

    <UiSection title="All categories">
      <UiTable :empty="!categories.length" empty-text="No categories yet.">
        <template #head>
          <tr>
            <th>Code</th>
            <th>Title</th>
            <th>Direction</th>
            <th>Income flag</th>
          </tr>
        </template>
        <tr v-for="c in categories" :key="c.id">
          <td class="muted"><code>{{ c.code }}</code></td>
          <td>{{ c.title }}</td>
          <td>{{ displayDirection(c.direction) }}</td>
          <td>
            <UiBadge :tone="c.counts_toward_property_income ? 'success' : 'neutral'" dot>
              {{ c.counts_toward_property_income ? 'Yes' : 'No' }}
            </UiBadge>
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
.form-actions { display: flex; justify-content: flex-end; }
.checkbox-field {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
}
.checkbox-field input[type='checkbox'] {
  width: auto;
  min-height: 0;
  margin: 0;
}
</style>
