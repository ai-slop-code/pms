<script setup lang="ts">
interface Props {
  sticky?: boolean
}
withDefaults(defineProps<Props>(), { sticky: false })
</script>

<template>
  <div class="ui-toolbar" :class="{ 'ui-toolbar--sticky': sticky }">
    <div class="ui-toolbar__main"><slot /></div>
    <div v-if="$slots.trailing" class="ui-toolbar__trailing"><slot name="trailing" /></div>
  </div>
</template>

<style scoped>
.ui-toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-1);
}
.ui-toolbar--sticky {
  position: sticky;
  top: calc(var(--topbar-height) + var(--space-3));
  z-index: 5;
}
.ui-toolbar__main {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  flex: 1;
}
.ui-toolbar__trailing {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-left: auto;
}
@media (max-width: 639.98px) {
  .ui-toolbar {
    padding: var(--space-3);
  }
}
</style>
