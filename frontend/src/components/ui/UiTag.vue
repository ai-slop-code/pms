<script setup lang="ts">
interface Props {
  label: string
  tone?: 'neutral' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
  removable?: boolean
}

withDefaults(defineProps<Props>(), {
  tone: 'neutral',
  removable: false,
})

defineEmits<{ (e: 'remove'): void }>()
</script>

<template>
  <span class="ui-tag" :class="`ui-tag--${tone}`">
    <slot>{{ label }}</slot>
    <button
      v-if="removable"
      type="button"
      class="ui-tag__remove"
      :aria-label="`Remove ${label}`"
      @click="$emit('remove')"
    >
      ×
    </button>
  </span>
</template>

<style scoped>
.ui-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: var(--font-size-xs);
  font-weight: 500;
  padding: 2px var(--space-2);
  border-radius: var(--radius-md);
  line-height: 1.4;
}
.ui-tag--neutral {
  background: var(--color-sunken);
  color: var(--color-text-muted);
}
.ui-tag--primary {
  background: var(--color-primary-weak);
  color: var(--color-primary);
}
.ui-tag--success {
  background: var(--success-weak);
  color: var(--success-fg);
}
.ui-tag--warning {
  background: var(--warning-weak);
  color: var(--warning-fg);
}
.ui-tag--danger {
  background: var(--danger-weak);
  color: var(--danger-fg);
}
.ui-tag--info {
  background: var(--info-weak);
  color: var(--info-fg);
}
.ui-tag__remove {
  appearance: none;
  border: none;
  background: transparent;
  color: currentColor;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  padding: 0 2px;
  opacity: 0.7;
}
.ui-tag__remove:hover {
  opacity: 1;
}
</style>
