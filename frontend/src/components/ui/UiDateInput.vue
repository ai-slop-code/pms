<script setup lang="ts">
import UiInput from './UiInput.vue'

interface Props {
  modelValue?: string | null
  label?: string
  help?: string
  error?: string
  required?: boolean
  disabled?: boolean
  min?: string
  max?: string
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
  min: '',
  max: '',
  id: '',
  name: '',
})

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

function onUpdate(v: string) {
  emit('update:modelValue', v)
}
</script>

<template>
  <UiInput
    :id="id"
    :model-value="props.modelValue ?? ''"
    type="date"
    :label="label"
    :help="help"
    :error="error"
    :required="required"
    :disabled="disabled"
    :name="name"
    @update:model-value="onUpdate"
  />
</template>
