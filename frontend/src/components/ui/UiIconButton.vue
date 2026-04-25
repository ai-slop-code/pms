<script setup lang="ts">
interface Props {
  label: string
  variant?: 'ghost' | 'secondary'
  size?: 'sm' | 'md'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
}

withDefaults(defineProps<Props>(), {
  variant: 'ghost',
  size: 'md',
  type: 'button',
  disabled: false,
})
</script>

<template>
  <button
    class="ui-iconbtn"
    :class="[`ui-iconbtn--${variant}`, `ui-iconbtn--${size}`]"
    :type="type"
    :disabled="disabled || undefined"
    :aria-label="label"
  >
    <slot />
  </button>
</template>

<style scoped>
.ui-iconbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background var(--motion-1) var(--ease-standard),
    color var(--motion-1) var(--ease-standard);
}
.ui-iconbtn--sm {
  width: 28px;
  height: 28px;
}
.ui-iconbtn--md {
  width: 36px;
  height: 36px;
}
.ui-iconbtn--ghost:hover {
  background: var(--color-sunken);
  color: var(--color-text);
}
.ui-iconbtn--secondary {
  border-color: var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
}
.ui-iconbtn--secondary:hover {
  background: var(--color-sunken);
}
.ui-iconbtn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
@media (max-width: 767.98px) {
  .ui-iconbtn--md {
    width: 44px;
    height: 44px;
  }
}
</style>
