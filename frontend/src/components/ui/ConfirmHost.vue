<script setup lang="ts">
import UiDialog from './UiDialog.vue'
import UiButton from './UiButton.vue'
import { useConfirm } from '@/composables/useConfirm'

const { state, respond } = useConfirm()

function onUpdateOpen(value: boolean) {
  if (!value) respond(false)
}
</script>

<template>
  <UiDialog
    :open="state.open"
    :title="state.options.title || 'Confirm'"
    size="sm"
    @update:open="onUpdateOpen"
  >
    <p class="confirm-host__message">{{ state.options.message }}</p>
    <template #footer>
      <UiButton variant="secondary" @click="respond(false)">
        {{ state.options.cancelLabel || 'Cancel' }}
      </UiButton>
      <UiButton
        :variant="state.options.tone === 'danger' ? 'danger' : 'primary'"
        @click="respond(true)"
      >
        {{ state.options.confirmLabel || 'Confirm' }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<style scoped>
.confirm-host__message {
  margin: 0;
  color: var(--color-text);
  line-height: var(--line-height-body);
  white-space: pre-wrap;
}
</style>
