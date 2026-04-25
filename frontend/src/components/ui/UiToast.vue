<script setup lang="ts">
import { computed } from 'vue'
import { Info, CheckCircle2, AlertTriangle, XCircle, X } from 'lucide-vue-next'
import type { ToastTone } from '@/composables/useToast'

interface Props {
  tone?: ToastTone
  title?: string
  message?: string
}
const props = withDefaults(defineProps<Props>(), {
  tone: 'info',
  title: '',
  message: '',
})

const emit = defineEmits<{ (e: 'dismiss'): void }>()

const iconComponent = computed(() => {
  switch (props.tone) {
    case 'success':
      return CheckCircle2
    case 'warning':
      return AlertTriangle
    case 'danger':
      return XCircle
    default:
      return Info
  }
})
</script>

<template>
  <div
    class="ui-toast"
    :class="`ui-toast--${tone}`"
    role="status"
    :aria-live="tone === 'danger' ? 'assertive' : 'polite'"
  >
    <component :is="iconComponent" class="ui-toast__icon" :size="18" aria-hidden="true" />
    <div class="ui-toast__body">
      <strong v-if="title" class="ui-toast__title">{{ title }}</strong>
      <div v-if="message" class="ui-toast__message">{{ message }}</div>
    </div>
    <button
      type="button"
      class="ui-toast__close"
      aria-label="Dismiss notification"
      @click="emit('dismiss')"
    >
      <X :size="14" aria-hidden="true" />
    </button>
  </div>
</template>

<style scoped>
.ui-toast {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-left: 4px solid;
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-2);
  min-width: 280px;
  max-width: 420px;
  animation: ui-toast-in var(--motion-2) var(--ease-standard);
}

@keyframes ui-toast-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-toast {
    animation: none;
  }
}

.ui-toast__icon {
  flex-shrink: 0;
  margin-top: 2px;
}
.ui-toast__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.ui-toast__title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text);
}
.ui-toast__message {
  font-size: var(--font-size-sm);
  color: var(--color-text);
  line-height: var(--line-height-normal);
  word-break: break-word;
}
.ui-toast__close {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  margin-top: -2px;
}
.ui-toast__close:hover {
  background: var(--color-sunken);
  color: var(--color-text);
}

.ui-toast--info {
  border-left-color: var(--info-fg);
}
.ui-toast--info .ui-toast__icon {
  color: var(--info-fg);
}
.ui-toast--success {
  border-left-color: var(--success-fg);
}
.ui-toast--success .ui-toast__icon {
  color: var(--success-fg);
}
.ui-toast--warning {
  border-left-color: var(--warning-fg);
}
.ui-toast--warning .ui-toast__icon {
  color: var(--warning-fg);
}
.ui-toast--danger {
  border-left-color: var(--danger-fg);
}
.ui-toast--danger .ui-toast__icon {
  color: var(--danger-fg);
}
</style>
