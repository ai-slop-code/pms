<script setup lang="ts">
import { computed } from 'vue'
import { Loader2 } from 'lucide-vue-next'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'
type Size = 'sm' | 'md' | 'lg'

interface Props {
  variant?: Variant
  size?: Size
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  block?: boolean
  ariaLabel?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'secondary',
  size: 'md',
  type: 'button',
  disabled: false,
  loading: false,
  block: false,
  ariaLabel: undefined,
})

const classes = computed(() => [
  'ui-btn',
  `ui-btn--${props.variant}`,
  `ui-btn--${props.size}`,
  { 'ui-btn--block': props.block, 'ui-btn--loading': props.loading },
])
</script>

<template>
  <button
    :type="type"
    :class="classes"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
    :aria-label="ariaLabel"
  >
    <Loader2 v-if="loading" class="ui-btn__spinner" :size="16" aria-hidden="true" />
    <slot v-else name="iconLeft" />
    <span v-if="$slots.default" class="ui-btn__label"><slot /></span>
    <slot name="iconRight" />
  </button>
</template>

<style scoped>
.ui-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  font-family: var(--font-family-sans);
  font-weight: 500;
  cursor: pointer;
  transition: background var(--motion-1) var(--ease-standard),
    color var(--motion-1) var(--ease-standard),
    border-color var(--motion-1) var(--ease-standard);
  white-space: nowrap;
  user-select: none;
}
.ui-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ui-btn--block {
  width: 100%;
}

/* Sizes */
.ui-btn--sm {
  height: 28px;
  padding: 0 10px;
  font-size: var(--font-size-sm);
}
.ui-btn--md {
  height: 36px;
  padding: 0 14px;
  font-size: var(--font-size-md);
}
.ui-btn--lg {
  height: 44px;
  padding: 0 18px;
  font-size: var(--font-size-md);
}

/* Variants */
.ui-btn--primary {
  background: var(--color-primary);
  color: #fff;
}
.ui-btn--primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.ui-btn--secondary {
  background: var(--color-surface);
  color: var(--color-text);
  border-color: var(--color-border);
}
.ui-btn--secondary:hover:not(:disabled) {
  background: var(--color-sunken);
  border-color: var(--color-border-strong);
}

.ui-btn--ghost {
  background: transparent;
  color: var(--color-text-muted);
}
.ui-btn--ghost:hover:not(:disabled) {
  background: var(--color-sunken);
  color: var(--color-text);
}

.ui-btn--danger {
  background: var(--danger-bg);
  color: #fff;
}
.ui-btn--danger:hover:not(:disabled) {
  background: var(--danger-fg);
}

.ui-btn__spinner {
  animation: ui-btn-spin 900ms linear infinite;
}
@keyframes ui-btn-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (max-width: 767.98px) {
  .ui-btn--sm {
    min-height: 32px;
  }
  .ui-btn--md {
    min-height: 40px;
  }
}
</style>
