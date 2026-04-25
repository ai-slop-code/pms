<script setup lang="ts">
import { Check } from 'lucide-vue-next'
import UiCard from '@/components/ui/UiCard.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { LANG_LABELS } from './helpers'
import type { CleaningMessageResponse } from '@/api/types/messages'

defineProps<{
  message: CleaningMessageResponse
  copied: boolean
}>()

const emit = defineEmits<{ copy: [] }>()
</script>

<template>
  <UiCard class="msg-card">
    <div class="msg-header">
      <div class="msg-lang">
        <UiBadge tone="info">{{ message.language_code.toUpperCase() }}</UiBadge>
        <span class="lang-name">
          {{ LANG_LABELS[message.language_code] || message.language_code }}
          · {{ message.stays_count }} stay{{ message.stays_count === 1 ? '' : 's' }}
        </span>
      </div>
      <UiButton :variant="copied ? 'primary' : 'secondary'" size="sm" @click="emit('copy')">
        <template v-if="copied" #iconLeft>
          <Check :size="14" aria-hidden="true" />
        </template>
        {{ copied ? 'Copied' : 'Copy' }}
      </UiButton>
    </div>
    <h4 class="msg-title">{{ message.title }}</h4>
    <pre class="msg-body">{{ message.body }}</pre>
  </UiCard>
</template>

<style scoped>
.msg-card { margin-top: var(--space-3); }
.msg-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-2);
  gap: var(--space-3);
  flex-wrap: wrap;
}
.msg-lang {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.lang-name {
  font-weight: 500;
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
.msg-title {
  margin: var(--space-2) 0;
  font-size: var(--font-size-md);
  color: var(--color-text);
  font-weight: 600;
}
.msg-body {
  background: var(--color-sunken);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  font-family: inherit;
  font-size: var(--font-size-sm);
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
  margin: 0;
  color: var(--color-text);
}
</style>
