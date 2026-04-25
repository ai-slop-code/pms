<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { Paperclip, X } from 'lucide-vue-next'

interface Props {
  modelValue?: File | null
  label?: string
  accept?: string
  help?: string
  error?: string
  required?: boolean
  disabled?: boolean
  buttonLabel?: string
  emptyText?: string
  id?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: null,
  label: '',
  accept: '',
  help: '',
  error: '',
  required: false,
  disabled: false,
  buttonLabel: 'Choose file',
  emptyText: 'No file selected',
  id: '',
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: File | null): void
}>()

const autoId = useId()
const fieldId = computed(() => props.id || `ui-file-${autoId}`)
const helpId = computed(() => `${fieldId.value}-help`)
const errorId = computed(() => `${fieldId.value}-error`)

const describedBy = computed(() => {
  const parts: string[] = []
  if (props.help) parts.push(helpId.value)
  if (props.error) parts.push(errorId.value)
  return parts.length ? parts.join(' ') : undefined
})

const inputRef = ref<HTMLInputElement | null>(null)
const filename = computed(() => props.modelValue?.name || '')

function openPicker() {
  if (props.disabled) return
  inputRef.value?.click()
}

function onChange(e: Event) {
  const target = e.target as HTMLInputElement
  emit('update:modelValue', target.files?.[0] ?? null)
}

function clear() {
  emit('update:modelValue', null)
  if (inputRef.value) inputRef.value.value = ''
}

watch(
  () => props.modelValue,
  (value) => {
    if (!value && inputRef.value) {
      inputRef.value.value = ''
    }
  }
)
</script>

<template>
  <div class="ui-field ui-file" :class="{ 'ui-field--error': !!error, 'ui-field--disabled': disabled }">
    <label v-if="label" :for="fieldId" class="ui-field__label">
      {{ label }}<span v-if="required" class="ui-field__required" aria-hidden="true">*</span>
    </label>
    <div class="ui-file__row">
      <button
        type="button"
        class="ui-file__btn"
        :aria-label="label ? undefined : buttonLabel"
        :aria-describedby="describedBy"
        :disabled="disabled || undefined"
        @click="openPicker"
      >
        <Paperclip :size="14" aria-hidden="true" />
        <span>{{ buttonLabel }}</span>
      </button>
      <span v-if="filename" class="ui-file__name" :title="filename">{{ filename }}</span>
      <span v-else class="ui-file__empty">{{ emptyText }}</span>
      <button
        v-if="filename && !disabled"
        type="button"
        class="ui-file__clear"
        aria-label="Clear selected file"
        @click="clear"
      >
        <X :size="14" aria-hidden="true" />
      </button>
      <input
        :id="fieldId"
        ref="inputRef"
        type="file"
        class="ui-file__native"
        :accept="accept || undefined"
        :required="required || undefined"
        :disabled="disabled || undefined"
        :aria-invalid="!!error || undefined"
        tabindex="-1"
        @change="onChange"
      />
    </div>
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
.ui-file__row {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: 36px;
  flex-wrap: wrap;
}
.ui-file__btn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  height: 36px;
  padding: 0 14px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text);
  font: 500 var(--font-size-sm) / 1 var(--font-family-sans);
  cursor: pointer;
  transition: background var(--motion-1) var(--ease-standard),
    border-color var(--motion-1) var(--ease-standard);
}
.ui-file__btn:hover:not(:disabled) {
  background: var(--color-sunken);
  border-color: var(--color-border-strong, var(--color-border));
}
.ui-file__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ui-file__name {
  font-size: var(--font-size-sm);
  color: var(--color-text);
  max-width: 28ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ui-file__empty {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-style: italic;
}
.ui-file__clear {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
}
.ui-file__clear:hover {
  background: var(--color-sunken);
  color: var(--color-text);
}
.ui-file__native {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
.ui-field--error .ui-file__btn {
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
</style>

