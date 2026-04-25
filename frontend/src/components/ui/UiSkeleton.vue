<script setup lang="ts">
interface Props {
  variant?: 'text' | 'rect' | 'kpi' | 'row'
  count?: number
  /** For text variant, width e.g. "60%" */
  width?: string
  height?: string
}
withDefaults(defineProps<Props>(), {
  variant: 'text',
  count: 1,
  width: '',
  height: '',
})
</script>

<template>
  <div class="ui-skeleton-group" aria-hidden="true">
    <template v-if="variant === 'kpi'">
      <div v-for="n in count" :key="n" class="ui-skel ui-skel--kpi" />
    </template>
    <template v-else-if="variant === 'row'">
      <div v-for="n in count" :key="n" class="ui-skel ui-skel--row" />
    </template>
    <template v-else-if="variant === 'rect'">
      <div
        v-for="n in count"
        :key="n"
        class="ui-skel ui-skel--rect"
        :style="{ width: width || '100%', height: height || '120px' }"
      />
    </template>
    <template v-else>
      <div
        v-for="n in count"
        :key="n"
        class="ui-skel ui-skel--text"
        :style="{ width: width || '100%' }"
      />
    </template>
  </div>
</template>

<style scoped>
.ui-skeleton-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ui-skel {
  background: linear-gradient(
    90deg,
    var(--color-sunken) 0%,
    var(--color-border) 50%,
    var(--color-sunken) 100%
  );
  background-size: 200% 100%;
  animation: ui-skel-shimmer 1.4s linear infinite;
  border-radius: var(--radius-md);
}
.ui-skel--text {
  height: 12px;
  border-radius: var(--radius-sm);
}
.ui-skel--kpi {
  height: 104px;
}
.ui-skel--row {
  height: 40px;
}
@keyframes ui-skel-shimmer {
  to {
    background-position: -200% 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .ui-skel {
    animation: none;
    background: var(--color-sunken);
  }
}
</style>
