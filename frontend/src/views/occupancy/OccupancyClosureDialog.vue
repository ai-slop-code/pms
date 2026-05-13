<script setup lang="ts">
// PMS_14 manual labelling dialog. Shared between "Mark closed" (mode='close')
// and "Mark externally sold" (mode='external_sale') so the validation,
// keyboard wiring, and slot semantics stay in one place.
import { computed, ref, watch } from 'vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { closureCategories, externalChannels } from './closure'

type Mode = 'close' | 'external_sale'

const props = defineProps<{
  open: boolean
  mode: Mode
  /** Stay summary shown in the header (e.g. "Apr 12 → Apr 14, Booking Block"). */
  stayLabel?: string
  busy?: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'submit', payload: SubmitPayload): void
}>()

export interface SubmitPayload {
  reason: string
  // close-only:
  category?: string
  // external-sale-only:
  net_amount_cents?: number
  currency?: string
  channel?: string
}

const reason = ref('')
const category = ref('')
const amountStr = ref('')
const currency = ref('EUR')
const channel = ref('')

watch(
  () => props.open,
  (v) => {
    if (v) {
      reason.value = ''
      category.value = ''
      amountStr.value = ''
      currency.value = 'EUR'
      channel.value = ''
    }
  },
)

const title = computed(() =>
  props.mode === 'close' ? 'Mark night as closed' : 'Mark night as externally sold',
)

const amountCents = computed(() => {
  const n = Number(amountStr.value.replace(',', '.'))
  if (!Number.isFinite(n) || n < 0) return null
  return Math.round(n * 100)
})

const submitDisabled = computed(() => {
  if (props.busy) return true
  if (reason.value.length > 500) return true
  if (props.mode === 'external_sale') {
    if (amountCents.value === null) return true
  }
  return false
})

function onSubmit() {
  if (submitDisabled.value) return
  if (props.mode === 'close') {
    emit('submit', {
      reason: reason.value.trim(),
      category: category.value || undefined,
    })
    return
  }
  emit('submit', {
    reason: reason.value.trim(),
    net_amount_cents: amountCents.value ?? 0,
    currency: currency.value.trim() || undefined,
    channel: channel.value || undefined,
  })
}
</script>

<template>
  <UiDialog :open="props.open" :title="title" size="md" @update:open="emit('update:open', $event)">
    <p v-if="props.stayLabel" class="closure-dialog__stay">
      {{ props.stayLabel }}
    </p>

    <div class="closure-dialog__fields">
      <template v-if="props.mode === 'close'">
        <UiSelect v-model="category" label="Category (optional)">
          <option value="">—</option>
          <option v-for="c in closureCategories" :key="c.value" :value="c.value">{{ c.label }}</option>
        </UiSelect>
      </template>

      <template v-else>
        <UiInput
          v-model="amountStr"
          label="Net amount"
          inputmode="decimal"
          placeholder="0.00"
          required
        />
        <UiInput v-model="currency" label="Currency" placeholder="EUR" maxlength="3" />
        <UiSelect v-model="channel" label="Channel (optional)">
          <option value="">—</option>
          <option v-for="c in externalChannels" :key="c.value" :value="c.value">{{ c.label }}</option>
        </UiSelect>
      </template>

      <label class="closure-dialog__field">
        <span class="closure-dialog__label">Note (optional)</span>
        <textarea
          v-model="reason"
          rows="3"
          maxlength="500"
          class="closure-dialog__textarea"
        ></textarea>
        <span class="closure-dialog__hint">{{ reason.length }}/500</span>
      </label>
    </div>

    <p v-if="props.errorMessage" class="closure-dialog__error" role="alert">
      {{ props.errorMessage }}
    </p>

    <template #footer>
      <UiButton variant="ghost" @click="emit('update:open', false)">Cancel</UiButton>
      <UiButton variant="primary" :disabled="submitDisabled" @click="onSubmit">
        {{ props.mode === 'close' ? 'Mark closed' : 'Mark externally sold' }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<style scoped>
.closure-dialog__stay {
  margin: 0 0 var(--space-3);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.closure-dialog__fields {
  display: grid;
  gap: var(--space-3);
}
.closure-dialog__error {
  margin-top: var(--space-3);
  color: var(--color-text-danger);
  font-size: var(--font-size-sm);
}
.closure-dialog__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.closure-dialog__label {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
}
.closure-dialog__textarea {
  font: inherit;
  padding: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  resize: vertical;
}
.closure-dialog__hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  align-self: flex-end;
}
</style>
