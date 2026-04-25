<script setup lang="ts">
import { computed } from 'vue'
import { Info, CheckCircle2, AlertTriangle, XCircle } from 'lucide-vue-next'

interface Props {
  tone?: 'info' | 'success' | 'warning' | 'danger'
  title?: string
  icon?: boolean
}
const props = withDefaults(defineProps<Props>(), {
  tone: 'info',
  title: '',
  icon: true,
})

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
    class="ui-banner"
    :class="`ui-banner--${tone}`"
    role="status"
    :aria-live="tone === 'danger' ? 'assertive' : 'polite'"
  >
    <component :is="iconComponent" v-if="icon" class="ui-banner__icon" :size="18" aria-hidden="true" />
    <div class="ui-banner__body">
      <strong v-if="title" class="ui-banner__title">{{ title }}</strong>
      <div v-if="$slots.default" class="ui-banner__message"><slot /></div>
    </div>
    <div v-if="$slots.actions" class="ui-banner__actions"><slot name="actions" /></div>
  </div>
</template>

<style scoped>
.ui-banner {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  border-left: 4px solid;
}
.ui-banner__icon {
  flex-shrink: 0;
  margin-top: 2px;
}
.ui-banner__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.ui-banner__title {
  font-weight: 600;
  font-size: var(--font-size-md);
}
.ui-banner__message {
  font-size: var(--font-size-sm);
  line-height: var(--line-height-normal);
}
.ui-banner__actions {
  flex-shrink: 0;
}

.ui-banner--info {
  background: var(--info-weak);
  border-left-color: var(--info-fg);
  color: var(--info-fg);
}
.ui-banner--success {
  background: var(--success-weak);
  border-left-color: var(--success-fg);
  color: var(--success-fg);
}
.ui-banner--warning {
  background: var(--warning-weak);
  border-left-color: var(--warning-fg);
  color: var(--warning-fg);
}
.ui-banner--danger {
  background: var(--danger-weak);
  border-left-color: var(--danger-fg);
  color: var(--danger-fg);
}

.ui-banner__title,
.ui-banner__message {
  color: inherit;
}
</style>
