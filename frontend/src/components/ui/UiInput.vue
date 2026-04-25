<script setup lang="ts">
import { computed, useId } from 'vue'

interface Props {
  modelValue?: string | number | null
  label?: string
  type?: string
  placeholder?: string
  help?: string
  error?: string
  required?: boolean
  disabled?: boolean
  readonly?: boolean
  autocomplete?: string
  id?: string
  name?: string
  inputmode?: 'none' | 'text' | 'tel' | 'url' | 'email' | 'numeric' | 'decimal' | 'search'
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  label: '',
  type: 'text',
  placeholder: '',
  help: '',
  error: '',
  required: false,
  disabled: false,
  readonly: false,
  autocomplete: '',
  id: '',
  name: '',
  inputmode: undefined,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'blur'): void
}>()

const autoId = useId()
const fieldId = computed(() => props.id || `ui-input-${autoId}`)
const helpId = computed(() => `${fieldId.value}-help`)
const errorId = computed(() => `${fieldId.value}-error`)

const describedBy = computed(() => {
  const parts: string[] = []
  if (props.help) parts.push(helpId.value)
  if (props.error) parts.push(errorId.value)
  return parts.length ? parts.join(' ') : undefined
})

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="ui-field" :class="{ 'ui-field--error': !!error, 'ui-field--disabled': disabled }">
    <label v-if="label" :for="fieldId" class="ui-field__label">
      {{ label }}<span v-if="required" class="ui-field__required" aria-hidden="true">*</span>
    </label>
    <input
      :id="fieldId"
      class="ui-field__control"
      :type="type"
      :value="modelValue ?? ''"
      :placeholder="placeholder || undefined"
      :required="required || undefined"
      :disabled="disabled || undefined"
      :readonly="readonly || undefined"
      :autocomplete="autocomplete || undefined"
      :name="name || undefined"
      :inputmode="inputmode"
      :aria-invalid="!!error || undefined"
      :aria-describedby="describedBy"
      @input="onInput"
      @blur="emit('blur')"
    />
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
  margin-bottom: 0;
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
.ui-field__control:disabled {
  background: var(--color-sunken);
  color: var(--color-text-disabled);
  cursor: not-allowed;
}
.ui-field--error .ui-field__control {
  border-color: var(--danger-fg);
}
.ui-field--error .ui-field__control:focus {
  box-shadow: var(--focus-ring-danger);
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
