<script setup lang="ts">
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { LANG_LABELS } from './helpers'
import type { MessageTemplate } from '@/api/types/messages'

defineProps<{
  template: MessageTemplate
  title: string
  body: string
  supportedPlaceholders: string[]
  saving: boolean
}>()

const emit = defineEmits<{
  'update:title': [value: string]
  'update:body': [value: string]
  save: []
  cancel: []
  insertPlaceholder: [placeholder: string]
}>()
</script>

<template>
  <UiSection :title="`Edit template — ${LANG_LABELS[template.language_code] || template.language_code}`">
    <UiCard>
      <UiInput
        :model-value="title"
        label="Title"
        @update:model-value="emit('update:title', String($event))"
      />
      <label class="field" style="margin-top: var(--space-3)">
        <span class="field__label">Body</span>
        <textarea
          :value="body"
          rows="14"
          class="template-textarea"
          @input="emit('update:body', ($event.target as HTMLTextAreaElement).value)"
        />
      </label>
      <div class="placeholders-bar">
        <span class="ph-label">Insert placeholder:</span>
        <button
          v-for="ph in supportedPlaceholders"
          :key="ph"
          type="button"
          class="ph-btn"
          @click="emit('insertPlaceholder', ph)"
        >{{ ph }}</button>
      </div>
      <div class="actions actions--right">
        <UiButton variant="ghost" @click="emit('cancel')">Cancel</UiButton>
        <UiButton variant="primary" :loading="saving" @click="emit('save')">Save template</UiButton>
      </div>
    </UiCard>
  </UiSection>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.field__label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-muted);
}
.template-textarea {
  width: 100%;
  font-family: var(--font-family-mono, 'SF Mono', 'Fira Code', monospace);
  font-size: 0.88rem;
  line-height: 1.5;
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text);
  resize: vertical;
}
.template-textarea:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
}
.placeholders-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1);
  margin-top: var(--space-3);
}
.ph-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-right: var(--space-1);
}
.ph-btn {
  background: var(--color-sunken);
  color: var(--color-text);
  font-size: var(--font-size-xs);
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border);
  font-family: var(--font-family-mono, monospace);
  cursor: pointer;
  min-height: 0;
}
.ph-btn:hover {
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  color: var(--color-primary);
}
.actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.actions--right {
  justify-content: flex-end;
}
</style>
