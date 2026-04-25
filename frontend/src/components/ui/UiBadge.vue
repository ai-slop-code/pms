<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'info'
  dot?: boolean
  size?: 'sm' | 'md'
  label?: string
}
const props = withDefaults(defineProps<Props>(), {
  tone: 'neutral',
  dot: false,
  size: 'md',
  label: '',
})

const classes = computed(() => [
  'ui-badge',
  `ui-badge--${props.tone}`,
  `ui-badge--${props.size}`,
])
</script>

<template>
  <span :class="classes">
    <span v-if="dot" class="ui-badge__dot" aria-hidden="true" />
    <slot>{{ label }}</slot>
  </span>
</template>

<style scoped>
.ui-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-weight: 600;
  line-height: 1;
  border-radius: var(--radius-full);
  padding: 2px 10px;
  white-space: nowrap;
}
.ui-badge--sm {
  font-size: var(--font-size-2xs);
  padding: 2px 8px;
}
.ui-badge--md {
  font-size: var(--font-size-xs);
}

.ui-badge__dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: var(--radius-full);
  background: currentColor;
}

.ui-badge--neutral {
  background: var(--color-sunken);
  color: var(--color-text-muted);
}
.ui-badge--success {
  background: var(--success-weak);
  color: var(--success-fg);
}
.ui-badge--warning {
  background: var(--warning-weak);
  color: var(--warning-fg);
}
.ui-badge--danger {
  background: var(--danger-weak);
  color: var(--danger-fg);
}
.ui-badge--info {
  background: var(--info-weak);
  color: var(--info-fg);
}
</style>
