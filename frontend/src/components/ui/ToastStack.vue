<script setup lang="ts">
import { useToast } from '@/composables/useToast'
import UiToast from './UiToast.vue'

const { toasts, dismiss } = useToast()
</script>

<template>
  <Teleport to="body">
    <div class="toast-stack" aria-label="Notifications" role="region">
      <TransitionGroup name="toast-stack" tag="div" class="toast-stack__list">
        <UiToast
          v-for="t in toasts"
          :key="t.id"
          :tone="t.tone"
          :title="t.title"
          :message="t.message"
          @dismiss="dismiss(t.id)"
        />
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-stack {
  position: fixed;
  top: calc(var(--topbar-height) + var(--space-3));
  right: var(--space-4);
  z-index: 60;
  pointer-events: none;
}
.toast-stack__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.toast-stack :deep(.ui-toast) {
  pointer-events: auto;
}

.toast-stack-leave-active {
  transition: opacity var(--motion-2) var(--ease-standard),
    transform var(--motion-2) var(--ease-standard);
}
.toast-stack-leave-to {
  opacity: 0;
  transform: translateX(12px);
}

@media (prefers-reduced-motion: reduce) {
  .toast-stack-leave-active {
    transition: none;
  }
}

@media (max-width: 639.98px) {
  .toast-stack {
    left: var(--space-3);
    right: var(--space-3);
  }
  .toast-stack :deep(.ui-toast) {
    min-width: 0;
    max-width: none;
  }
}
</style>
