<script setup lang="ts">
import { computed, useId } from 'vue'

interface Props {
  modelValue?: string | number | null
  label?: string
  help?: string
  error?: string
  required?: boolean
  disabled?: boolean
  id?: string
  name?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  label: '',
  help: '',
  error: '',
  required: false,
  disabled: false,
  id: '',
  name: '',
})

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const autoId = useId()
const fieldId = computed(() => props.id || `ui-select-${autoId}`)
const helpId = computed(() => `${fieldId.value}-help`)
const errorId = computed(() => `${fieldId.value}-error`)
const describedBy = computed(() => {
  const parts: string[] = []
  if (props.help) parts.push(helpId.value)
  if (props.error) parts.push(errorId.value)
  return parts.length ? parts.join(' ') : undefined
})

function onChange(e: Event) {
  emit('update:modelValue', (e.target as HTMLSelectElement).value)
}
</script>

<template>
  <div class="ui-field" :class="{ 'ui-field--error': !!error, 'ui-field--disabled': disabled }">
    <label v-if="label" :for="fieldId" class="ui-field__label">
      {{ label }}<span v-if="required" class="ui-field__required" aria-hidden="true">*</span>
    </label>
    <select
      :id="fieldId"
      class="ui-field__control"
      :value="modelValue ?? ''"
      :required="required || undefined"
      :disabled="disabled || undefined"
      :name="name || undefined"
      :aria-invalid="!!error || undefined"
      :aria-describedby="describedBy"
      @change="onChange"
    >
      <slot />
    </select>
    <p v-if="error" :id="errorId" class="ui-field__message ui-field__message--error">{{ error }}</p>
    <p v-else-if="help" :id="helpId" class="ui-field__message">{{ help }}</p>
  </div>
</template>

<style scoped>
.ui-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.ui-field__label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-muted);
}
.ui-field__required {
  color: var(--danger-fg);
  margin-left: 2px;
}
.ui-field__control {
  width: 100%;
  min-height: 36px;
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: var(--font-size-md) / 1.4 var(--font-family-sans);
  color: var(--color-text);
  background: var(--color-surface);
  transition: border-color var(--motion-1) var(--ease-standard),
    box-shadow var(--motion-1) var(--ease-standard);
}
.ui-field__control:focus {
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
  outline: none;
}
.ui-field--error .ui-field__control {
  border-color: var(--danger-fg);
}
.ui-field__message {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin: 0;
}
.ui-field__message--error {
  color: var(--danger-fg);
}
@media (max-width: 767.98px) {
  .ui-field__control {
    min-height: 44px;
  }
}
</style>
